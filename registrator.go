package main

import (
	"flag"
	"log"
	"os"
	"strings"

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
		log.Fatal("registrator: ", err)
	}
}

func retry(fn func() error) error {
	return backoff.Retry(fn, backoff.NewExponentialBackOff())
}

func mapdefault(m map[string]string, key, default_ string) string {
	v, ok := m[key]
	if !ok {
		return default_
	}
	return v
}

type ServiceRegistry interface {
	Register(service *Service) error
	Deregister(service *Service) error
}

func NewServiceRegistry(uri *url.URL) ServiceRegistry {
	factory := map[string]func(*url.URL) ServiceRegistry{
		"consul": NewConsulRegistry,
		"etcd":   NewEtcdRegistry,
	}[uri.Scheme]
	if factory == nil {
		log.Fatal("unrecognized registry backend: ", uri.Scheme)
	}
	return factory(uri)
}

func main() {
	flag.Parse()

	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///var/run/docker.sock"))
	assert(err)

	registry := NewServiceRegistry(flag.Arg(0))

	bridge := &RegistryBridge{
		docker:   docker,
		registry: registry,
		services: make(map[string][]*Service),
	}

	containers, err := docker.ListContainers(dockerapi.ListContainersOptions{})
	assert(err)
	for _, listing := range containers {
		bridge.Add(listing.ID[:12])
	}

	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))
	log.Println("registrator: Listening for Docker events...")
	for msg := range events {
		switch msg.Status {
		case "start":
			go bridge.Add(msg.ID)
		case "die":
			go bridge.Remove(msg.ID)
		}
	}

	log.Fatal("registrator: docker event loop closed") // todo: reconnect?
}
