package etcd

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"

	etcd2 "github.com/coreos/go-etcd/etcd"
	etcd "gopkg.in/coreos/go-etcd.v0/etcd"
)

type EtcdClient struct {
	client  *etcd.Client
	client2 *etcd2.Client
}

func NewEtcdClient(host string) (*EtcdClient, error) {
	urls := make([]string, 0)
	if host != "" {
		urls = append(urls, "http://"+host)
	} else {
		urls = append(urls, "http://127.0.0.1:4001")
	}

	res, err := http.Get(urls[0] + "/version")
	if err != nil {
		return nil, fmt.Errorf("error retrieving version: %s", err)
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	if match, _ := regexp.Match("0\\.4\\.*", body); match == true {
		log.Println("etcd client: using v0 client")
		return &EtcdClient{client: etcd.NewClient(urls)}, nil
	}

	return &EtcdClient{client2: etcd2.NewClient(urls)}, nil
}

func (e *EtcdClient) Ping() error {
	var err error
	if e.client != nil {
		rr := etcd.NewRawRequest("GET", "version", nil, nil)
		_, err = e.client.SendRequest(rr)
	} else {
		rr := etcd2.NewRawRequest("GET", "version", nil, nil)
		_, err = e.client2.SendRequest(rr)
	}
	return err
}

func (e *EtcdClient) Set(path, value string, ttl uint64) error {
	var err error
	if e.client != nil {
		_, err = e.client.Set(path, value, ttl)
	} else {
		_, err = e.client2.Set(path, value, ttl)
	}
	return err
}

func (e *EtcdClient) Delete(path string, recursive bool) error {
	var err error
	if e.client != nil {
		_, err = e.client.Delete(path, recursive)
	} else {
		_, err = e.client2.Delete(path, recursive)
	}
	return err
}
