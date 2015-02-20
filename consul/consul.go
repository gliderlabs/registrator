package consul

import (
	"fmt"
	"log"
	"net/url"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "consul")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consul: ", uri.Scheme)
	}
	return &ConsulAdapter{client: client}
}

type ConsulAdapter struct {
	client *consulapi.Client
}

func (r *ConsulAdapter) Ping() error {
	return nil // TODO
}

func (r *ConsulAdapter) Register(service *bridge.Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service)
	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if path := service.Attrs["check_http"]; path != "" {
		check.Script = fmt.Sprintf("check-http %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, path)
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd)
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Script = script
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else {
		return nil
	}
	if check.Script != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	return check
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
	return nil
}
