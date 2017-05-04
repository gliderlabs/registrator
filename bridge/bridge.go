package bridge

import (
	"errors"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	dockerapi "github.com/fsouza/go-dockerclient"
)

type Bridge struct {
	sync.Mutex
	hostname       string
	hostip         string
	registry       RegistryAdapter
	docker         *dockerapi.Client
	services       map[string](map[string]*Service)
	deadContainers map[string]*DeadContainer
	config         Config
}

func getHostIP(config Config) (string, error) {
	// set host ip
	intf, err := net.InterfaceByName(config.Intf)
	if err != nil {
		return "", err
	}
	addrs, err := intf.Addrs()
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", errors.New("interface " + intf.Name + " doesn't have any ip address")
	}
	if len(addrs[0].String()) == 0 {
		return "", errors.New("interface " + intf.Name + " doesn't have valid ip address")
	}
	results := strings.Split(addrs[0].String(), "/")
	return results[0], nil
}

func New(docker *dockerapi.Client, adapterUri string, config Config) (*Bridge, error) {
	// set bridge's host ip
	hostIP, err := getHostIP(config)
	if err != nil {
		return nil, err
	}
	// set bridge's host name
	hostname, err := os.Hostname()
	if err != nil || len(hostname) == 0 {
		hostname = hostIP
	}
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
		hostname:       hostname,
		hostip:         hostIP,
		docker:         docker,
		config:         config,
		registry:       factory.New(uri),
		services:       make(map[string](map[string]*Service)),
		deadContainers: make(map[string]*DeadContainer),
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

func (b *Bridge) Refresh() {
	b.Lock()
	defer b.Unlock()

	for containerId, deadContainer := range b.deadContainers {
		deadContainer.TTL -= b.config.RefreshInterval
		if deadContainer.TTL <= 0 {
			delete(b.deadContainers, containerId)
		}
	}

	for containerId, services := range b.services {
		for _, service := range services {
			err := b.registry.Refresh(service)
			if err != nil {
				log.Println("refresh failed:", service.ID, err)
				continue
			}
			log.Println("refreshed:", containerId[:12], service.ID)
		}
	}
}

func (b *Bridge) Sync(quiet bool) {
	b.Lock()
	defer b.Unlock()

	containers, err := b.docker.ListContainers(dockerapi.ListContainersOptions{})
	if err != nil {
		if quiet {
			log.Println("error listing containers, skipping sync")
			return
		}
		log.Fatal(err)
	}

	log.Printf("Syncing services on %d containers", len(containers))

	// NOTE: This assumes reregistering will do the right thing, i.e. nothing..
	for _, listing := range containers {
		services := b.services[listing.ID]
		if services == nil {
			b.add(listing.ID, quiet)
			continue
		}
		for _, service := range services {
			err := b.registry.Register(service)
			if err != nil {
				log.Println("sync register failed:", service, err)
			}
		}
	}

	// Clean up services that were registered previously, but aren't
	// acknowledged within registrator
	if b.config.Cleanup {
		// Remove services if its corresponding container is not running
		log.Println("Listing non-exited containers")
		filters := map[string][]string{"status": {"created", "restarting", "running", "paused"}}
		nonExitedContainerList, err := b.docker.ListContainers(dockerapi.ListContainersOptions{Filters: filters})
		if err != nil {
			log.Println("error listing nonExitedContainers, skipping sync", err)
			return
		}
		nonExitedContainerMap := make(map[string]dockerapi.APIContainers)
		for _, container := range nonExitedContainerList {
			nonExitedContainerMap[container.ID] = container
		}
		// remove on exit when container not found (exited)
		for containerID, _ := range b.services {
			if _, ok := nonExitedContainerMap[containerID]; !ok {
				log.Printf("stale: Removing service %s because it does not exist", containerID)
				go b.RemoveOnExit(containerID)
			}
		}
		// get external services
		log.Println("Listing external services")
		extServices, err := b.registry.Services()
		if err != nil {
			log.Println("listing external services failed:", err)
			return
		}

		for _, extService := range extServices {
			if strings.HasPrefix(extService.ID, b.hostname + ":") == false {
				continue
			}
			if b.getService(extService.ID) != nil {
				continue
			}
			log.Println("service not found (deregister):", extService.ID)
			err := b.registry.Deregister(extService)
			if err != nil {
				log.Println("deregister failed:", extService.ID, err)
				continue
			}
		}
	}
}

func (b *Bridge) getService(extServiceID string) *Service {
	for _, services := range b.services {
		for serviceID, service := range services {
			if serviceID == extServiceID {
				return service
			}
		}
	}
	return nil
}

func (b *Bridge) add(containerId string, quiet bool) {
	if d := b.deadContainers[containerId]; d != nil {
		b.services[containerId] = d.Services
		delete(b.deadContainers, containerId)
	}

	if b.services[containerId] != nil {
		log.Println("container, ", containerId[:12], ", already exists, ignoring")
		// Alternatively, remove and readd or resubmit.
		return
	}

	container, err := b.docker.InspectContainer(containerId)
	if err != nil {
		log.Println("unable to inspect container:", containerId[:12], err)
		return
	}
	
	// build filter from registrator option & labels
	filters, err := NewFilter(container, b.config.Filter)
	if err != nil {
		log.Println("create filter error: " + err.Error())
		return
	}

	// create service from network settings
	for _, containerNetwork := range container.NetworkSettings.Networks {
		network, err := b.docker.NetworkInfo(containerNetwork.NetworkID)
		if err != nil {
			continue
		}
		// rebuild port map from scattered port informations
		servicePorts := make(map[dockerapi.Port][]dockerapi.PortBinding)
		for port, bind := range container.HostConfig.PortBindings {
			if _, ok := servicePorts[port]; !ok {
				servicePorts[port] = bind
			}
		}
		for port, bind := range container.NetworkSettings.Ports {
			if _, ok := servicePorts[port]; !ok {
				servicePorts[port] = bind
			}
		}
		for port, _ := range container.Config.ExposedPorts {
			if _, ok := servicePorts[port]; !ok {
				servicePorts[port] = nil
			}
		}
		// host mode network (consider external)
		if network.Driver == "host" {
			for port, _ := range servicePorts {
				b.registerService(container, b.hostip, port.Port(), port.Proto(), false, filters)
			}
			continue
		}
		// internal network
		if network.Driver == "bridge" || network.Driver == "overlay" {
			for port, portBindings := range servicePorts {
				b.registerService(container, containerNetwork.IPAddress, port.Port(), port.Proto(), true, filters)
				if portBindings != nil {
					for _, bindings := range portBindings {
						if bindings.HostIP == "0.0.0.0" || bindings.HostIP == "" {
							b.registerService(container, b.hostip, bindings.HostPort, port.Proto(), false, filters)
						} else {
							b.registerService(container, bindings.HostIP, bindings.HostPort, port.Proto(), false, filters)
						}
					}
				}
			}
			continue
		}
		// external network
		for port, _ := range servicePorts {
			b.registerService(container, containerNetwork.IPAddress, port.Port(), port.Proto(), false, filters)
		}
	}
}

func (b *Bridge) registerService(container *dockerapi.Container, ip string, port string, proto string, internal bool, filters *Filters) {
	result, _, err := filters.Match(ip, port, internal)
	if err != nil || result == false {
		return
	}
	// set container name (task name > container name)
	containerName := container.Name[1:]
	if name, ok := container.Config.Labels["com.docker.swarm.service.name"]; ok {
		containerName = name
	}

	// create service
	service := new(Service)
	service.ID = b.hostname + ":" + ip + ":" + containerName + ":" + port
	service.Name = containerName
	service.Port, _ = strconv.Atoi(port)
	service.IP = ip
	service.TTL = b.config.RefreshTtl
	// register service
	err = b.registry.Register(service)
	if err != nil {
		log.Println("register failed:", service, err)
		return
	}
	// update local service list
	if _, ok := b.services[container.ID]; !ok {
		b.services[container.ID] = make(map[string]*Service)
	}
	b.services[container.ID][service.ID] = service
	log.Println("added:", container.ID[:12], service.ID)
}

func (b *Bridge) remove(containerId string, deregister bool) {
	b.Lock()
	defer b.Unlock()

	if deregister {
		deregisterAll := func(services map[string]*Service) {
			for _, service := range services {
				err := b.registry.Deregister(service)
				if err != nil {
					log.Println("deregister failed:", service.ID, err)
					continue
				}
				log.Println("removed:", containerId[:12], service.ID)
			}
		}
		deregisterAll(b.services[containerId])
		if d := b.deadContainers[containerId]; d != nil {
			deregisterAll(d.Services)
			delete(b.deadContainers, containerId)
		}
	} else if b.config.RefreshTtl != 0 && b.services[containerId] != nil {
		// need to stop the refreshing, but can't delete it yet
		b.deadContainers[containerId] = &DeadContainer{b.config.RefreshTtl, b.services[containerId]}
	}
	delete(b.services, containerId)
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

func init() {
}
