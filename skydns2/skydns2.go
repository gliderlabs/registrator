package skydns2

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/coreos/etcd/client"
	"github.com/pipedrive/registrator/bridge"
	"time"
	"context"
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

	etcd2Config := client.Config{
		Endpoints:               urls,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}

	etcdClient, err := client.New(etcd2Config)

	if err != nil {
		log.Fatal("etcd: can't initialize client", err)
	}

	return &Skydns2Adapter{client: &etcdClient, path: domainPath(uri.Path[1:])}
}

type Skydns2Adapter struct {
	client  *client.Client
	keysApi *client.KeysAPI
	path    string
}

func (r *Skydns2Adapter) Ping() error {
	_, err := (*r.client).GetVersion(context.Background())
	if err != nil {
		return err
	}
	return nil
}

func (r *Skydns2Adapter) Register(service *bridge.Service) error {
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	_, err := (*r.keysApi).Set(context.Background(), r.servicePath(service), record, &client.SetOptions{TTL: time.Duration(service.TTL)})
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Deregister(service *bridge.Service) error {
	_, err := (*r.keysApi).Delete(context.Background(), r.servicePath(service), &client.DeleteOptions{Recursive: false})
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) SetupHealthCheck(service *bridge.Service, healthCheck *bridge.TtlHealthCheck) error {
	return nil
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
