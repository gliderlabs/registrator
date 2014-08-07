package main

import (
	"net"
	"net/url"
	"strconv"

	"github.com/armon/consul-api"
)

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

	checkScript, hasCheckScript := service.Attrs["SERVICE_CHECK_SCRIPT"]
	checkInterval, hasCheckInterval := service.Attrs["SERVICE_CHECK_INTERVAL"]
	checkTTL, hasCheckTTL := service.Attrs["SERVICE_CHECK_TTL"]

	if hasCheckScript && hasCheckTTL {
		// TODO: log/raise an error about script and ttl being exclusive?
	} else if hasCheckScript || hasCheckTTL {
		check := new(consulapi.AgentServiceCheck)
		if hasCheckScript {
			check.Script = checkScript
			if hasCheckInterval {
				check.Interval = checkInterval
			} else {
				check.Interval = "10s"
			}
		} else {
			check.TTL = checkTTL
		}
		registration.Check = check
	}
	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulRegistry) registerWithKV(service *Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.KV().Put(&consulapi.KVPair{Key: path, Value: []byte(addr)}, nil)
	return err
}

func (r *ConsulRegistry) Deregister(service *Service) error {
	if r.path == "" || r.path == "/" {
		return r.deregisterWithCatalog(service)
	} else {
		return r.deregisterWithKV(service)
	}
}

func (r *ConsulRegistry) deregisterWithCatalog(service *Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulRegistry) deregisterWithKV(service *Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	_, err := r.client.KV().Delete(path, nil)
	return err
}
