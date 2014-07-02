package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/fsouza/go-dockerclient"
)

func debug(v ...interface{}) {
	if os.Getenv("DEBUG") != "" {
		log.Println(v...)
	}
}

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func marshal(obj interface{}) []byte {
	bytes, err := json.Marshal(obj)
	if err != nil {
		log.Println("marshal:", err)
	}
	return bytes
}

func containerEnv(container *docker.Container, prefix, key, dfault string) string {
	if prefix != "" {
		key = "consul_" + prefix + "_" + key
	} else {
		key = "consul_" + key
	}

	for _, env := range container.Config.Env {
		kv := strings.SplitN(env, "=", 2)

		if strings.ToLower(kv[0]) == strings.ToLower(key) {
			return kv[1]
		}
	}

	return dfault
}

func makeService(container *docker.Container, hostPort, exposedPort, portType string, multiService bool) map[string]interface{} {
	var keyPrefix, defaultName string

	if multiService {
		keyPrefix = exposedPort
		defaultName = container.Name[1:] + "-" + exposedPort
	} else {
		defaultName = container.Name[1:]
	}

	service := make(map[string]interface{})
	service["Name"] = containerEnv(container, keyPrefix, "name", defaultName)
	p, _ := strconv.Atoi(hostPort)
	service["Port"] = p
	service["Tags"] = make([]string, 0)

	if portType == "udp" {
		service["Tags"] = append(service["Tags"].([]string), "udp")
	}

	tags := containerEnv(container, keyPrefix, "tags", "")
	if tags != "" {
		service["Tags"] = append(service["Tags"].([]string), strings.Split(tags, ",")...)
	}

	// allow multiple instances of a service per host by passing
	// the container Id as consul service id
	service["ID"] = keyPrefix + "-" + container.ID[:12]

	return service
}

type ContainerServiceBridge struct {
	dockerClient *docker.Client
	consulAddr   string
	linked       map[string][]*Linked
}

type Linked struct {
	Name     string
	ConsulId string
}

func NewLinked(name string, id string) *Linked {
	return &Linked{Name: name, ConsulId: id}
}

func (b *ContainerServiceBridge) register(service map[string]interface{}) {
	url := b.consulAddr + "/v1/agent/service/register"
	body := bytes.NewBuffer(marshal(service))
	req, err := http.NewRequest("PUT", url, body)

	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")
	_, err = http.DefaultClient.Do(req)

	if err != nil {
		panic(err)
	}
}

func (b *ContainerServiceBridge) deregister(serviceId string) {
	url := b.consulAddr + "/v1/agent/service/deregister/" + serviceId
	_, err := http.DefaultClient.Get(url)
	if err != nil {
		panic(err)
	}
}

func (b *ContainerServiceBridge) Link(containerId string) {
	container, err := b.dockerClient.InspectContainer(containerId)
	assert(err)

	portDefs := make([][]string, 0)
	for port, published := range container.NetworkSettings.Ports {
		if len(published) > 0 {
			p := strings.Split(string(port), "/")
			portDefs = append(portDefs, []string{published[0].HostPort, p[0], p[1]})
		}
	}

	multiservice := len(portDefs) > 1
	for _, port := range portDefs {
		service := makeService(container, port[0], port[1], port[2], multiservice)
		b.register(service)
		b.linked[container.ID] = append(b.linked[container.ID], NewLinked(service["Name"].(string), service["ID"].(string)))
		log.Println("link:", container.ID[:12], service)
	}
}

func (b *ContainerServiceBridge) Unlink(containerId string) {
	for _, linked := range b.linked[containerId] {
		b.deregister(linked.ConsulId)
		log.Println("unlink:", containerId[:12], linked.Name)
	}

	delete(b.linked, containerId)
}

func main() {
	flag.Parse()

	consulAddr := flag.Arg(0)
	if consulAddr == "" {
		consulAddr = "http://0.0.0.0:8500"
	}
	dockerAddr := flag.Arg(1)
	if dockerAddr == "" {
		dockerAddr = "unix:///var/run/docker.sock"
	}

	client, err := docker.NewClient(dockerAddr)
	assert(err)

	bridge := &ContainerServiceBridge{client, consulAddr, make(map[string][]*Linked)}

	containers, err := client.ListContainers(docker.ListContainersOptions{})
	assert(err)
	for _, listing := range containers {
		bridge.Link(listing.ID[:12])
	}

	events := make(chan *docker.APIEvents)
	assert(client.AddEventListener(events))

	for msg := range events {
		debug("event:", msg.ID[:12], msg.Status)

		switch msg.Status {
		case "start":
			go bridge.Link(msg.ID)
		case "die":
			go bridge.Unlink(msg.ID)
		}
	}

	log.Fatal("docker event loop closed") // todo: loop?
}
