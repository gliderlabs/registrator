package main

import (
	"flag"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/armon/consul-api"
	"github.com/cenkalti/backoff"
	dockerapi "github.com/fsouza/go-dockerclient"
)

func getopt(name, def string) string {
	if env := os.Getenv(name); env != "" {
		return env
	}
	return def
}

func assert(err error) {
	if err != nil {
		log.Fatal("docksul:", err)
	}
}

func containerServiceData(container *dockerapi.Container, prefix, key, dfault string) string {
	if prefix != "" {
		key = "SERVICE_" + prefix + "_" + key
	} else {
		key = "SERVICE_" + key
	}

	for _, env := range container.Config.Env {
		kv := strings.SplitN(env, "=", 2)

		if strings.ToLower(kv[0]) == strings.ToLower(key) {
			return kv[1]
		}
	}

	return dfault
}

type Bridge struct {
	sync.Mutex
	docker   *dockerapi.Client
	consul   *consulapi.Client
	nodeName string
	services map[string][]*consulapi.AgentServiceRegistration
}

func (b *Bridge) buildService(container *dockerapi.Container, hostPort, exposedPort, portType string, multiService bool) *consulapi.AgentServiceRegistration {
	var keyPrefix, defaultName string

	defaultName = path.Base(container.Config.Image)
	if multiService {
		keyPrefix = exposedPort
		defaultName = defaultName + "-" + exposedPort
	}

	service := new(consulapi.AgentServiceRegistration)
	service.ID = b.nodeName + "/" + container.Name[1:] + ":" + exposedPort
	service.Name = containerServiceData(container, keyPrefix, "name", defaultName)
	p, _ := strconv.Atoi(hostPort)
	service.Port = p
	service.Tags = make([]string, 0)

	if portType == "udp" {
		service.ID = service.ID + "/udp"
		service.Tags = append(service.Tags, "udp")
	}

	tags := containerServiceData(container, keyPrefix, "tags", "")
	if tags != "" {
		service.Tags = append(service.Tags, strings.Split(tags, ",")...)
	}

	return service
}

func (b *Bridge) Add(containerId string) {
	b.Lock()
	defer b.Unlock()
	container, err := b.docker.InspectContainer(containerId)
	if err != nil {
		log.Println("docksul: unable to inspect container:", containerId, err)
		return
	}

	portDefs := make([][]string, 0)
	for port, published := range container.NetworkSettings.Ports {
		if len(published) > 0 {
			p := strings.Split(string(port), "/")
			portDefs = append(portDefs, []string{published[0].HostPort, p[0], p[1]})
		}
	}

	multiservice := len(portDefs) > 1
	for _, port := range portDefs {
		service := b.buildService(container, port[0], port[1], port[2], multiservice)
		err := backoff.Retry(func() error {
			return b.consul.Agent().ServiceRegister(service)
		}, backoff.NewExponentialBackOff())
		if err != nil {
			log.Println("docksul: unable to register service:", service, err)
			continue
		}
		b.services[container.ID] = append(b.services[container.ID], service)
		log.Println("docksul: added:", container.ID[:12], service.ID)
	}
}

func (b *Bridge) Remove(containerId string) {
	b.Lock()
	defer b.Unlock()
	for _, service := range b.services[containerId] {
		err := backoff.Retry(func() error {
			return b.consul.Agent().ServiceDeregister(service.ID)
		}, backoff.NewExponentialBackOff())
		if err != nil {
			log.Println("docksul: unable to deregister service:", service.ID, err)
			continue
		}
		log.Println("docksul: removed:", containerId[:12], service.ID)
	}
	delete(b.services, containerId)
}

func main() {
	flag.Parse()

	consulConfig := consulapi.DefaultConfig()
	if flag.Arg(0) != "" {
		consulConfig.Address = flag.Arg(0)
	}
	consul, err := consulapi.NewClient(consulConfig)
	assert(err)

	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///var/run/docker.sock"))
	assert(err)

	log.Println("docksul: Getting Consul nodename...")
	var nodeName string
	err = backoff.Retry(func() (e error) {
		nodeName, e = consul.Agent().NodeName()
		if e != nil {
			log.Println(e)
		}
		return
	}, backoff.NewExponentialBackOff())
	assert(err)

	bridge := &Bridge{
		docker:   docker,
		consul:   consul,
		nodeName: nodeName,
		services: make(map[string][]*consulapi.AgentServiceRegistration),
	}

	containers, err := docker.ListContainers(dockerapi.ListContainersOptions{})
	assert(err)
	for _, listing := range containers {
		bridge.Add(listing.ID[:12])
	}

	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))
	log.Println("docksul: Listening for Docker events...")
	for msg := range events {
		switch msg.Status {
		case "start":
			go bridge.Add(msg.ID)
		case "die":
			go bridge.Remove(msg.ID)
		}
	}

	log.Fatal("docksul: docker event loop closed") // todo: reconnect?
}
