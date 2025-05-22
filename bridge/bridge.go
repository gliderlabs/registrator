package bridge

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	jsonp "github.com/buger/jsonparser"
	dockerapi "github.com/fsouza/go-dockerclient"
)

var serviceIDPattern = regexp.MustCompile(`^(.+?):([a-zA-Z0-9][a-zA-Z0-9_.-]+):[0-9]+(?::udp)?$`)

type Bridge struct {
	sync.Mutex
	registry       RegistryAdapter
	docker         *dockerapi.Client
	services       map[string][]*Service
	deadContainers map[string]*DeadContainer
	config         Config
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
		docker:         docker,
		config:         config,
		registry:       factory.New(uri),
		services:       make(map[string][]*Service),
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
	if err != nil && quiet {
		log.Println("error listing containers, skipping sync")
		return
	} else if err != nil && !quiet {
		log.Fatal(err)
	}

	log.Printf("Syncing services on %d containers", len(containers))

	// NOTE: This assumes reregistering will do the right thing, i.e. nothing..
	for _, listing := range containers {
		services := b.services[listing.ID]
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
		for listingId := range b.services {
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
			matches := serviceIDPattern.FindStringSubmatch(extService.ID)
			if len(matches) != 3 {
				// There's no way this was registered by us, so leave it
				continue
			}
			serviceHostname := matches[1]
			if serviceHostname != Hostname {
				// ignore because registered on a different host
				continue
			}
			serviceContainerName := matches[2]
			for _, listing := range b.services {
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

	ports := make(map[string]ServicePort)

	// Extract configured host port mappings, relevant when using --net=host
	for port := range container.Config.ExposedPorts {
		published := []dockerapi.PortBinding{{HostIP: "0.0.0.0", HostPort: port.Port()}}
		ports[string(port)] = servicePort(container, port, published)
	}

	// Extract runtime port mappings, relevant when using --net=bridge
	for port, published := range container.NetworkSettings.Ports {
		ports[string(port)] = servicePort(container, port, published)
	}

	if len(ports) == 0 && !quiet {
		log.Println("ignored:", container.ID[:12], "no published ports")
		return
	}

	servicePorts := make(map[string]ServicePort)
	for key, port := range ports {
		if !b.config.Internal && port.HostPort == "" {
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
		b.services[container.ID] = append(b.services[container.ID], service)
		log.Println("added:", container.ID[:12], service.ID)
	}
}

func (b *Bridge) newService(port ServicePort, isgroup bool) *Service {
	container := port.container
	defaultName := strings.Split(path.Base(container.Config.Image), ":")[0]

	// not sure about this logic. kind of want to remove it.
	hostname := Hostname
	if hostname == "" {
		hostname = port.HostIP
	}
	if port.HostIP == "0.0.0.0" {
		ip, err := net.ResolveIPAddr("ip", hostname)
		if err == nil {
			port.HostIP = ip.String()
		}
	}

	if b.config.HostIp != "" {
		port.HostIP = b.config.HostIp
	}

	metadata, metadataFromPort := serviceMetaData(container.Config, port.ExposedPort)

	ignore := mapDefault(metadata, "ignore", "")
	if ignore != "" {
		return nil
	}

	serviceName := mapDefault(metadata, "name", "")
	if serviceName == "" {
		if b.config.Explicit {
			return nil
		}
		serviceName = defaultName
	}

	service := new(Service)
	service.Origin = port
	service.ID = hostname + ":" + container.Name[1:] + ":" + port.ExposedPort
	service.Name = serviceName
	if isgroup && !metadataFromPort["name"] {
		service.Name += "-" + port.ExposedPort
	}
	var p int

	if b.config.Internal {
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
				b.config.UseIpFromLabel + "'")
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

	// Use container inspect data to populate tags list
	// https://github.com/fsouza/go-dockerclient/blob/master/container.go#L441-L483
	ForceTags := b.config.ForceTags
	if len(ForceTags) != 0 {
		// Template functions
		fm := template.FuncMap{
			// Template function name: strSlice
			// Description: Slice string from start to end (same as s[start:end] where s represents string).
			//
			// Usage: strSlice s start end
			//
			// Example: strSlice .ID 0 12
			// {
			//     "Id": "e20f9c1a76565d62ae24a3bb877b17b862b6eab94f4e95a0e07ccf25087aaf4f"
			// }
			// Output: "e20f9c1a7656"
			//
			"strSlice": func(v string, i ...int) string {
				if len(i) == 1 {
					if len(v) >= i[0] {
						return v[i[0]:]
					}
				}

				if len(i) == 2 {
					if len(v) >= i[0] && len(v) >= i[1] {
						if i[0] == 0 {
							return v[:i[1]]
						}
						if i[1] < i[0] {
							return v[i[0]:]
						}
						return v[i[0]:i[1]]
					}
				}

				return v
			},
			// Template function name: sIndex
			// Description: Return element from slice or array s by specifiying index i (same as s[i] where s represents slice or array - index i can also take negative values to extract elements in reverse order).
			//
			// Usage: sIndex i s
			//
			// Example: sIndex 0 .Config.Env
			// {
			//     "Config": {
			//         "Env": [
			//             "ENVIRONMENT=test",
			//             "SERVICE_8105_NAME=foo",
			//             "HOME=/home/foobar",
			//             "SERVICE_9404_NAME=bar"
			//         ]
			//     }
			// }
			// Output: "ENVIRONMENT=test"
			//
			"sIndex": func(i int, s []string) string {
				if i < 0 {
					i = i * -1
					if i >= len(s) {
						return s[0]
					}
					return s[len(s)-i]
				}

				if i >= len(s) {
					return s[len(s)-1]
				}

				return s[i]
			},
			// Template function name: mIndex
			// Description: Return value for key k stored in the map m (same as m["k"]).
			//
			// Usage: mIndex k m
			//
			// Example: mIndex "com.amazonaws.ecs.task-arn" .Config.Labels
			// {
			//     "Config": {
			//         "Labels": {
			//             "com.amazonaws.ecs.task-arn": "arn:aws:ecs:region:xxxxxxxxxxxx:task/368f4403-0ee4-4f4c-b7a5-be50c57db5cf"
			//         }
			//     }
			// }
			// Output: "arn:aws:ecs:region:xxxxxxxxxxxx:task/368f4403-0ee4-4f4c-b7a5-be50c57db5cf"
			//
			"mIndex": func(k string, m map[string]string) string {
				return m[k]
			},
			// Template function name: toUpper
			// Description: Return s with all letters mapped to their upper case.
			//
			// Usage: toUpper s
			//
			// Example: toUpper "foo"
			// Output: "FOO"
			//
			"toUpper": func(v string) string {
				return strings.ToUpper(v)
			},
			// Template function name: toLower
			// Description: Return s with all letters mapped to their lower case.
			//
			// Usage: toLower s
			//
			// Example: toLower "FoO"
			// Output: "foo"
			//
			"toLower": func(v string) string {
				return strings.ToLower(v)
			},
			// Template function name: replace
			// Description: Replace all (-1) or first n occurrences of "old" with "new" found in the designated string s.
			//
			// Usage: replace n old new s
			//
			// Example: replace -1 "=" "" "=foo="
			// Output: "foo"
			//
			"replace": func(n int, old, new, v string) string {
				return strings.Replace(v, old, new, n)
			},
			// Template function name: join
			// Description: Create a single string from all the elements found in the slice s where sep will be used as separator.
			//
			// Usage: join sep s
			//
			// Example: join "," .Config.Env
			// {
			//     "Config": {
			//         "Env": [
			//             "ENVIRONMENT=test",
			//             "SERVICE_8105_NAME=foo",
			//             "HOME=/home/foobar",
			//             "SERVICE_9404_NAME=bar"
			//         ]
			//     }
			// }
			// Output: "ENVIRONMENT=test,SERVICE_8105_NAME=foo,HOME=/home/foobar,SERVICE_9404_NAME=bar"
			//
			"join": func(sep string, s []string) string {
				return strings.Join(s, sep)
			},
			// Template function name: split
			// Description: Split string s into all substrings separated by sep and return a slice of the substrings between those separators.
			//
			// Usage: split sep s
			//
			// Example: split "," "/proc/bus,/proc/fs,/proc/irq"
			// Output: [/proc/bus /proc/fs /proc/irq]
			//
			"split": func(sep, v string) []string {
				return strings.Split(v, sep)
			},
			// Template function name: splitIndex
			// Description: split and sIndex function combined, index i can also take negative values to extract elements in reverse order.
			//				Same result can be achieved if using pipeline with both functions: {{ split sep s | sIndex i }}
			//
			// Usage: splitIndex i sep s
			//
			// Example: splitIndex -1 "/" "arn:aws:ecs:region:xxxxxxxxxxxx:task/368f4403-0ee4-4f4c-b7a5-be50c57db5cf"
			// Output: "368f4403-0ee4-4f4c-b7a5-be50c57db5cf"
			//
			"splitIndex": func(i int, sep, v string) string {
				l := strings.Split(v, sep)

				if i < 0 {
					i = i * -1
					if i >= len(l) {
						return l[0]
					}
					return l[len(l)-i]
				}

				if i >= len(l) {
					return l[len(l)-1]
				}

				return l[i]
			},
			// Template function name: matchFirstElement
			// Description: Iterate through slice s and return first element that match regex expression.
			//
			// Usage: matchFirstElement regex s
			//
			// Example: matchFirstElement "^SERVICE_" .Config.Env
			// {
			//     "Config": {
			//         "Env": [
			//             "ENVIRONMENT=test",
			//             "SERVICE_8105_NAME=foo",
			//             "HOME=/home/foobar",
			//             "SERVICE_9404_NAME=bar"
			//         ]
			//     }
			// }
			// Output: "SERVICE_8105_NAME=foo"
			//
			"matchFirstElement": func(r string, s []string) string {
				var m string

				re := regexp.MustCompile(r)
				for _, e := range s {
					if re.MatchString(e) {
						m = e
						break
					}
				}

				return m
			},
			// Template function name: matchAllElements
			// Description: Iterate through slice s and return slice of all elements that match regex expression.
			//
			// Usage: matchAllElements regex s
			//
			// Example: matchAllElements "^SERVICE_" .Config.Env
			// {
			//     "Config": {
			//         "Env": [
			//             "ENVIRONMENT=test",
			//             "SERVICE_8105_NAME=foo",
			//             "HOME=/home/foobar",
			//             "SERVICE_9404_NAME=bar"
			//         ]
			//     }
			// }
			// Output: [SERVICE_8105_NAME=foo SERVICE_9404_NAME=bar]
			//
			"matchAllElements": func(r string, s []string) []string {
				var m []string

				re := regexp.MustCompile(r)
				for _, e := range s {
					if re.MatchString(e) {
						m = append(m, e)
					}
				}

				return m
			},
			// Template function name: httpGet
			// Description: Fetch an object from URL.
			//
			// Usage: httpGet url
			//
			// Example: httpGet "https://ajpi.me/all"
			// Output: []byte (e.g. JSON object)
			//
			"httpGet": func(url string) []byte {
				// HTTP client configuration
				c := &http.Client{
					Timeout: 10 * time.Second,
				}

				res, err := c.Get(url)
				if err != nil {
					log.Printf("httpGet template function encountered an error while executing HTTP request: %v", err)
					return []byte("")
				}

				body, err := ioutil.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					log.Printf("httpGet template function encountered an error while reading HTTP body payload: %v", err)
					return []byte("")
				}

				return body
			},
			// Template function name: jsonParse
			// Description: Extract value from JSON object by specifying exact path (nested objects). Keys in path has to be separated with double colon sign.
			//
			// Usage: jsonParse b key1::key2::key3::keyN
			//
			// Example: jsonParse b "Additional::Country"
			// {
			//     "Additional": {
			//         "Country": "United States"
			//     }
			// }
			// Output: "United States"
			//
			"jsonParse": func(b []byte, k string) string {
				var (
					keys []string
					js   []byte
					err  error
				)

				keys = strings.Split(k, "::")

				js, _, _, err = jsonp.Get(b, keys...)
				if err != nil {
					log.Printf("jsonParse template function encountered an error while parsing JSON object %v: %v", keys, err)
				}

				return string(js)
			},
		}

		tmpl, err := template.New("tags").Funcs(fm).Parse(ForceTags)
		if err != nil {
			log.Fatalf("%s template parsing failed with error: %s", ForceTags, err)
		}

		var b bytes.Buffer
		err = tmpl.Execute(&b, container)
		if err != nil {
			log.Fatalf("%s template execution failed with error: %s", ForceTags, err)
		}

		ForceTags = b.String()
	}

	if port.PortType == "udp" {
		service.Tags = combineTags(
			mapDefault(metadata, "tags", ""), ForceTags, "udp")
		service.ID = service.ID + ":udp"
	} else {
		service.Tags = combineTags(
			mapDefault(metadata, "tags", ""), ForceTags)
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

var Hostname string

func init() {
	// It's ok for Hostname to ultimately be an empty string
	// An empty string will fall back to trying to make a best guess
	Hostname, _ = os.Hostname()
}
