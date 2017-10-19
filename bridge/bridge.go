package bridge

import (
	"errors"
	"log"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/docker/docker/api/types/swarm"
)

// format: node_id:container_name:port(:"upd")
var containerSvcIDPattern = regexp.MustCompile(`^(.+?):([a-zA-Z0-9][a-zA-Z0-9_.-]+):[0-9]+(?::udp)?$`)

// format: "swarm-vip-svc":swarm_service_id:node_id:port(:"upd")
var swarmVipSvcIDPattern = regexp.MustCompile(`^swarm-vip-svc:(.+?):([a-zA-Z0-9][a-zA-Z0-9_.-]+):[0-9]+(?::udp)?$`)

// format: "swarm-mgr-svc":node_id:port
var swarmMgrSvcIDPattern = regexp.MustCompile(`^swarm-mgr-svc:(.+?):[0-9]+$`)


type Bridge struct {
	sync.Mutex
	registry              RegistryAdapter
	docker                *dockerapi.Client
	// containerId -> services
	containerServices     map[string][]*Service
	deadContainers        map[string]*DeadContainer
	// swarmServiceId -> services
	swarmVipServices      map[string][]*Service
	// swarmNodeId -> services
	swarmMgrServices      map[string][]*Service
	config                Config
	swarmControlAvailable bool
}

func New(docker *dockerapi.Client, adapterUri string, config Config) (*Bridge, error) {
	uri, err := url.Parse(adapterUri)
	if err != nil {
		return nil, errors.New("bad adapter uri: " + adapterUri)
	}
	factory, found := AdapterFactories.Lookup(uri.Scheme)
	if !found {
		return nil, errors.New("unrecognized adapter: " + adapterUri)
	}

	log.Println("Using", uri.Scheme, "adapter:", uri)

	return &Bridge{
		docker:                docker,
		config:                config,
		registry:              factory.New(uri),
		containerServices:     make(map[string][]*Service),
		deadContainers:        make(map[string]*DeadContainer),
		swarmVipServices:      make(map[string][]*Service),
	}, nil
}

func (b *Bridge) Ping() error {
	return b.registry.Ping()
}

func (b *Bridge) Add(containerId string) {
	b.Lock()
	defer b.Unlock()
	b.add(containerId, false)
}

func (b *Bridge) Remove(containerId string) {
	b.remove(containerId, true)
}

func (b *Bridge) RemoveOnExit(containerId string) {
	b.remove(containerId, b.shouldRemove(containerId))
}

func (b *Bridge) refreshServices(servicesMap map[string][]*Service) {
	for _, services := range servicesMap {
		for _, service := range services {
			err := b.registry.Refresh(service)
			if err != nil {
				log.Printf("service %s refresh failed: %v", service.ID, err)
				continue
			}
			log.Printf("refreshed: service %s", service.ID)
		}
	}
}

func (b *Bridge) Refresh() {
	b.Lock()
	defer b.Unlock()

	for containerId, deadContainer := range b.deadContainers {
		deadContainer.TTL -= b.config.RefreshInterval
		if deadContainer.TTL <= 0 {
			delete(b.deadContainers, containerId)
		}
	}

	b.refreshServices(b.containerServices)
	b.refreshServices(b.swarmVipServices)
	b.refreshServices(b.swarmMgrServices)
}

func (b *Bridge) Sync(quiet bool) {
	b.Lock()
	defer b.Unlock()

	containers, err := b.docker.ListContainers(dockerapi.ListContainersOptions{})
	if err != nil && quiet {
		log.Println("error listing containers, skipping sync")
		return
	} else if err != nil && !quiet {
		log.Fatal(err)
	}

	log.Printf("Syncing services on %d containers", len(containers))

	// NOTE: This assumes reregistering will do the right thing, i.e. nothing..
	for _, listing := range containers {
		services := b.containerServices[listing.ID]
		if services == nil {
			b.add(listing.ID, quiet)
		} else {
			for _, service := range services {
				err := b.registry.Register(service)
				if err != nil {
					log.Println("sync register failed:", service, err)
				}
			}
		}
	}

	// check whether swarm control is available (is this node a manager node currently)
	b.swarmControlAvailable = b.isSwarmControlAvailable()

	// sync all swarm dependent services
	b.syncSwarmServices()

	// Clean up services that were registered previously, but aren't
	// acknowledged within registrator
	if b.config.Cleanup {
		// Remove services if its corresponding container is not running
		log.Println("Listing non-exited containers")
		filters := map[string][]string{"status": {"created", "restarting", "running", "paused"}}
		nonExitedContainers, err := b.docker.ListContainers(dockerapi.ListContainersOptions{Filters: filters})
		if err != nil {
			log.Println("error listing nonExitedContainers, skipping sync", err)
			return
		}
		for listingId, _ := range b.containerServices {
			found := false
			for _, container := range nonExitedContainers {
				if listingId == container.ID {
					found = true
					break
				}
			}
			// This is a container that does not exist
			if !found {
				log.Printf("stale: Removing service %s because it does not exist", listingId)
				go b.RemoveOnExit(listingId)
			}
		}

		log.Println("Cleaning up dangling services")
		extServices, err := b.registry.Services()
		if err != nil {
			log.Println("cleanup failed:", err)
			return
		}

	Outer:
		for _, extService := range extServices {
			matches := containerSvcIDPattern.FindStringSubmatch(extService.ID)

			if len(matches) != 3 {
				// There's no way this was registered by us, so leave it
				continue
			}
			serviceNodeId := matches[1]
			if serviceNodeId != b.config.NodeId {
				// ignore because registered on a different host
				continue
			}
			serviceContainerName := matches[2]
			for _, listing := range b.containerServices {
				for _, service := range listing {
					if service.Name == extService.Name && serviceContainerName == service.Origin.container.Name[1:] {
						continue Outer
					}
				}
			}
			log.Println("dangling:", extService.ID)
			err := b.registry.Deregister(extService)
			if err != nil {
				log.Println("deregister failed:", extService.ID, err)
				continue
			}
			log.Println(extService.ID, "removed")
		}
	}
}

func (b *Bridge) add(containerId string, quiet bool) {
	if d := b.deadContainers[containerId]; d != nil {
		b.containerServices[containerId] = d.Services
		delete(b.deadContainers, containerId)
	}

	if b.containerServices[containerId] != nil {
		log.Println("container, ", containerId[:12], ", already exists, ignoring")
		// Alternatively, remove and readd or resubmit.
		return
	}

	container, err := b.docker.InspectContainer(containerId)
	if err != nil {
		log.Println("unable to inspect container:", containerId[:12], err)
		return
	}

	ports := make(map[string]ServicePort)

	// Extract configured host port mappings, relevant when using --net=host
	for port, _ := range container.Config.ExposedPorts {
		published := []dockerapi.PortBinding{ {"0.0.0.0", port.Port()}, }
		ports[string(port)] = servicePort(b.config.Internal, container, port, published)
	}

	// Extract runtime port mappings, relevant when using --net=bridge
	for port, published := range container.NetworkSettings.Ports {
		ports[string(port)] = servicePort(b.config.Internal, container, port, published)
	}

	if len(ports) == 0 && !quiet {
		log.Println("ignored:", container.ID[:12], "no published ports")
		return
	}

	for _, port := range ports {
		if b.config.Internal != true && port.HostPort == "" {
			if !quiet {
				log.Println("ignored:", container.ID[:12], "port", port.ExposedPort, "not published on host")
			}
			continue
		}
		service := b.newService(port, len(ports) > 1)
		if service == nil {
			if !quiet {
				log.Println("ignored:", container.ID[:12], "service on port", port.ExposedPort)
			}
			continue
		}
		err := b.registry.Register(service)
		if err != nil {
			log.Println("register failed:", service, err)
			continue
		}
		b.containerServices[container.ID] = append(b.containerServices[container.ID], service)
		log.Println("added:", container.ID[:12], service.ID)
	}

	servicePorts := make(map[string]ServicePort)
	for key, port := range ports {
		if b.config.Internal != true && port.HostPort == "" {
			if !quiet {
				log.Println("ignored:", container.ID[:12], "port", port.ExposedPort, "not published on host")
			}
			continue
		}
		servicePorts[key] = port
	}

	isGroup := len(servicePorts) > 1
	for _, port := range servicePorts {
		service := b.newService(port, isGroup)
		if service == nil {
			if !quiet {
				log.Println("ignored:", container.ID[:12], "service on port", port.ExposedPort)
			}
			continue
		}
		err := b.registry.Register(service)
		if err != nil {
			log.Println("register failed:", service, err)
			continue
		}
		b.containerServices[container.ID] = append(b.containerServices[container.ID], service)
		log.Println("added:", container.ID[:12], service.ID)
	}
}

func (b *Bridge) newService(port ServicePort, isgroup bool) *Service {
	container := port.container
	defaultName := strings.Split(path.Base(container.Config.Image), ":")[0]

	if b.config.HostIp != "" {
		port.HostIP = b.config.HostIp
	}

	if port.HostIP == "0.0.0.0" {
		log.Printf("ignored: no valid ip address found %s", container.ID[:12])
		return nil
	}

	metadata, metadataFromPort := serviceMetaData(container.Config, port.ExposedPort)

	ignore := mapDefault(metadata, "ignore", "")
	if ignore != "" {
		return nil
	}

	serviceName := mapDefault(metadata, "name", "")
	if serviceName == "" {
		if b.config.Explicit {
			log.Printf("ignored: container service without explicit naming %s", container.ID[:12])
			return nil
		}
		serviceName = defaultName
	}

	service := new(Service)
	service.Origin = port

	// consider swarm mode
	if swarmServiceName, ok := port.container.Config.Labels["com.docker.swarm.service.name"]; ok {
		// swarm mode has concept of services
		service.Name = mapDefault(metadata, "name", swarmServiceName)
	} else {
		service.Name = mapDefault(metadata, "name", defaultName)
	}

	// must match containerSvcIDPattern
	service.ID = b.config.NodeId + ":" + container.Name[1:] + ":" + port.ExposedPort

	if isgroup && !metadataFromPort["name"] {
		service.Name += "-" + port.ExposedPort
	}
	var p int

	if b.config.Internal == true {
		service.IP = port.ExposedIP
		p, _ = strconv.Atoi(port.ExposedPort)
	} else {
		service.IP = port.HostIP
		p, _ = strconv.Atoi(port.HostPort)
	}
	service.Port = p

	if b.config.UseIpFromLabel != "" {
		containerIp := container.Config.Labels[b.config.UseIpFromLabel]
		if containerIp != "" {
			slashIndex := strings.LastIndex(containerIp, "/")
			if slashIndex > -1 {
				service.IP = containerIp[:slashIndex]
			} else {
				service.IP = containerIp
			}
			log.Println("using container IP " + service.IP + " from label '" +
				b.config.UseIpFromLabel  + "'")
		} else {
			log.Println("Label '" + b.config.UseIpFromLabel +
				"' not found in container configuration")
		}
	}

	// NetworkMode can point to another container (kuberenetes pods)
	networkMode := container.HostConfig.NetworkMode
	if networkMode != "" {
		if strings.HasPrefix(networkMode, "container:") {
			networkContainerId := strings.Split(networkMode, ":")[1]
			log.Println(service.Name + ": detected container NetworkMode, linked to: " + networkContainerId[:12])
			networkContainer, err := b.docker.InspectContainer(networkContainerId)
			if err != nil {
				log.Println("unable to inspect network container:", networkContainerId[:12], err)
			} else {
				service.IP = networkContainer.NetworkSettings.IPAddress
				log.Println(service.Name + ": using network container IP " + service.IP)
			}
		}
	}

	if port.PortType == "udp" {
		service.Tags = combineTags(
			mapDefault(metadata, "tags", ""), b.config.ForceTags, "udp")
		service.ID = service.ID + ":udp"
	} else {
		service.Tags = combineTags(
			mapDefault(metadata, "tags", ""), b.config.ForceTags)
	}

	id := mapDefault(metadata, "id", "")
	if id != "" {
		service.ID = id
	}

	delete(metadata, "id")
	delete(metadata, "tags")
	delete(metadata, "name")
	service.Attrs = metadata
	service.TTL = b.config.RefreshTtl

	return service
}

/*
 * Find registered services by filtering. This is done by using the given filter function. The filter function
 * is called for each service. If it returns a string reference this reference will be used as key
 * into the result map and the service will be appended to this map entry then.
 */
func filterServices(services []*Service, filter func(service *Service) *string) map[string][]*Service {
	result := make(map[string][]*Service)
	for _, service := range services {
		if key := filter(service); key != nil {
			if services, ok := result[*key]; ok {
				services = append(services, service)
				result[*key] = services
			} else {
				services := make([]*Service, 0)
				services = append(services, service)
				result[*key] = services
			}
		}
	}
	return result
}

/*
 * Find registered swarm mode vip services by filtering. This is done by pattern matching the service ids.
 * Returns a map with the id of the swarm service as key and its associated registered services as value.
 */
func (b *Bridge) filterSwarmVipServices(services []*Service) map[string][]*Service {

	swarmVipServiceFilter := func(service *Service) *string {
		matches := swarmVipSvcIDPattern.FindStringSubmatch(service.ID)

		if len(matches) != 3 {
			// this service wasn't registered as swarm mode service so we have to filter it here
			return nil
		}

		nodeId := matches[2]

		if nodeId != b.config.NodeId {
			// we can't deregister services that were registered on a different node
			return nil
		}

		return &matches[1]
	}

	return filterServices(services, swarmVipServiceFilter)
}

func (b *Bridge) registeredServices() ([]*Service, error) {
	registeredServices, err := b.registry.Services()
	if err != nil {
		log.Fatalf("can't get registered services: %v", err)
	}
	return registeredServices, err
}

/*
 * Iterates over all registered swarm mode vip services from registration backend and calls the given condition function.
 * If condition returns true it deregisters the appropriate service from the registration backend.
 */
func (b *Bridge) deregisterRegisteredSwarmVipServices(condition func(swarmServiceId string, registeredService *Service) bool) {
	registeredServices, err := b.registeredServices()

	if err != nil {
		return
	}

	for swarmServiceId, registeredServices := range b.filterSwarmVipServices(registeredServices) {
		for _, registeredService := range registeredServices {
			if condition(swarmServiceId, registeredService) {
				b.registry.Deregister(registeredService)
				// remove swarm vip service from cache
				delete(b.swarmVipServices, swarmServiceId)
			}
		}
	}
}

func (b *Bridge) syncSwarmServices() {
	b.syncSwarmManagerService()
	b.syncSwarmVipServices()
}

func (b *Bridge) SyncSwarmServices() {
	b.Lock()
	defer b.Unlock()

	swarmControlAvailable := b.swarmControlAvailable

	// check whether swarm control is available (is this node a manager node currently)
	b.swarmControlAvailable = b.isSwarmControlAvailable()

	swarmControlChanged := swarmControlAvailable != b.swarmControlAvailable

	// sync swarm services only when swarm control availability of the own node has changes
	if swarmControlChanged {
		b.syncSwarmServices()
	}
}

/*
 * Syncs docker swarm mode vip services with registration backend. For vip/ingress services each
 * swarm node is part of the routing mesh so we would have to register a service for each node.
 * Sadly this is not possible atm for worker nodes as they don't provide the needed /services endpoint
 * nor it is possible to receive service events via the docker remote api.
 */
func (b *Bridge) syncSwarmVipServices() {

	log.Printf("Syncing swarm mode vip services. Swarm control available: %v", b.swarmControlAvailable)

	var swarmServices []swarm.Service
	var err error

	// docker daemons "/services" api endpoint is available on swarm manager nodes only.
	if b.swarmControlAvailable {
		// get existing swarm services
		servicefilters := map[string][]string{}
		swarmServices, err = b.docker.ListServices(dockerapi.ListServicesOptions{Filters: servicefilters})
		if err != nil {
			log.Printf("error listing swarm mode services: %v", err)
		}
	}

	swarmServicesMap := make(map[string]swarm.Service)

	for _, swarmService := range swarmServices {
		swarmServicesMap[swarmService.ID] = swarmService
		b.registerSwarmService(swarmService)
	}

	deregisterCondition := func(swarmServiceId string, registeredService *Service) bool {
		if swarmService, ok := swarmServicesMap[swarmServiceId]; ok {
			mode := swarmService.Spec.EndpointSpec.Mode
			if swarmService.Spec.Mode.Replicated != nil && b.config.SwarmReplicasAware && *swarmService.Spec.Mode.Replicated.Replicas < uint64(1) {
				log.Printf("removed: swarm vip service without replicas %s:%d ", registeredService.Name, registeredService.Port)
				return true
			} else if mode != swarm.ResolutionModeVIP {
				log.Printf("removed: swarm vip service %s:%d", registeredService.Name, registeredService.Port)
				return true
			} else {
				return false
			}
		}	else {
			log.Printf("removed: swarm vip service not running anymore %s:%d", registeredService.Name, registeredService.Port)
			return true
		}
	}

	b.deregisterRegisteredSwarmVipServices(deregisterCondition)
}

func (b *Bridge) RegisterSwarmServiceById(aSwarmServiceId string) (*swarm.Service, error) {
	b.Lock()
	defer b.Unlock()

	return b.registerSwarmServiceById(aSwarmServiceId)
}

func (b *Bridge) registerSwarmServiceById(aSwarmServiceId string) (*swarm.Service, error) {

	swarmService, err := b.docker.InspectService(aSwarmServiceId)

	if err == nil {
		b.registerSwarmService(*swarmService)
		return swarmService, err
	} else {
		return swarmService, err
	}
}

func (b *Bridge) DeregisterSwarmServiceById(aSwarmServiceId string) {
	b.Lock()
	defer b.Unlock()

	b.deregisterSwarmServiceById(aSwarmServiceId)
}

func (b *Bridge) deregisterSwarmServiceById(aSwarmServiceId string) {

	deregisterCondition := func(swarmServiceId string, registeredService *Service) bool {
		if swarmServiceId == aSwarmServiceId {
			log.Printf("removed: swarm vip service %s:%d", registeredService.Name, registeredService.Port)
			return true
		} else {
			return false
		}
	}

	b.deregisterRegisteredSwarmVipServices(deregisterCondition)
}

func (b *Bridge) UpdateSwarmServiceById(aSwarmServiceId string) {
	b.Lock()
	defer b.Unlock()

	if aSwarmService := b.swarmVipServices[aSwarmServiceId]; aSwarmService == nil {
		// serviceId is unknown so we won't update. this is an optimization for
		// docker service events with unknown service ids (don't know the meaning of those events)
		return
	}

	// registers service eventually (because replicas>0) if currently not registered (because replicas=0)
	swarmService, err := b.registerSwarmServiceById(aSwarmServiceId)

	if err != nil {
		log.Printf("can't register swarm service %s by id: %v", aSwarmServiceId, err)
		return
	}

	deregisterCondition := func(swarmServiceId string, registeredService *Service) bool {
		mode := swarmService.Spec.EndpointSpec.Mode
		if swarmServiceId == aSwarmServiceId && swarmService.Spec.Mode.Replicated != nil && b.config.SwarmReplicasAware && *swarmService.Spec.Mode.Replicated.Replicas < uint64(1) {
			log.Printf("removed: swarm vip service without replicas %s:%d ", registeredService.Name, registeredService.Port)
			return true
		} else if mode != swarm.ResolutionModeVIP {
			log.Printf("removed: swarm vip service %s:%d ", registeredService.Name, registeredService.Port)
			return true
		} else {
			return false
		}
	}

	b.deregisterRegisteredSwarmVipServices(deregisterCondition)
}

func (b *Bridge) registerSwarmService(swarmService swarm.Service) {
	if swarmService.Spec.EndpointSpec != nil {
		mode := swarmService.Spec.EndpointSpec.Mode
		// DNSrr service will be handled as container service (see Sync())
		if mode == swarm.ResolutionModeVIP {
			if (len(swarmService.Endpoint.VirtualIPs) > 0) {
				if swarmService.Spec.Mode.Replicated != nil && b.config.SwarmReplicasAware {
					if *swarmService.Spec.Mode.Replicated.Replicas > uint64(0) {
						b.registerSwarmVipServices(swarmService)
					}
				} else {
					b.registerSwarmVipServices(swarmService)
				}
			}
		} else {
			log.Printf("swarm DNSrr service will be handled as container service", swarmService.Spec.Name)
		}
	}
}

// there are two types of endpoints VIP and DNS rr based
// DNS rr happens implicitly by registering multiple services with the same name
// so that no extra effort is required
// in case of VIP based services, user specifies the published ports
// which are equivalent of docker port binding, but works differently
// swarm mode provides ingress network, where services are load-balanced
// behind VIP address. From inside network (if there any) perspective
// only one service is need, with swarm mode assigned VIP address.
// From outside perspective, every docker host IP address becomes an entry point
// for load-balancer, so published ports shall be registered for each docker host
func (b *Bridge) registerSwarmVipServices(swarmService swarm.Service) {

	// if internal, register the internal VIP services
	if b.config.Internal {
		for _, vip := range swarmService.Endpoint.VirtualIPs {
			if network, err := b.docker.NetworkInfo(vip.NetworkID); err != nil {
				log.Println("unable to inspect network while evaluating VIPs for service:", swarmService.Spec.Name, err)
			} else {
				// no point to publish docker swarm internal ingress network VIP
				if network.Name != "ingress" && len(vip.Addr) > 0 && strings.Contains(vip.Addr, "/") {
					vipAddr := strings.Split(vip.Addr, "/")[0]
					if len(swarmService.Endpoint.Ports) > 0 {
						b.registerSwarmVipServicePorts(swarmService, true, vipAddr)
					}
				}
			}
		}
	} else {
		// if there is no published ports, no point to register it out side
		if len(swarmService.Endpoint.Ports) > 0 {
			b.registerSwarmVipServicePorts(swarmService, false, b.config.HostIp)
		}
	}
}

/*
 * Registers each port of a swarm service as a single service in the registration backend.
 */
func (b *Bridge) registerSwarmVipServicePorts(swarmService swarm.Service, inside bool, vip string) {

	containerLabels := swarmService.Spec.TaskTemplate.ContainerSpec.Labels
	containerEnv := envToMap(swarmService.Spec.TaskTemplate.ContainerSpec.Env)
	serviceLabels := swarmService.Spec.Labels

	metadata := make(map[string]string)

	metadataFilter := func(key string) bool {
		// filter all registrator specific metadata
		return strings.HasPrefix(key, "SERVICE_")
	}

	// order is crucial here as service labels must override container settings
	joinMaps(containerLabels, metadata, metadataFilter)
	joinMaps(containerEnv, metadata, metadataFilter)
	joinMaps(serviceLabels, metadata, metadataFilter)

	portsMetadata := make(map[int]map[string]string)

	for key, value := range metadata {
		key = strings.ToLower(strings.TrimPrefix(key, "SERVICE_"))
		portkey := strings.SplitN(key, "_", 2)
		p, err := strconv.Atoi(portkey[0])
		if err == nil && len(portkey) > 1 {
			if portMeta, ok := portsMetadata[p]; ok {
				portMeta[portkey[1]] = value
			}	else {
				portsMetadata[p] = make(map[string]string)
				portsMetadata[p][portkey[1]] = value
			}
			delete(metadata, key)
		}
	}

	services := make([]*Service, 0)

	for _, port := range swarmService.Endpoint.Ports {

		// If the service port is published in host mode and there isn't a published port configured docker will
		// auto assign a random port. This port is not available via service inspection because it may
		// differ for each replica/host. So we can't register the service here.
		// Instead the appropriate container has a published port defined and will be registered as normal
		// non-swarm service then. See Sync() and Add().
		if port.PublishMode == "host" && port.PublishedPort == 0 {
			continue
		}

		var portNum uint32
		if portNum = port.PublishedPort; inside {
			// inside port is not translated to published port
			portNum = port.TargetPort
		}

		targetPort := int(port.TargetPort)
		serviceName := swarmService.Spec.Name + "-" + strconv.Itoa(targetPort)
		portMeta, ok := portsMetadata[targetPort]

		if ok {
		  if portName, ok := portMeta["name"]; ok {
			  serviceName = portName
				delete(portMeta, "name")
			}
		} else {
			if b.config.Explicit {
				log.Printf("ignored: swarm vip service has no explicit naming %s", swarmService.Spec.Name)
				continue
			}
			portMeta = make(map[string]string)
		}

		// must match swarmVipSvcIDPattern
		serviceID := "swarm-vip-svc:" + swarmService.ID + ":" + b.config.NodeId + ":" + strconv.Itoa(targetPort)

		services = append(services, b.registerSwarmVipServicePort(serviceID, serviceName, portMeta, inside, vip, int(portNum), port.Protocol))
	}

	// cache registered swarm services
	b.swarmVipServices[swarmService.ID] = services

  log.Printf("registered %d services for swarm service %s ", len(services), swarmService.ID)
}

/*
 * Registers a single port of a swarm vip service as service in the registration backend.
 */
func (b *Bridge) registerSwarmVipServicePort(serviceID string, serviceName string, metadata map[string]string, inside bool, vip string, port int, protocol swarm.PortConfigProtocol) *Service {

	var tag string
	if tag = "vip-outside"; inside {
		tag = "vip-inside"
	}

	service := new(Service)

	service.ID = serviceID
	service.Name = serviceName

	tags := mapDefault(metadata, "tags", "")

	// tag it for convenience
	if protocol != swarm.PortConfigProtocolTCP {
		service.Tags = combineTags(tags, b.config.ForceTags, tag, string(protocol))
	} else {
		service.Tags = combineTags(tags, b.config.ForceTags, tag)
	}

	delete(metadata, "tags")
	service.IP = vip
	service.Port = port
	service.Attrs = metadata

	err := b.registry.Register(service)
	if err != nil {
		log.Printf("register failed:", service.Name, err)
	}

	log.Printf("added: swarm vip service %s:%d", service.Name, service.Port)

	return service
}

func (b *Bridge) remove(containerId string, deregister bool) {
	b.Lock()
	defer b.Unlock()

	if deregister {
		deregisterAll := func(services []*Service) {
			for _, service := range services {
				err := b.registry.Deregister(service)
				if err != nil {
					log.Println("deregister failed:", service.ID, err)
					continue
				}
				log.Println("removed:", containerId[:12], service.ID)
			}
		}
		deregisterAll(b.containerServices[containerId])
		if d := b.deadContainers[containerId]; d != nil {
			deregisterAll(d.Services)
			delete(b.deadContainers, containerId)
		}
	} else if b.config.RefreshTtl != 0 && b.containerServices[containerId] != nil {
		// need to stop the refreshing, but can't delete it yet
		b.deadContainers[containerId] = &DeadContainer{b.config.RefreshTtl, b.containerServices[containerId]}
	}
	delete(b.containerServices, containerId)
}

// bit set on ExitCode if it represents an exit via a signal
var dockerSignaledBit = 128

func (b *Bridge) shouldRemove(containerId string) bool {
	if b.config.DeregisterCheck == "always" {
		return true
	}
	container, err := b.docker.InspectContainer(containerId)
	if _, ok := err.(*dockerapi.NoSuchContainer); ok {
		// the container has already been removed from Docker
		// e.g. probabably run with "--rm" to remove immediately
		// so its exit code is not accessible
		log.Printf("registrator: container %v was removed, could not fetch exit code", containerId[:12])
		return true
	}

	switch {
	case err != nil:
		log.Printf("registrator: error fetching status for container %v on \"die\" event: %v\n", containerId[:12], err)
		return false
	case container.State.Running:
		log.Printf("registrator: not removing container %v, still running", containerId[:12])
		return false
	case container.State.ExitCode == 0:
		return true
	case container.State.ExitCode&dockerSignaledBit == dockerSignaledBit:
		return true
	}
	return false
}

func (b *Bridge) isSwarmControlAvailable() bool {
	dockerInfo, err := b.docker.Info()

	if err != nil {
		log.Fatal(err)
	}

	return dockerInfo.Swarm.ControlAvailable
}

/*
 * Find registered swarm mode manager services by filtering. This is done by pattern matching the service ids.
 * Returns a map with the id of the swarm node as key and its associated registered service(s) as value.
 */
func (b *Bridge) filterSwarmManagerServices(services []*Service) map[string][]*Service {

	swarmVipServiceFilter := func(service *Service) *string {
		matches := swarmMgrSvcIDPattern.FindStringSubmatch(service.ID)

		if len(matches) != 2 {
			// this service wasn't registered as swarm manager service so we have to filter it here
			return nil
		}

		nodeId := matches[1]

		if nodeId != b.config.NodeId {
			// we can't deregister services that were registered on a different node
			return nil
		}

		return &nodeId
	}

	return filterServices(services, swarmVipServiceFilter)
}

/*
 * Iterates over all registered swarm manager services from registration backend and calls the given condition function.
 * If condition returns true it deregisters the appropriate service from the registration backend.
 */
func (b *Bridge) deregisterRegisteredSwarmManagerServices(condition func(nodeId string, registeredService *Service) bool) {
	registeredServices, err := b.registeredServices()

	if err != nil {
		return
	}

	for nodeId, registeredServices := range b.filterSwarmManagerServices(registeredServices) {
		for _, registeredService := range registeredServices {
			if condition(nodeId, registeredService) {
				b.registry.Deregister(registeredService)
				// remove swarm manager service from cache
				delete(b.swarmMgrServices, nodeId)
			}
		}
	}
}

func (b *Bridge) syncSwarmManagerService() {

	if b.config.SwarmManagerSvcName != "" {
		log.Printf("Syncing swarm mode manager service. Swarm control available: %v", b.swarmControlAvailable)

		if b.swarmControlAvailable {
			service := new(Service)

			// must match swarmMgrSvcIDPattern
			service.ID = "swarm-mgr-svc:" + b.config.NodeId + ":2376"
			service.Name = b.config.SwarmManagerSvcName
			service.IP = b.config.HostIp
			service.Port = 2376

			err := b.registry.Register(service)
			if err != nil {
				log.Printf("register swarm manager service %s failed: %v", service.Name, err)
			} else {
				log.Printf("registered swarm manager service: %s", service.Name)
			}
		}
	}

	deregisterCondition := func(nodeId string, registeredService *Service) bool {
		remove := !b.swarmControlAvailable
		if remove {
			log.Printf("removed: swarm manager service %s:%d ", registeredService.Name, registeredService.Port)
		}
		return remove
	}

	b.deregisterRegisteredSwarmManagerServices(deregisterCondition)

}
