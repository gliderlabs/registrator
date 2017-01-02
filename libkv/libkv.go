package libkv

import (
	"log"
	"net"
	"net/url"
	"strconv"

	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/libkv/store/dynamodb"
	"github.com/docker/libkv/store/etcd"
	"github.com/docker/libkv/store/zookeeper"
	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	dynamodb.Register()
	bridge.Register("dynamodb", New(store.DYNAMODB))

	etcd.Register()
	bridge.Register("etcd", New(store.ETCD))

	consul.Register()
	bridge.Register("consulkv", New(store.CONSUL))

	zookeeper.Register()
	bridge.Register("zookeeper", New(store.ZK))
}

func New(backend store.Backend) bridge.Initialize {
	return func(uri *url.URL) (bridge.RegistryAdapter, error) {
		var endpoint string
		if uri.Host != "" {
			endpoint = uri.String()
		}

		kv, err := libkv.NewStore(
			backend,
			[]string{endpoint},
			nil,
		)
		return &KVAdapter{client: kv}, err
	}
}

type KVAdapter struct {
	client store.Store
}

func (r *KVAdapter) Ping() error {
	_, err := r.client.List("/")
	return err
}

func (r *KVAdapter) Register(service *bridge.Service) error {
	key := service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	log.Println("backend: registering", key, addr)
	return r.client.Put(key, []byte(addr), nil)
}

func (r *KVAdapter) Deregister(service *bridge.Service) error {
	key := service.Name + "/" + service.ID
	return r.client.Delete(key)
}

func (r *KVAdapter) Refresh(service *bridge.Service) error {
	return bridge.ErrCallNotSupported
}

func (r *KVAdapter) Services() ([]*bridge.Service, error) {
	return nil, bridge.ErrCallNotSupported
}
