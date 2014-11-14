package main

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/coreos/go-etcd/etcd"
)

type Skydns2Registry struct {
	client *etcd.Client
	path   string
}

func NewSkydns2Registry(uri *url.URL) ServiceRegistry {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}

	return &Skydns2Registry{client: etcd.NewClient(urls), path: domainPath(uri.Path[1:])}
}

func (r *Skydns2Registry) Register(service *Service) error {
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	_, err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	return err
}

func (r *Skydns2Registry) Deregister(service *Service) error {
	_, err := r.client.Delete(r.servicePath(service), false)
	return err
}

func (r *Skydns2Registry) Refresh(service *Service) error {
	return r.Register(service)
}

func (r *Skydns2Registry) servicePath(service *Service) string {
	return r.path + "/" + service.Name + "/" + strings.Replace(service.ID, ":", "-", -1)
}

func domainPath(domain string) string {
	components := strings.Split(domain, ".")
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return "/skydns/" + strings.Join(components, "/")
}
