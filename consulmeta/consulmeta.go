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
	bridge.Register(new(Factory), "consulmeta")
}

func (r *ConsulMetaAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.Origin.HostIP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", service.Origin.HostPort, -1)
	return withPort
}

type Factory struct{}

type ConsulMetaAdapter struct {
	client *consulapi.Client
	path   string
}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consul: ", uri.Scheme)
	}
	return &ConsulMetaAdapter{client: client, path: uri.Path}
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *ConsulMetaAdapter) Ping() error {
	status := r.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}
	log.Println("consul: current leader ", leader)

	return nil
}

func (r *ConsulMetaAdapter) Register(service *bridge.Service) error {

	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service)

	basePath := r.path[1:] + "/" + service.Name + "/" + service.ID

	for key, value := range service.Attrs {
		path := basePath + "/" + key
		_, err := r.client.KV().Put(&consulapi.KVPair{Key: path, Value: []byte(value)}, nil)
		if err != nil {
			log.Println("consulmeta: failed to register k/v for attribute ["+path+"]:", err)
		}
	}

	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulMetaAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
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

func (r *ConsulMetaAdapter) Deregister(service *bridge.Service) error {

	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	_, err := r.client.KV().DeleteTree(path, nil)

	if err != nil {
		log.Println("consulmeta: failed to delete k/v for ["+path+"]:", err)
	}

	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulMetaAdapter) Refresh(service *bridge.Service) error {
	return nil
}
