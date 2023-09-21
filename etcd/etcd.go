package etcd

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	etcd2 "github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
	etcd3 "go.etcd.io/etcd/client/v3"
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

	if match, _ := regexp.Match("3\\.[0-9]*\\.[0-9]*", body); match == true {
		cli, err := etcd3.New(etcd3.Config{Endpoints: urls, DialTimeout: 5 * time.Second})
		if err != nil {
			log.Fatal("etcd: error connecting etcd", err)
		}
		log.Println("etcd: using v3 client")
		return &EtcdAdapter{client3: cli, path: uri.Path}
	}

	log.Println("etcd: using v2 client")
	return &EtcdAdapter{client2: etcd2.NewClient(urls), path: uri.Path}
}

type EtcdAdapter struct {
	client  *etcd.Client
	client2 *etcd2.Client
	client3 *etcd3.Client

	path string
}

func (r *EtcdAdapter) Ping() error {
	r.syncEtcdCluster()

	var err error
	if r.client != nil {
		rr := etcd.NewRawRequest("GET", "version", nil, nil)
		_, err = r.client.SendRequest(rr)
	} else if r.client3 != nil {
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
	} else if r.client3 != nil {
		r.client3.Sync(context.TODO())
	} else {
		result = r.client2.SyncCluster()
	}

	if !result {
		log.Println("etcd: sync cluster was unsuccessful")
	}
}

func (r *EtcdAdapter) Register(service *bridge.Service) error {
	r.syncEtcdCluster()

	path := r.path + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)

	var err error
	if r.client != nil {
		_, err = r.client.Set(path, addr, uint64(service.TTL))
	} else if r.client3 != nil {
		if service.TTL == 0 {
			_, err = r.client3.Put(context.TODO(), path, addr)
		} else {
			var resp *etcd3.LeaseGrantResponse
			resp, err = r.client3.Grant(context.TODO(), int64(service.TTL))
			if err == nil {
				_, err = r.client3.Put(context.TODO(), path, addr, etcd3.WithLease(resp.ID))
			}
		}
	} else {
		_, err = r.client2.Set(path, addr, uint64(service.TTL))
	}

	if err != nil {
		log.Println("etcd: failed to register service:", err)
	}
	return err
}

func (r *EtcdAdapter) Deregister(service *bridge.Service) error {
	r.syncEtcdCluster()

	path := r.path + "/" + service.Name + "/" + service.ID

	var err error
	if r.client != nil {
		_, err = r.client.Delete(path, false)
	} else if r.client3 != nil {
		_, err = r.client3.Delete(context.TODO(), path)
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
