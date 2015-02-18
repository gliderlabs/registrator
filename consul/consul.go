package consul

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

const DefaultInterval = "10s"

var UseCatalog bool // TODO: redesign this

type ConsulRegistry struct {
	client *consulapi.Client
	path   string
}

func NewConsulRegistry(uri *url.URL) bridge.ServiceRegistry {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consul: ", uri.Scheme)
	}
	return &ConsulRegistry{client: client, path: uri.Path}
}

func (r *ConsulRegistry) Register(service *bridge.Service) error {
	if r.path == "" || r.path == "/" {
		if UseCatalog {
			return r.registerWithCatalog(service)
		} else {
			return r.registerWithAgent(service)
		}
	} else {
		return r.registerWithKV(service)
	}
}

func (r *ConsulRegistry) registerWithCatalog(service *bridge.Service) error {
	writeOptions := new(consulapi.WriteOptions)
	regCatalog := new(consulapi.CatalogRegistration)
	regCatalog.Datacenter = "dc1"
	regCatalog.Node = service.Origin.HostName
	regCatalog.Address = service.IP

	regCatalog.Service = new(consulapi.AgentService)
	regCatalog.Service.ID = service.ID
	regCatalog.Service.Service = service.Name
	regCatalog.Service.Port = service.Port
	regCatalog.Service.Tags = service.Tags

	_, err := r.client.Catalog().Register(regCatalog, writeOptions)
	if err != nil {
		log.Println("registrator: consul: failed to register catalog service:", err)
	}
	return err
}

func (r *ConsulRegistry) registerWithAgent(service *bridge.Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service)

	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulRegistry) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if path := service.Attrs["check_http"]; path != "" {
		check.Script = fmt.Sprintf("check-http %s %s %s", service.Origin.Container.ID[:12], service.Origin.ExposedPort, path)
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.Container.ID[:12], service.Origin.ExposedPort, cmd)
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

func (r *ConsulRegistry) registerWithKV(service *bridge.Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.KV().Put(&consulapi.KVPair{Key: path, Value: []byte(addr)}, nil)
	if err != nil {
		log.Println("registrator: consul: failed to register service:", err)
	}
	return err
}

func (r *ConsulRegistry) Deregister(service *bridge.Service) error {
	if r.path == "" || r.path == "/" {
		if UseCatalog {
			return r.deregisterWithCatalog(service)
		} else {
			return r.deregisterWithAgent(service)
		}
	} else {
		return r.deregisterWithKV(service)
	}
}

func (r *ConsulRegistry) Refresh(service *bridge.Service) error {
	return errors.New("consul backend does not support refresh (use a TTL health check instead)")
}

func (r *ConsulRegistry) deregisterWithCatalog(service *bridge.Service) error {
	writeOptions := new(consulapi.WriteOptions)
	deregCatalog := new(consulapi.CatalogDeregistration)
	deregCatalog.Datacenter = "dc1"
	deregCatalog.Node = service.Origin.HostName
	deregCatalog.Address = service.IP
	deregCatalog.ServiceID = service.ID

	_, err := r.client.Catalog().Deregister(deregCatalog, writeOptions)
	if err != nil {
		log.Println("registrator: consul: failed to deregister catalog service:", err)
	}
	return err
}

func (r *ConsulRegistry) deregisterWithAgent(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulRegistry) deregisterWithKV(service *bridge.Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	_, err := r.client.KV().Delete(path, nil)
	if err != nil {
		log.Println("registrator: consul: failed to register service:", err)
	}
	return err
}
