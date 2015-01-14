package main

import (
	"log"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/armon/consul-api"
)

const DefaultInterval = "10s"

type ConsulRegistry struct {
	client *consulapi.Client
	path   string
}

func NewConsulRegistry(uri *url.URL) ServiceRegistry {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	assert(err)
	return &ConsulRegistry{client: client, path: uri.Path}
}

func (r *ConsulRegistry) Register(service *Service) error {
	if r.path == "" || r.path == "/" {
		return r.registerWithCatalog(service)
	} else {
		return r.registerWithKV(service)
	}
}

func (r *ConsulRegistry) registerWithCatalog(service *Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Check = r.buildCheck(service)

	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulRegistry) buildCheck(service *Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if path := service.Attrs["check_http"]; path != "" {
		check.Script = fmt.Sprintf("check-http %s %s %s", service.pp.Container.ID[:12], service.pp.ExposedPort, path)
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.pp.Container.ID[:12], service.pp.ExposedPort, cmd)
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

func (r *ConsulRegistry) registerWithKV(service *Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.KV().Put(&consulapi.KVPair{Key: path, Value: []byte(addr)}, nil)
	if err != nil {
		log.Println("registrator: consul: failed to register service:", err)
	}
	return err
}

func (r *ConsulRegistry) Deregister(service *Service) error {
	if r.path == "" || r.path == "/" {
		return r.deregisterWithCatalog(service)
	} else {
		return r.deregisterWithKV(service)
	}
}

func (r *ConsulRegistry) Refresh(service *Service) error {
	return errors.New("consul backend does not support refresh (use a TTL health check instead)")
}

func (r *ConsulRegistry) deregisterWithCatalog(service *Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulRegistry) deregisterWithKV(service *Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	_, err := r.client.KV().Delete(path, nil)
	if err != nil {
		log.Println("registrator: consul: failed to register service:", err)
	}
	return err
}
