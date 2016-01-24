package skydns2

import (
	"log"
    "os"
	"crypto/tls"
	"net/http"
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

	if len(uri.Path) < 2 {
		log.Fatal("skydns2: dns domain required e.g.: skydns2://<host>/<domain>")
	}

	tlskey := os.Getenv("ETCD_TLSKEY")
	tlspem := os.Getenv("ETCD_TLSPEM")
	cacert := os.Getenv("ETCD_CACERT")

	var client *etcd.Client

    // Assuming https
    if cacert != "" {
        urls = append(urls, "https://"+uri.Host)

        // Assuming Client authentication if tlskey and tlspem is set
        if tlskey != "" && tlspem != "" {
            var err error
            if client, err = etcd.NewTLSClient(urls, tlspem, tlskey, cacert); err != nil {
                log.Fatalf("skydns2: failure to connect: %s", err)
            }
        } else {
            client = etcd.NewClient(urls)
            tr := &http.Transport {
                TLSClientConfig:    &tls.Config{RootCAs: cacert},
                DisableCompression: true,
            }
            client.SetTransport(tr)
        }
    } else {
        urls = append(urls, "http://"+uri.Host)
        client = etcd.NewClient(urls)
    }

	return &Skydns2Adapter{client: client, path: domainPath(uri.Path[1:])}
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
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	_, err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
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
