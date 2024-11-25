package etcd

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"context"
	"net/url"

	//"github.com/coreos/go-etcd/etcd"
	"go.etcd.io/etcd/client"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/gliderlabs/registrator/bridge"
	"crypto/tls"
)

func init() {
	bridge.Register(new(Factory), "etcd2")
}

type Factory struct{}

func CreateEndpoints(addrs []string, scheme string) (entries []string) {
	for _, addr := range addrs {
		entries = append(entries, scheme+"://"+addr)
	}
	return entries
}

func setTLS(cfg *client.Config, tls *tls.Config, addrs []string) {
	entries := CreateEndpoints(addrs, "https")
	cfg.Endpoints = entries

	// Set transport
	t := http.Transport{
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tls,
	}

	cfg.Transport = &t
}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	cert := os.Getenv("ETCD_CERT_FILE")
	key := os.Getenv("ETCD_KEY_FILE")
	cacert := os.Getenv("ETCD_CA_CERT_FILE")
	

	var (
		addrs = strings.Split(uri.Host, ",")
		path = uri.Path
                entries []string
	)
        entries = CreateEndpoints(addrs, "http")
	cfg := &client.Config{
		Endpoints:               entries,
		Transport:               client.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}

	// Assuming https
	if cacert != "" {
		if key != "" && cert != "" {
			tlsConfig, err := tlsconfig.Client(tlsconfig.Options{
				CAFile:   cacert,
				CertFile: cert,
				KeyFile:  key,
			})
			if err != nil {
				log.Fatalf("etcd: tls config: %s", err)
			}
			setTLS(cfg, tlsConfig, addrs)
		}
	}
	c, err := client.New(*cfg)
	if err != nil {
		log.Fatalf("etcd: failure to connect: %s", err)
	}
	kapi := client.NewKeysAPI(c)

	return &EtcdAdapter{client: kapi, path: path}
}

type EtcdAdapter struct {
	client client.KeysAPI

	path string
}

func (r *EtcdAdapter) Ping() error {
	var err error

	getOpts := &client.GetOptions{
		Quorum: true,
		Sort: true, 
		Recursive: true,
	}

	_, err = r.client.Get(context.Background(), "/", getOpts)
	if err != nil {
		return err
	}
	return nil
}

func (r *EtcdAdapter) Register(service *bridge.Service) error {
	setOpts := &client.SetOptions{}
	path := r.path + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)

	var err error
	setOpts.TTL = time.Duration(service.TTL) * time.Second	
	_, err = r.client.Set(context.Background(), path, addr, setOpts)

	if err != nil {
		log.Println("etcd: failed to register service:", err)
	}
	return err
}

func (r *EtcdAdapter) Deregister(service *bridge.Service) error {
	opts := &client.DeleteOptions{
		Recursive: false,
	}
	path := r.path + "/" + service.Name + "/" + service.ID

	var err error
	_, err = r.client.Delete(context.Background(), path, opts)

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
