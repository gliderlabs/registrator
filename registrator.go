package main // import "github.com/gliderlabs/registrator"

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	dockerapi "github.com/fsouza/go-dockerclient"

	"github.com/gliderlabs/registrator/bridge"
	"github.com/gliderlabs/registrator/consul"
	"github.com/gliderlabs/registrator/etcd"
	"github.com/gliderlabs/registrator/skydns2"
)

var Version string

var hostIp = flag.String("ip", "", "IP for ports mapped to the host")
var internal = flag.Bool("internal", false, "Use internal ports instead of published ones")
var refreshInterval = flag.Int("ttl-refresh", 0, "Frequency with which service TTLs are refreshed")
var refreshTtl = flag.Int("ttl", 0, "TTL for services (default is no expiry)")
var forceTags = flag.String("tags", "", "Append tags for all registered services")
var resyncInterval = flag.Int("resync", 0, "Frequency with which services are resynchronized")

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

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(Version)
		os.Exit(0)
	}
	log.Printf("Starting registrator %s\n", Version)

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

	consul.UseCatalog = *internal // temporary hack for Consul
	b := bridge.New(docker, bridge.Config{
		HostIp:          *hostIp,
		Internal:        *internal,
		ForceTags:       *forceTags,
		RefreshTtl:      *refreshTtl,
		RefreshInterval: *refreshInterval,
	})

	uri, err := url.Parse(flag.Arg(0))
	assert(err)
	factory := map[string]func(*url.URL) bridge.ServiceRegistry{
		"consul":  consul.NewConsulRegistry,
		"etcd":    etcd.NewEtcdRegistry,
		"skydns2": skydns2.NewSkydns2Registry,
	}[uri.Scheme]
	if factory == nil {
		log.Fatal("unrecognized registry backend: ", uri.Scheme)
	}
	b.Registry = factory(uri)
	log.Println("registrator: Using", uri.Scheme, "registry backend at", uri)

	// Start event listener before listing containers to avoid missing anything
	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))
	log.Println("registrator: Listening for Docker events...")

	b.Sync(false)

	// Start the TTL refresh timer
	quit := make(chan struct{})
	if *refreshInterval > 0 {
		ticker := time.NewTicker(time.Duration(*refreshInterval) * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					b.Refresh()
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
	}

	if *resyncInterval > 0 {
		resyncTicker := time.NewTicker(time.Duration(*resyncInterval) * time.Second)
		go func() {
			for {
				select {
				case <-resyncTicker.C:
					b.Sync(true)
				case <-quit:
					resyncTicker.Stop()
					return
				}
			}
		}()
	}

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			go b.Add(msg.ID)
		case "stop":
			go b.Remove(msg.ID)
		case "die":
			go b.Remove(msg.ID)
		}
	}

	close(quit)
	log.Fatal("registrator: docker event loop closed") // todo: reconnect?
}
