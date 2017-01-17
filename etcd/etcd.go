package etcd

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	etcd2 "github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
	etcd "gopkg.in/coreos/go-etcd.v0/etcd"
)

func init() {
	bridge.Register(new(Factory), "etcd")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	} else {
		urls = append(urls, "http://127.0.0.1:2379")
	}

	res, err := http.Get(urls[0] + "/version")
	if err != nil {
		log.Fatal("etcd: error retrieving version", err)
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	if match, _ := regexp.Match("0\\.4\\.*", body); match == true {
		log.Println("etcd: using v0 client")
		return &EtcdAdapter{client: etcd.NewClient(urls), path: uri.Path}
	}

	query := uri.Query()
	method := "plain"
	if nm, ok := query["method"]; ok {
		method = nm[0]
	}

	return &EtcdAdapter{client2: etcd2.NewClient(urls), path: uri.Path, method: method}
}

type EtcdAdapter struct {
	client  *etcd.Client
	client2 *etcd2.Client

	path string
	method string
}

func (r *EtcdAdapter) Ping() error {
	r.syncEtcdCluster()

	var err error
	if r.client != nil {
		rr := etcd.NewRawRequest("GET", "version", nil, nil)
		_, err = r.client.SendRequest(rr)
	} else {
		rr := etcd2.NewRawRequest("GET", "version", nil, nil)
		_, err = r.client2.SendRequest(rr)
	}

	if err != nil {
		return err
	}
	return nil
}

func (r *EtcdAdapter) syncEtcdCluster() {
	var result bool
	if r.client != nil {
		result = r.client.SyncCluster()
	} else {
		result = r.client2.SyncCluster()
	}

	if !result {
		log.Println("etcd: sync cluster was unsuccessful")
	}
}

func (r *EtcdAdapter) Register(service *bridge.Service) error {
	r.syncEtcdCluster()

	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)

	desiredKV := r.getKeyValueMapForMethod(service, addr)

	var err error
	for k, v := range(desiredKV) {
		if r.client != nil {
			_, err = r.client.Set(k, v, uint64(service.TTL))
		} else {
			_, err = r.client2.Set(k, v, uint64(service.TTL))
		}

		if err != nil {
			log.Println("etcd: failed to register service:", err)
			return err
		}
	}

	return nil
}

func (r *EtcdAdapter) Deregister(service *bridge.Service) error {
	r.syncEtcdCluster()

	path := r.path + "/" + service.Name + "/" + service.ID

	var err error
	if r.client != nil {
		_, err = r.client.Delete(path, false)
	} else {
		_, err = r.client2.Delete(path, false)
	}

	if err != nil {
		log.Println("etcd: failed to deregister service:", err)
	}
	return err
}

func (r *EtcdAdapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *EtcdAdapter) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}

func (r *EtcdAdapter) getKeyValueMapForMethod(s *bridge.Service, addr string) map[string]string {
	if r.method != "traefik" {
		return map[string]string {
			r.path + "/" + s.Name + "/" + s.ID: addr,
		}
	}

	tags := extractTraefikTags(s.Tags)
	backendName := s.Name

	if be, ok := tags["backend"]; ok {
		backendName = be
	}

	prefix := r.path + "/backends" + backendName + "/servers/" + addr

	return map[string]string {
		prefix + "/url": addr,
		prefix + "/weight": "10",
	}

}

func extractTraefikTags(tags []string) map[string]string {
	a := map[string]string{}

	for _, v := range(tags) {
		if strings.HasPrefix(v, "traefik.") {
			kv := strings.Split(v[8:], "=")

			if len(kv) == 2 {
				a[kv[0]] = kv[1]
			}
		}
	}

	return a
}
