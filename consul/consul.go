package consul

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"os"
	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-cleanhttp"
)

const DefaultInterval = "10s"

func init() {
	f := new(Factory)
	bridge.Register(f, "consul")
	bridge.Register(f, "consul-tls")
	bridge.Register(f, "consul-unix")
}

func (r *ConsulAdapter) interpolateService(check string, service *bridge.Service, services []*bridge.Service) string {
	result := check

	result = strings.Replace(result, "$SERVICE_IP", service.Origin.HostIP, -1)
	result = strings.Replace(result, "$SERVICE_PORT", service.Origin.HostPort, -1)

	for _, s := range services {
		ip := fmt.Sprintf("$SERVICE_%s_IP", s.Origin.ExposedPort)
		result = strings.Replace(result, ip, s.Origin.HostIP, -1)
		port := fmt.Sprintf("$SERVICE_%s_PORT", s.Origin.ExposedPort)
		result = strings.Replace(result, port, s.Origin.HostPort, -1)
	}
	return result
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Scheme == "consul-unix" {
		config.Address = strings.TrimPrefix(uri.String(), "consul-")
	} else if uri.Scheme == "consul-tls" {
	        tlsConfigDesc := &consulapi.TLSConfig {
			  Address: uri.Host,
			  CAFile: os.Getenv("CONSUL_CACERT"),
  			  CertFile: os.Getenv("CONSUL_TLSCERT"),
  			  KeyFile: os.Getenv("CONSUL_TLSKEY"),
			  InsecureSkipVerify: false,
		}
		tlsConfig, err := consulapi.SetupTLSConfig(tlsConfigDesc)
		if err != nil {
		   log.Fatal("Cannot set up Consul TLSConfig", err)
		}
		config.Scheme = "https"
		transport := cleanhttp.DefaultPooledTransport()
		transport.TLSClientConfig = tlsConfig
		config.HttpClient.Transport = transport
		config.Address = uri.Host
	} else if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatal("consul: ", uri.Scheme)
	}
	return &ConsulAdapter{client: client}
}

type ConsulAdapter struct {
	client *consulapi.Client
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *ConsulAdapter) Ping() error {
	status := r.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}
	log.Println("consul: current leader ", leader)

	return nil
}

func (r *ConsulAdapter) Register(service *bridge.Service, services []*bridge.Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service, services)
	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulAdapter) buildCheck(service *bridge.Service, services []*bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if url := service.Attrs["check_http"]; url != "" {
		if strings.HasPrefix(url, "/") {
			check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, url)
		} else {
			check.HTTP = r.interpolateService(url, service, services)
		}
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd)
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Script = r.interpolateService(script, service, services)
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else if tcp := service.Attrs["check_tcp"]; tcp != "" {
		check.TCP = fmt.Sprintf("%s:%d", service.IP, service.Port)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else {
		return nil
	}
	if check.Script != "" || check.HTTP != "" || check.TCP != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	return check
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulAdapter) Refresh(service *bridge.Service, services []*bridge.Service) error {
	return nil
}

func (r *ConsulAdapter) Services() ([]*bridge.Service, error) {
	services, err := r.client.Agent().Services()
	if err != nil {
		return []*bridge.Service{}, err
	}
	out := make([]*bridge.Service, len(services))
	i := 0
	for _, v := range services {
		s := &bridge.Service{
			ID:   v.ID,
			Name: v.Service,
			Port: v.Port,
			Tags: v.Tags,
			IP:   v.Address,
		}
		out[i] = s
		i++
	}
	return out, nil
}
