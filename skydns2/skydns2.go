package skydns2

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
	etcd3 "go.etcd.io/etcd/client/v3"
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

	res, err := http.Get(urls[0] + "/version")
	if err != nil {
		log.Fatal("etcd: error retrieving version", err)
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	if match, _ := regexp.Match("3\\.[0-9]*\\.[0-9]*", body); match == true {
		cli, err := etcd3.New(etcd3.Config{Endpoints: urls, DialTimeout: 5 * time.Second})
		if err != nil {
			log.Fatal("etcd: error connecting etcd", err)
		}
		log.Println("etcd: using v3 client")
		return &Skydns2Adapter{client3: cli, path: domainPath(uri.Path[1:])}
	}

	log.Println("etcd: using v2 client")
	return &Skydns2Adapter{client: etcd.NewClient(urls), path: domainPath(uri.Path[1:])}
}

type Skydns2Adapter struct {
	client  *etcd.Client
	client3 *etcd3.Client
	path    string
}

func (r *Skydns2Adapter) Ping() error {
	var err error
	if r.client != nil {
		rr := etcd.NewRawRequest("GET", "version", nil, nil)
		_, err = r.client.SendRequest(rr)
	}
	if err != nil {
		return err
	}
	return nil
}

func (r *Skydns2Adapter) Register(service *bridge.Service) error {
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	var err error
	if r.client != nil {
		_, err = r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	} else if r.client3 != nil {
		if service.TTL == 0 {
			_, err = r.client3.Put(context.TODO(), r.servicePath(service), record)
		} else {
			var resp *etcd3.LeaseGrantResponse
			resp, err = r.client3.Grant(context.TODO(), int64(service.TTL))
			if err == nil {
				_, err = r.client3.Put(context.TODO(), r.servicePath(service), record, etcd3.WithLease(resp.ID))
			}
		}
	}
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Deregister(service *bridge.Service) error {
	var err error
	if r.client != nil {
		_, err = r.client.Delete(r.servicePath(service), false)
	} else if r.client3 != nil {
		_, err = r.client3.Delete(context.TODO(), r.servicePath(service))
	}
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
