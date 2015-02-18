//go:generate go-extpoints .
package bridge

import (
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/cenkalti/backoff"
	dockerapi "github.com/fsouza/go-dockerclient"
)

type ServiceRegistry interface {
	Register(service *Service) error
	Deregister(service *Service) error
	Refresh(service *Service) error
}

type Service struct {
	ID   string
	Name string
	// HostName string
	Port  int
	IP    string
	Tags  []string
	Attrs map[string]string
	TTL   int

	Origin ServicePort
}

type ServicePort struct {
	HostPort    string
	HostIP      string
	HostName    string
	ExposedPort string
	ExposedIP   string
	PortType    string
	Container   *dockerapi.Container
}

func retry(fn func() error) error {
	return backoff.Retry(fn, backoff.NewExponentialBackOff())
}

func mapDefault(m map[string]string, key, default_ string) string {
	v, ok := m[key]
	if !ok {
		return default_
	}
	return v
}

func combineTags(tagParts ...string) []string {
	tags := make([]string, 0)
	for _, element := range tagParts {
		if element != "" {
			tags = append(tags, strings.Split(element, ",")...)
		}
	}
	return tags
}

func serviceMetaData(env []string, port string) map[string]string {
	metadata := make(map[string]string)
	for _, kv := range env {
		kvp := strings.SplitN(kv, "=", 2)
		if strings.HasPrefix(kvp[0], "SERVICE_") && len(kvp) > 1 {
			key := strings.ToLower(strings.TrimPrefix(kvp[0], "SERVICE_"))
			portkey := strings.SplitN(key, "_", 2)
			_, err := strconv.Atoi(portkey[0])
			if err == nil && len(portkey) > 1 {
				if portkey[0] != port {
					continue
				}
				metadata[portkey[1]] = kvp[1]
			} else {
				metadata[key] = kvp[1]
			}
		}
	}
	return metadata
}

type RegistryBridge struct {
	sync.Mutex
	Registry ServiceRegistry
	docker   *dockerapi.Client
	services map[string][]*Service
	config   Config
}

type Config struct {
	HostIp          string
	Internal        bool
	ForceTags       string
	RefreshTtl      int
	RefreshInterval int
}

func New(docker *dockerapi.Client, config Config) *RegistryBridge {
	return &RegistryBridge{
		docker:   docker,
		config:   config,
		services: make(map[string][]*Service),
	}
}

func (b *RegistryBridge) newService(port ServicePort, isgroup bool) *Service {
	container := port.Container
	defaultName := strings.Split(path.Base(container.Config.Image), ":")[0]
	if isgroup {
		defaultName = defaultName + "-" + port.ExposedPort
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = port.HostIP
	} else {
		if port.HostIP == "0.0.0.0" {
			ip, err := net.ResolveIPAddr("ip", hostname)
			if err == nil {
				port.HostIP = ip.String()
			}
		}
	}

	if b.config.HostIp != "" {
		port.HostIP = b.config.HostIp
	}

	metadata := serviceMetaData(container.Config.Env, port.ExposedPort)

	ignore := mapDefault(metadata, "ignore", "")
	if ignore != "" {
		return nil
	}

	service := new(Service)
	service.Origin = port
	if b.config.Internal {
		service.ID = port.HostName + ":" + container.Name[1:] + ":" + port.ExposedPort
	} else {
		service.ID = hostname + ":" + container.Name[1:] + ":" + port.ExposedPort
	}
	service.Name = mapDefault(metadata, "name", defaultName)
	var p int
	if b.config.Internal == true {
		service.IP = port.ExposedIP
		p, _ = strconv.Atoi(port.ExposedPort)
		// service.HostName = port.HostName
	} else {
		service.IP = port.HostIP
		p, _ = strconv.Atoi(port.HostPort)
	}
	service.Port = p

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

func servicePort(container *dockerapi.Container, port dockerapi.Port, published []dockerapi.PortBinding) ServicePort {
	var hp, hip string
	if len(published) > 0 {
		hp = published[0].HostPort
		hip = published[0].HostIP
	}
	if hip == "" {
		hip = "0.0.0.0"
	}
	p := strings.Split(string(port), "/")
	return ServicePort{
		HostPort:    hp,
		HostIP:      hip,
		HostName:    container.Config.Hostname,
		ExposedPort: p[0],
		ExposedIP:   container.NetworkSettings.IPAddress,
		PortType:    p[1],
		Container:   container,
	}
}

func (b *RegistryBridge) Add(containerId string) {
	b.Lock()
	defer b.Unlock()
	b.addInternal(containerId, false)
}

func (b *RegistryBridge) addInternal(containerId string, quiet bool) {
	if b.services[containerId] != nil {
		log.Println("registrator: container, ", containerId[:12], ", already exists, ignoring")
		// Alternatively, remove and readd or resubmit.
		return
	}

	container, err := b.docker.InspectContainer(containerId)
	if err != nil {
		log.Println("registrator: unable to inspect container:", containerId[:12], err)
		return
	}

	ports := make(map[string]ServicePort)

	// Extract configured host port mappings, relevant when using --net=host
	for port, published := range container.HostConfig.PortBindings {
		ports[string(port)] = servicePort(container, port, published)
	}

	// Extract runtime port mappings, relevant when using --net=bridge
	for port, published := range container.NetworkSettings.Ports {
		ports[string(port)] = servicePort(container, port, published)
	}

	for _, port := range ports {
		if *internal != true && port.HostPort == "" {
			if !quiet {
				log.Println("registrator: ignored", container.ID[:12], "port", port.ExposedPort, "not published on host")
			}
			continue
		}
		service := b.newService(port, len(ports) > 1)
		if service == nil {
			if !quiet {
				log.Println("registrator: ignored:", container.ID[:12], "service on port", port.ExposedPort)
			}
			continue
		}
		err := retry(func() error {
			return b.Registry.Register(service)
		})
		if err != nil {
			log.Println("registrator: unable to register service:", service, err)
			continue
		}
		b.services[container.ID] = append(b.services[container.ID], service)
		log.Println("registrator: added:", container.ID[:12], service.ID)
	}

	if len(b.services[container.ID]) == 0 && !quiet {
		log.Println("registrator: ignored:", container.ID[:12], "no published ports")
	}
}

func (b *RegistryBridge) Remove(containerId string) {
	b.Lock()
	defer b.Unlock()
	for _, service := range b.services[containerId] {
		err := retry(func() error {
			return b.Registry.Deregister(service)
		})
		if err != nil {
			log.Println("registrator: unable to deregister service:", service.ID, err)
			continue
		}
		log.Println("registrator: removed:", containerId[:12], service.ID)
	}
	delete(b.services, containerId)
}

func (b *RegistryBridge) Refresh() {
	b.Lock()
	defer b.Unlock()
	for containerId, services := range b.services {
		for _, service := range services {
			err := b.Registry.Refresh(service)
			if err != nil {
				log.Println("registrator: unable to refresh service:", service.ID, err)
				continue
			}
			log.Println("registrator: refreshed:", containerId[:12], service.ID)
		}
	}
}

func (b *RegistryBridge) Sync(quiet bool) {
	b.Lock()
	defer b.Unlock()

	log.Println("registrator: resyncing services")

	containers, err := b.docker.ListContainers(dockerapi.ListContainersOptions{})
	if err != nil && quiet {
		log.Println("registrator: error listing containers, skipping sync")
		return
	} else if err != nil && !quiet {
		log.Fatal(err)
	}

	// NOTE: This assumes reregistering will do the right thing, i.e. nothing.
	// NOTE: This will NOT remove services.
	for _, listing := range containers {
		services := b.services[listing.ID]
		if services == nil {
			b.addInternal(listing.ID, quiet)
		} else {
			for _, service := range services {
				err := retry(func() error {
					return b.registry.Register(service)
				})
				if err != nil {
					log.Println("registrator: unable to sync service:", service, err)
				}
			}
		}
	}
}
