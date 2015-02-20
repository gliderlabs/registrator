package consul

import (
	"log"
	"net"
	"net/url"
	"strconv"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

func init() {
	bridge.Register(new(Factory), "consulkv")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.ServiceRegistry {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consulkv: ", uri.Scheme)
	}
	return &ConsulRegistry{client: client, path: uri.Path}
}

type ConsulRegistry struct {
	client *consulapi.Client
	path   string
}

func (r *ConsulRegistry) Ping() error {
	return nil // TODO
}

func (r *ConsulRegistry) Register(service *bridge.Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.KV().Put(&consulapi.KVPair{Key: path, Value: []byte(addr)}, nil)
	if err != nil {
		log.Println("consulkv: failed to register service:", err)
	}
	return err
}

func (r *ConsulRegistry) Deregister(service *bridge.Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	_, err := r.client.KV().Delete(path, nil)
	if err != nil {
		log.Println("consulkv: failed to deregister service:", err)
	}
	return err
}

func (r *ConsulRegistry) Refresh(service *bridge.Service) error {
	return nil
}
