package consul

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "consul")
}

func (r *ConsulAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.Origin.HostIP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", service.Origin.HostPort, -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	servicePrefix := uri.Query()["prefix"]

	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consul: ", uri.Scheme)
	}
	if(len(servicePrefix)>0) {
		return &ConsulAdapter{client: client, servicePrefix: servicePrefix[0]}
	} else {
		return &ConsulAdapter{client: client}

	}
}

type ConsulAdapter struct {
	client        *consulapi.Client
	servicePrefix string
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *ConsulAdapter) Ping() error {
	status := r.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}
	log.Println("consul: current leader ", leader)

	return nil
}

func (r *ConsulAdapter) Register(service *bridge.Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service)
	if r.servicePrefix != "" {
		kv := r.client.KV()
		for k, v := range service.Attrs {
			pair := &consulapi.KVPair{Key: r.servicePrefix + "/attributes/" + service.ID + "/" + k, Value: []byte(v)}
			_, err := kv.Put(pair, nil)
			if err != nil {
				panic(err)
			}
		}
		if(service.Origin.ContainerID!="") {
			pair := &consulapi.KVPair{Key: r.servicePrefix + "/container/" + service.Origin.ContainerID, Value: []byte(service.ID)}
//			pair := &consulapi.KVPair{Key: r.servicePrefix + "/container/" + service.ID + "/" + service.Origin.ContainerID, Value: []byte(service.Origin.ContainerID)}
			_, err := kv.Put(pair, nil)
			if err != nil {
				panic(err)
			}
		}
	}
	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if path := service.Attrs["check_http"]; path != "" {
		check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd)
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Script = r.interpolateService(script, service)
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else {
		return nil
	}
	if check.Script != "" || check.HTTP != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	return check
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	//	pair := &consulapi.KVPair{Key: "service_attribute" + "/" + service.Name + "/" + k, Value: []byte(v)}
	success := r.client.Agent().ServiceDeregister(service.ID)
	r.client.KV().DeleteTree("service_attribute"+"/attributes/"+service.ID, nil)
	return success
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
	return nil
}
