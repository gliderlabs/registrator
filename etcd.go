package main

import (
	"net"
	"net/url"
	"strconv"

	"github.com/coreos/go-etcd/etcd"
)

type EtcdRegistry struct {
	client *etcd.Client
	path   string
}

func NewEtcdRegistry(uri *url.URL) ServiceRegistry {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}
	return &EtcdRegistry{client: etcd.NewClient(urls), path: uri.Path}
}

func (r *EtcdRegistry) Register(service *Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.Create(path, addr, 0)
	return err
}

func (r *EtcdRegistry) Deregister(service *Service) error {
	path := r.path + "/" + service.Name + "/" + service.ID
	_, err := r.client.Delete(path, false)
	return err
}
