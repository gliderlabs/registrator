package skydns2

import (
	"encoding/json"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "skydns2")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}

	if len(uri.Path) < 2 {
		log.Fatal("skydns2: dns domain required e.g.: skydns2://<host>/<domain>")
	}

	return &Skydns2Adapter{client: etcd.NewClient(urls), path: domainPath(uri.Path[1:])}
}

type Skydns2Adapter struct {
	client *etcd.Client
	path   string
}

func (r *Skydns2Adapter) Ping() error {
	rr := etcd.NewRawRequest("GET", "version", nil, nil)
	_, err := r.client.SendRequest(rr)
	if err != nil {
		return err
	}
	return nil
}

func (r *Skydns2Adapter) Register(service *bridge.Service) error {
	record, err := serviceRecord(service)
	if err != nil {
		log.Println("skydns2: failed to marshal service record:", err)
	}

	_, err = r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Deregister(service *bridge.Service) error {
	_, err := r.client.Delete(r.servicePath(service), false)
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *Skydns2Adapter) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}

func (r *Skydns2Adapter) servicePath(service *bridge.Service) string {
	return r.path + "/" + service.Name + "/" + service.ID
}

func domainPath(domain string) string {
	components := strings.Split(domain, ".")
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return "/skydns/" + strings.Join(components, "/")
}

func serviceRecord(service *bridge.Service) (string, error) {
	priority, _ := strconv.Atoi(service.Attrs["priority"])
	weight, _ := strconv.Atoi(service.Attrs["weight"])

	record := struct {
		Host     string `json:"host,omitempty"`
		Port     int    `json:"port,omitempty"`
		Priority int    `json:"priority,omitempty"`
		Weight   int    `json:"weight,omitempty"`
	}{
		service.IP,
		service.Port,
		priority,
		weight,
	}

	j, err := json.Marshal(&record)
	if err != nil {
		return "", err
	}

	return string(j), nil
}
