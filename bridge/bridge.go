package bridge

import (
	"log"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	dockerapi "github.com/fsouza/go-dockerclient"
)

type Bridge struct {
	sync.Mutex
	registry RegistryAdapter
	docker   *dockerapi.Client
	services map[string][]*Service
	config   Config
}

func New(docker *dockerapi.Client, adapterUri string, config Config) *Bridge {
	uri, err := url.Parse(adapterUri)
	if err != nil {
		log.Fatal("Bad adapter URI:", adapterUri)
	}
	factory, found := AdapterFactories.Lookup(uri.Scheme)
	if !found {
		log.Fatal("Unrecognized adapter:", adapterUri)
	}
	adapter := factory.New(uri)
	err = adapter.Ping()
	if err != nil {
		log.Fatalf("%s: %s", uri.Scheme, err)
	}
	log.Println("Using", uri.Scheme, "adapter:", uri)
	return &Bridge{
		docker:   docker,
		config:   config,
		registry: adapter,
		services: make(map[string][]*Service),
	}
}

func (b *Bridge) Add(containerId string) {
	b.Lock()
	defer b.Unlock()
	b.add(containerId, false)
}

func (b *Bridge) Remove(containerId string) {
	b.Lock()
	defer b.Unlock()
	for _, service := range b.services[containerId] {
		log.Println("removing:", containerId[:12], service.ID)
		err := retry(func() error {
			return b.registry.Deregister(service)
		})
		if err != nil {
			log.Println("deregister failed:", service.ID, err)
		}
	}
	delete(b.services, containerId)
}

func (b *Bridge) Refresh() {
	b.Lock()
	defer b.Unlock()
	for containerId, services := range b.services {
		for _, service := range services {
			log.Println("refreshing:", containerId[:12], service.ID)
			err := b.registry.Refresh(service)
			if err != nil {
				log.Println("refresh failed:", service.ID, err)
			}
		}
	}
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

	// NOTE: This assumes reregistering will do the right thing, i.e. nothing.
	// NOTE: This will NOT remove services.
	for _, listing := range containers {
		services := b.services[listing.ID]
		if services == nil {
			b.add(listing.ID, quiet)
		} else {
			for _, service := range services {
				err := retry(func() error {
					return b.registry.Register(service)
				})
				if err != nil {
					log.Println("sync register failed:", service, err)
				}
			}
		}
	}
}

func (b *Bridge) add(containerId string, quiet bool) {
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
		log.Println("adding:", container.ID[:12], service.ID)
		err := retry(func() error {
			return b.registry.Register(service)
		})
		if err != nil {
			log.Println("register failed:", service, err)
		}
		b.services[container.ID] = append(b.services[container.ID], service)
	}

	if len(b.services[container.ID]) == 0 && !quiet {
		log.Println("ignored:", container.ID[:12], "no published ports")
	}
}

func (b *Bridge) newService(port ServicePort, isgroup bool) *Service {
	container := port.container
	defaultName := strings.Split(path.Base(container.Config.Image), ":")[0]
	if isgroup {
		defaultName = defaultName + "-" + port.ExposedPort
	}

	// not sure about this logic. kind of want to remove it.
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
	service.ID = hostname + ":" + container.Name[1:] + ":" + port.ExposedPort
	service.Name = mapDefault(metadata, "name", defaultName)
	var p int
	if b.config.Internal == true {
		service.IP = port.ExposedIP
		p, _ = strconv.Atoi(port.ExposedPort)
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
