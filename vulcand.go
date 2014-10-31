package main

import (
    "net"
    "net/url"
    "strconv"

    "github.com/coreos/go-etcd/etcd"
)

type VulcandRegistry struct {
    client *etcd.Client
    path   string
}

func NewVulcandRegistry(uri *url.URL) ServiceRegistry {
    urls := make([]string, 0)
    if uri.Host != "" {
        urls = append(urls, "http://"+uri.Host)
    }
    return &VulcandRegistry{client: etcd.NewClient(urls), path: uri.Path}
}

func (r *VulcandRegistry) Register(service *Service) error {
    path := r.path + "/upstreams/" + service.Name + "/endpoints/" + service.ID
    port := strconv.Itoa(service.Port)
    addr := "http://" + net.JoinHostPort(service.IP, port)
    _, err := r.client.Set(path, addr, uint64(service.TTL))
    return err
}

func (r *VulcandRegistry) Deregister(service *Service) error {
    path := r.path + "/upstreams/" + service.Name + "/endpoints/" + service.ID
    _, err := r.client.Delete(path, false)
    return err
}

func (r *VulcandRegistry) Refresh(service *Service) error {
    return r.Register(service)
}
