package main // import "github.com/progrium/registrator"

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	dockerapi "github.com/fsouza/go-dockerclient"
)

var Version string

var hostIp = flag.String("ip", "", "IP for ports mapped to the host")
var internal = flag.Bool("internal", false, "Use internal ports instead of published ones")
var refreshInterval = flag.Int("ttl-refresh", 0, "Frequency with which service TTLs are refreshed")
var refreshTtl = flag.Int("ttl", 0, "TTL for services (default is no expiry)")
var forceTags = flag.String("tags", "", "Append tags for all registered services")
var serviceRefreshInterval = flag.Int("refresh", 0, "Frequency with which services are reregistered")

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
	Refresh(service *Service) error
}

func NewServiceRegistry(uri *url.URL) ServiceRegistry {
	factory := map[string]func(*url.URL) ServiceRegistry{
		"consul":  NewConsulRegistry,
		"etcd":    NewEtcdRegistry,
		"skydns2": NewSkydns2Registry,
	}[uri.Scheme]
	if factory == nil {
		log.Fatal("unrecognized registry backend: ", uri.Scheme)
	}
	log.Println("registrator: Using", uri.Scheme, "registry backend at", uri)
	return factory(uri)
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(Version)
		os.Exit(0)
	}

	flag.Parse()

	if *hostIp != "" {
		log.Println("registrator: Forcing host IP to", *hostIp)
	}
	if (*refreshTtl == 0 && *refreshInterval > 0) || (*refreshTtl > 0 && *refreshInterval == 0) {
		assert(errors.New("-ttl and -ttl-refresh must be specified together or not at all"))
	} else if *refreshTtl > 0 && *refreshTtl <= *refreshInterval {
		assert(errors.New("-ttl must be greater than -ttl-refresh"))
	}

	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///tmp/docker.sock"))
	assert(err)

	uri, err := url.Parse(flag.Arg(0))
	assert(err)
	registry := NewServiceRegistry(uri)

	bridge := &RegistryBridge{
		docker:   docker,
		registry: registry,
		services: make(map[string][]*Service),
	}

	// Start event listener before listing containers to avoid missing anything
	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))
	log.Printf("registrator %s listening for Docker events...\n", Version)

	// List already running containers
	containers, err := docker.ListContainers(dockerapi.ListContainersOptions{})
	assert(err)
	for _, listing := range containers {
		bridge.Add(listing.ID)
	}

	// Start the TTL refresh timer
	quit := make(chan struct{})
	if *refreshInterval > 0 {
		ticker := time.NewTicker(time.Duration(*refreshInterval) * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					bridge.Refresh()
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
	}

	if *serviceRefreshInterval > 0 {
		refreshTicker := time.NewTicker(time.Duration(*serviceRefreshInterval) * time.Second)
		go func() {
			for {
				select {
				case <-refreshTicker.C:
					bridge.ResubmitAll()
				case <-quit:
					refreshTicker.Stop()
				}
			}
		}()
	}

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			go bridge.Add(msg.ID)
		case "stop":
			go bridge.Remove(msg.ID)
		case "die":
			go bridge.Remove(msg.ID)
		}
	}

	close(quit)
	log.Fatal("registrator: docker event loop closed") // todo: reconnect?
}
