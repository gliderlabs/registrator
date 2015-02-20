package etcd

import (
	"log"
	"net"
	"net/url"
	"strconv"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "etcd")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.ServiceRegistry {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}
	return &EtcdRegistry{client: etcd.NewClient(urls), path: uri.Path}
}

type EtcdRegistry struct {
	client *etcd.Client
	path   string
}

func (r *EtcdRegistry) Ping() error {
	return nil // TODO
}

func (r *EtcdRegistry) Register(service *bridge.Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.Set(path, addr, uint64(service.TTL))
	if err != nil {
		log.Println("etcd: failed to register service:", err)
	}
	return err
}

func (r *EtcdRegistry) Deregister(service *bridge.Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	_, err := r.client.Delete(path, false)
	if err != nil {
		log.Println("etcd: failed to deregister service:", err)
	}
	return err
}

func (r *EtcdRegistry) Refresh(service *bridge.Service) error {
	return r.Register(service)
}
