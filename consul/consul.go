package consul

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"strconv"
	"os"
	"time"
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

func (r *ConsulAdapter) Register(service *bridge.Service) error {
	if err := r.registerService(service); err != nil {
		return err
	}
	if err := r.registerChecks(service); err != nil {
		r.Deregister(service)
		return err
	}
	return nil
}

func (r *ConsulAdapter) registerService(service *bridge.Service) error {
	registration := new(consulapi.AgentServiceRegistration)
	registration.ID = service.ID
	registration.Name = service.Name
	registration.Port = service.Port
	registration.Tags = service.Tags
	registration.Address = service.IP
	return r.client.Agent().ServiceRegister(registration)
}

func (r *ConsulAdapter) registerChecks(service *bridge.Service) error {
	if service.TTL > 0 {
		check := r.newCheck(service, "service:%s:registrator_ttl", "Service '%s' Registrator TTL Check")
		check.TTL = fmt.Sprintf("%ds", service.TTL)
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	if path := service.Attrs["check_http"]; path != "" {
		check := r.newCheck(service, "service:%s:http", "Service '%s' HTTP Check")
		check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, path)
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	if path := service.Attrs["check_https"]; path != "" {
		check := r.newCheck(service, "service:%s:https", "Service '%s' HTTPS Check")
		check.HTTP = fmt.Sprintf("https://%s:%d%s", service.IP, service.Port, path)
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check := r.newCheck(service, "service:%s:cmd", "Service '%s' CMD Check")
		check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd)
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	if script := service.Attrs["check_script"]; script != "" {
		check := r.newCheck(service, "service:%s:script", "Service '%s' Script Check")
		check.Script = r.interpolateService(script, service)
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check := r.newCheck(service, "service:%s:ttl", "Service '%s' TTL Check")
		check.TTL = ttl
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	if tcp := service.Attrs["check_tcp"]; tcp != "" {
		check := r.newCheck(service, "service:%s:tcp", "Service '%s' TCP Check")
		check.TCP = fmt.Sprintf("%s:%d", service.IP, service.Port)
		if err := r.configureAndRegisterCheck(service, check); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConsulAdapter) newCheck(service *bridge.Service, idFormat string, nameFormat string) *consulapi.AgentCheckRegistration {
	check := new(consulapi.AgentCheckRegistration)
	check.ID = fmt.Sprintf(idFormat, service.ID)
	check.Name = fmt.Sprintf(nameFormat, service.Name)
	check.ServiceID = service.ID
	return check
}

func (r *ConsulAdapter) configureAndRegisterCheck(service *bridge.Service, check *consulapi.AgentCheckRegistration) error {
	if initialStatus := service.Attrs["check_initial_status"]; initialStatus != "" {
		check.Status = initialStatus
	}
	if check.HTTP != "" || check.TCP != "" {
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	}
	if check.Script != "" || check.HTTP != "" || check.TCP != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	if deregisterAfter := service.Attrs["check_deregister_after"]; deregisterAfter != "" {
		check.DeregisterCriticalServiceAfter = deregisterAfter
	}
	return r.client.Agent().CheckRegister(check)
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
	if service.TTL > 0 {
		checkID := fmt.Sprintf("service:%s:registrator_ttl", service.ID)
		note := fmt.Sprintf("Pass TTL at %s", time.Now().Format(time.UnixDate))
		log.Println(note, "for", checkID)
		r.client.Agent().PassTTL(checkID, note)
	}
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
