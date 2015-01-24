package main

import (
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	dockerapi "github.com/fsouza/go-dockerclient"
)

type PublishedPort struct {
	HostPort    string
	HostIP      string
	HostName    string
	ExposedPort string
	ExposedIP   string
	PortType    string
	Container   *dockerapi.Container
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

	pp PublishedPort
}

func CombineTags(tagParts ...string) []string {
	tags := make([]string, 0)
	for _, element := range tagParts {
		if element != "" {
			tags = append(tags, strings.Split(element, ",")...)
		}
	}
	return tags
}

func NewService(port PublishedPort, isgroup bool) *Service {
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

	if *hostIp != "" {
		port.HostIP = *hostIp
	}

	metadata := serviceMetaData(container.Config.Env, port.ExposedPort)

	ignore := mapdefault(metadata, "ignore", "")
	if ignore != "" {
		return nil
	}

	service := new(Service)
	service.pp = port
	if *internal {
		service.ID = port.HostName + ":" + container.Name[1:] + ":" + port.ExposedPort
	} else {
		service.ID = hostname + ":" + container.Name[1:] + ":" + port.ExposedPort
	}
	service.Name = mapdefault(metadata, "name", defaultName)
	var p int
	if *internal == true {
		service.IP = port.ExposedIP
		p, _ = strconv.Atoi(port.ExposedPort)
		// service.HostName = port.HostName
	} else {
		service.IP = port.HostIP
		p, _ = strconv.Atoi(port.HostPort)
	}
	service.Port = p

	if port.PortType == "udp" {
		service.Tags = CombineTags(mapdefault(metadata, "tags", ""), *forceTags, "udp")
		service.ID = service.ID + ":udp"
	} else {
		service.Tags = CombineTags(mapdefault(metadata, "tags", ""), *forceTags)
	}

	id := mapdefault(metadata, "id", "")
	if id != "" {
		service.ID = id
	}

	delete(metadata, "id")
	delete(metadata, "tags")
	delete(metadata, "name")
	service.Attrs = metadata

	service.TTL = *refreshTtl

	return service
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
	docker   *dockerapi.Client
	registry ServiceRegistry
	services map[string][]*Service
}

func (b *RegistryBridge) Add(containerId string) {
	b.Lock()
	defer b.Unlock()
	container, err := b.docker.InspectContainer(containerId)
	if err != nil {
		log.Println("registrator: unable to inspect container:", containerId[:12], err)
		return
	}

	ports := make([]PublishedPort, 0)
	for port, published := range container.NetworkSettings.Ports {
		var hp, hip string
		if len(published) > 0 {
			hp = published[0].HostPort
			hip = published[0].HostIP
		}
		p := strings.Split(string(port), "/")
		ports = append(ports, PublishedPort{
			HostPort:    hp,
			HostIP:      hip,
			HostName:    container.Config.Hostname,
			ExposedPort: p[0],
			ExposedIP:   container.NetworkSettings.IPAddress,
			PortType:    p[1],
			Container:   container,
		})
	}

	for _, port := range ports {
		if *internal != true && port.HostPort == "" {
			log.Println("registrator: ignored", container.ID[:12], "port", port.ExposedPort, "not published on host")
			continue
		}
		service := NewService(port, len(ports) > 1)
		if service == nil {
			log.Println("registrator: ignored:", container.ID[:12], "service on port", port.ExposedPort)
			continue
		}
		err := retry(func() error {
			return b.registry.Register(service)
		})
		if err != nil {
			log.Println("registrator: unable to register service:", service, err)
			continue
		}
		b.services[container.ID] = append(b.services[container.ID], service)
		log.Println("registrator: added:", container.ID[:12], service.ID)
	}

	if len(b.services) == 0 {
		log.Println("registrator: ignored:", container.ID[:12], "no published ports")
	}
}

func (b *RegistryBridge) Remove(containerId string) {
	b.Lock()
	defer b.Unlock()
	for _, service := range b.services[containerId] {
		err := retry(func() error {
			return b.registry.Deregister(service)
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
			err := b.registry.Refresh(service)
			if err != nil {
				log.Println("registrator: unable to refresh service:", service.ID, err)
				continue
			}
			log.Println("registrator: refreshed:", containerId[:12], service.ID)
		}
	}
}
