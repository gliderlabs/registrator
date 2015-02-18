package skydns2

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
)

type Skydns2Registry struct {
	client *etcd.Client
	path   string
}

func NewSkydns2Registry(uri *url.URL) bridge.ServiceRegistry {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}

	return &Skydns2Registry{client: etcd.NewClient(urls), path: domainPath(uri.Path[1:])}
}

func (r *Skydns2Registry) Register(service *bridge.Service) error {
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	_, err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	if err != nil {
		log.Println("registrator: skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Registry) Deregister(service *bridge.Service) error {
	_, err := r.client.Delete(r.servicePath(service), false)
	if err != nil {
		log.Println("registrator: skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Registry) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *Skydns2Registry) servicePath(service *bridge.Service) string {
	return r.path + "/" + service.Name + "/" + service.ID
}

func domainPath(domain string) string {
	components := strings.Split(domain, ".")
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return "/skydns/" + strings.Join(components, "/")
}
