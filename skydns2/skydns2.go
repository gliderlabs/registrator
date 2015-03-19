package skydns2

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/gliderlabs/registrator/bridge"
	"github.com/gliderlabs/registrator/etcd"
)

func init() {
	bridge.Register(new(Factory), "skydns2")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	client, err := etcd.NewEtcdClient(uri.Host)
	if err != nil {
		log.Fatal("skydns2: can't allocate client:", err)
	}

	if len(uri.Path) < 2 {
		log.Fatal("skydns2: dns domain required e.g.: skydns2://<host>/<domain>")
	}

	return &Skydns2Adapter{client: client, path: domainPath(uri.Path[1:])}
}

type Skydns2Adapter struct {
	client *etcd.EtcdClient
	path   string
}

func (r *Skydns2Adapter) Ping() error {
	return r.client.Ping()
}

func (r *Skydns2Adapter) Register(service *bridge.Service) error {
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Deregister(service *bridge.Service) error {
	err := r.client.Delete(r.servicePath(service), false)
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
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
