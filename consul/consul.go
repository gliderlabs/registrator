package consul

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

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

func (r *ConsulAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.IP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", strconv.Itoa(service.Port), -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Scheme == "consul-unix" {
		config.Address = strings.TrimPrefix(uri.String(), "consul-")
	} else if uri.Scheme == "consul-tls" {
		tlsConfigDesc := &consulapi.TLSConfig{
			Address:            uri.Host,
			CAFile:             os.Getenv("CONSUL_CACERT"),
			CertFile:           os.Getenv("CONSUL_CLIENT_CERT"),
			KeyFile:            os.Getenv("CONSUL_CLIENT_KEY"),
			InsecureSkipVerify: false,
		}
		tlsConfig, err := consulapi.SetupTLSConfig(tlsConfigDesc)
		if err != nil {
			log.Fatal("Cannot set up Consul TLSConfig", err)
		}
		config.Scheme = "https"
		transport := cleanhttp.DefaultPooledTransport()
		transport.TLSClientConfig = tlsConfig
		config.Transport = transport
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

func (r *ConsulAdapter) Register(service *bridge.Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	registration.Check = r.buildCheck(service)
	registration.Meta = service.Attrs
	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	if status := service.Attrs["check_initial_status"]; status != "" {
		check.Status = status
	}
	if path := service.Attrs["check_http"]; path != "" {
		check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
		if method := service.Attrs["check_http_method"]; method != "" {
			check.Method = method
		}
	} else if path := service.Attrs["check_https"]; path != "" {
		check.HTTP = fmt.Sprintf("https://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
		if method := service.Attrs["check_https_method"]; method != "" {
			check.Method = method
		}
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Args = []string{"check-cmd", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd}
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Args = []string{r.interpolateService(script, service)}
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else if tcp := service.Attrs["check_tcp"]; tcp != "" {
		check.TCP = fmt.Sprintf("%s:%d", service.IP, service.Port)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else if grpc := service.Attrs["check_grpc"]; grpc != "" {
		check.GRPC = fmt.Sprintf("%s:%d", service.IP, service.Port)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
		if useTLS := service.Attrs["check_grpc_use_tls"]; useTLS != "" {
			check.GRPCUseTLS = true
			if tlsSkipVerify := service.Attrs["check_tls_skip_verify"]; tlsSkipVerify != "" {
				check.TLSSkipVerify = true
			}
		}
	} else {
		return nil
	}
	if len(check.Args) != 0 || check.HTTP != "" || check.TCP != "" || check.GRPC != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	if deregister_after := service.Attrs["check_deregister_after"]; deregister_after != "" {
		check.DeregisterCriticalServiceAfter = deregister_after
	}
	return check
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
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
