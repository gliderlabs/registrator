package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"net"

	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/gliderlabs/pkg/usage"
	"github.com/gliderlabs/registrator/bridge"
	"github.com/docker/docker/api/types/swarm"
)

var Version string

var versionChecker = usage.NewChecker("registrator", Version)

var hostIp = flag.String("ip", "", "IP for ports mapped to the host")
var internal = flag.Bool("internal", false, "Use internal ports instead of published ones")
var explicit = flag.Bool("explicit", false, "Only register services which have SERVICE_NAME label set")
var useIpFromLabel = flag.String("useIpFromLabel", "", "Use IP which is stored in a label assigned to the container")
var refreshInterval = flag.Int("ttl-refresh", 0, "Frequency with which service TTLs are refreshed")
var refreshTtl = flag.Int("ttl", 0, "TTL for services (default is no expiry)")
var forceTags = flag.String("tags", "", "Append tags for all registered services")
var resyncInterval = flag.Int("resync", 0, "Frequency with which services are resynchronized")
var deregister = flag.String("deregister", "always", "Deregister exited services \"always\" or \"on-success\"")
var retryAttempts = flag.Int("retry-attempts", 0, "Max retry attempts to establish a connection with the backend. Use -1 for infinite retries")
var retryInterval = flag.Int("retry-interval", 2000, "Interval (in millisecond) between retry-attempts.")
var cleanup = flag.Bool("cleanup", false, "Remove dangling services")
var swarmReplicasAware = flag.Bool("swarm-replicas-aware", true, "Remove registered swarm services without replicas")
var swarmManagerSvcName = flag.String("swarm-manager-servicename", "", "Register swarm manager service when non-empty")

func getopt(name, def string) string {
	if env := os.Getenv(name); env != "" {
		return env
	}
	return def
}

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		versionChecker.PrintVersion()
		os.Exit(0)
	}
	log.Printf("Starting registratorv2 %s ...", Version)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s [options] <registry URI>\n\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 1 {
		if flag.NArg() == 0 {
			fmt.Fprint(os.Stderr, "Missing required argument for registry URI.\n\n")
		} else {
			fmt.Fprintln(os.Stderr, "Extra unparsed arguments:")
			fmt.Fprintln(os.Stderr, " ", strings.Join(flag.Args()[1:], " "))
			fmt.Fprint(os.Stderr, "Options should come before the registry URI argument.\n\n")
		}
		flag.Usage()
		os.Exit(2)
	}

	if *hostIp != "" {
		addr := net.ParseIP(*hostIp)

		// maybe -ip option references an interface name
		if addr == nil {
				ip := ipAddressForInterfaceName(*hostIp)
				if ip != "" {
						*hostIp = ip
				} else {
					log.Printf("Ignoring option -ip=%s as it references no valid ip address or interface name", *hostIp)
					*hostIp = ""
				}
		}
	}

	if (*refreshTtl == 0 && *refreshInterval > 0) || (*refreshTtl > 0 && *refreshInterval == 0) {
		assert(errors.New("-ttl and -ttl-refresh must be specified together or not at all"))
	} else if *refreshTtl > 0 && *refreshTtl <= *refreshInterval {
		assert(errors.New("-ttl must be greater than -ttl-refresh"))
	}

	if *retryInterval <= 0 {
		assert(errors.New("-retry-interval must be greater than 0"))
	}

	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		os.Setenv("DOCKER_HOST", "unix:///tmp/docker.sock")
	}

	docker, err := dockerapi.NewClientFromEnv()
	assert(err)

	if *deregister != "always" && *deregister != "on-success" {
		assert(errors.New("-deregister must be \"always\" or \"on-success\""))
	}

	// use docker info to determine node id that will be used as prefix to service id
	dockerInfo, err := docker.Info()
	assert(err)

	nodeId := new(string)
	// docker host name normally is hostname
	*nodeId = dockerInfo.Name

	if dockerInfo.Swarm.LocalNodeState != "" && dockerInfo.Swarm.LocalNodeState != swarm.LocalNodeStateInactive {
		if *hostIp == "" {
			// in case of swarm mode, docker host has information about ip
			// although it won't be always useful, we can use it if not provided by user
			*hostIp = dockerInfo.Swarm.NodeAddr
		}

		log.Printf("Docker host in Swarm Mode: %s (%s)", *nodeId, *hostIp)

	} else {
		if *hostIp != "" {
			log.Printf("Docker host: %s (%s)", *nodeId, *hostIp)
		} else {
			log.Printf("Docker host: %s", *nodeId)
		}
	}

	b, err := bridge.New(docker, flag.Arg(0), bridge.Config{
		NodeId:              *nodeId,
		HostIp:              *hostIp,
		Internal:            *internal,
		Explicit:            *explicit,
		UseIpFromLabel:      *useIpFromLabel,
		ForceTags:           *forceTags,
		RefreshTtl:          *refreshTtl,
		RefreshInterval:     *refreshInterval,
		DeregisterCheck:     *deregister,
		Cleanup:             *cleanup,
		SwarmReplicasAware:  *swarmReplicasAware,
		SwarmManagerSvcName: *swarmManagerSvcName,
	})

	assert(err)

	attempt := 0
	for *retryAttempts == -1 || attempt <= *retryAttempts {
		log.Printf("Connecting to backend (%v/%v)", attempt, *retryAttempts)

		err = b.Ping()
		if err == nil {
			break
		}

		if err != nil && attempt == *retryAttempts {
			assert(err)
		}

		time.Sleep(time.Duration(*retryInterval) * time.Millisecond)
		attempt++
	}

	// Start event listener before listing containers to avoid missing anything
	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))
	log.Println("Listening for Docker events ...")

	b.Sync(false)

	quit := make(chan struct{})

	// Start the TTL refresh timer
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

	// Start the resync timer if enabled
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
		switch msg.Type {
			case "container": {
				switch msg.Action {
					case "start": {
						log.Printf("event: container %s started", msg.Actor.ID)
						go b.Add(msg.Actor.ID)
					}
					case "die": {
						log.Printf("event: container %s died", msg.Actor.ID)
						go b.RemoveOnExit(msg.Actor.ID)
					}
					case "stop": {
						log.Printf("event: container %s stopped", msg.Actor.ID)
						go b.RemoveOnExit(msg.Actor.ID)
					}
					case "kill": {
						log.Printf("event: container %s killed", msg.Actor.ID)
						go b.RemoveOnExit(msg.Actor.ID)
					}
					default: {
						log.Printf("event: %s %s %s", msg.Type, msg.Action, msg.Actor.ID)
					}
				}
			}
			case "service": {
				switch msg.Action {
					case "create": {
						log.Printf("event: swarm service %s created", msg.Actor.ID)
						go b.RegisterSwarmServiceById(msg.Actor.ID)
					}
					case "update": {
						log.Printf("event: swarm service %s updated", msg.Actor.ID)
						go b.UpdateSwarmServiceById(msg.Actor.ID)
					}
					case "remove": {
						log.Printf("event: swarm service %s removed", msg.Actor.ID)
						go b.DeregisterSwarmServiceById(msg.Actor.ID)
					}
					default: {
						log.Printf("event: %s %s %s", msg.Type, msg.Action, msg.Actor.ID)
					}
				}
			}
			case "node": {
				switch msg.Action {
					default: {
						log.Printf("event: %s %s %s", msg.Type, msg.Action, msg.Actor.ID)
						go b.SyncSwarmServices()
					}
				}
			}
		}
	}

	close(quit)
	log.Fatal("Docker event loop closed") // todo: reconnect?
}

/**
 * Get IPv4 address for the given interface name. Returns empty string when interface
 * not found or no IPv4 address is assigned to that interface.
 **/
func ipAddressForInterfaceName(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}

	addrs, err := iface.Addrs()

	if err != nil {
		return ""
	}

	for _, a := range addrs {
		switch v := a.(type) {
		case *net.IPNet:
				if v.IP.To4() != nil {
					return v.IP.String()
				}
		case *net.IPAddr:
			if v.IP.To4() != nil {
				return v.IP.String()
			}
		default:
				continue
		}
	}

	return ""
}
