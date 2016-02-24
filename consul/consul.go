package consul

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

const DefaultInterval = "10s"

func init() {
	f := new(Factory)
	bridge.Register(f, "consul")
	bridge.Register(f, "consul-unix")
}

func (r *ConsulAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.Origin.HostIP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", service.Origin.HostPort, -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Scheme == "consul-unix" {
		config.Address = strings.TrimPrefix(uri.String(), "consul-")
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

	if registration.Check == nil && service.TTL > 0 {
		registration.Check = r.buildDefaultCheck(service)
	}

	response := r.client.Agent().ServiceRegister(registration)

	return response
}

func (r *ConsulAdapter) buildHttpCheck(check_attr string, service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, check_attr)
	check.Interval = DefaultInterval

	if timeout := service.Attrs["check_timeout"]; timeout != "" {
		check.Timeout = timeout
	}
	if interval := service.Attrs["check_interval"]; interval != "" {
		check.Interval = interval
	}

	return check
}

func (r *ConsulAdapter) buildCmdCheck(check_attr string, service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	check.Script = fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, check_attr)
	return check
}

func (r *ConsulAdapter) buildScriptCheck(check_attr string, service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	check.Script = r.interpolateService(check_attr, service)
	check.Interval = DefaultInterval

	if interval := service.Attrs["check_interval"]; interval != "" {
		check.Interval = interval
	}

	return check
}

func (r *ConsulAdapter) buildTtlCheck(check_attr string, service *bridge.Service) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	check.TTL = check_attr
	return check
}

func (r *ConsulAdapter) buildDefaultCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	return r.buildTtlCheck(fmt.Sprintf("%ds", service.TTL), service)
}

func (r *ConsulAdapter) buildCheck(service *bridge.Service) *consulapi.AgentServiceCheck {
	for key, value := range service.Attrs {
		switch key {
		case "check_http":
			return r.buildHttpCheck(value, service)
		case "check_cmd":
			return r.buildCmdCheck(value, service)
		case "check_script":
			return r.buildScriptCheck(value, service)
		case "check_ttl":
			return r.buildTtlCheck(value, service)
		}
	}

	return nil
}

func (r *ConsulAdapter) usesDefaultCheck(service *bridge.Service) bool {
	return r.buildCheck(service) == nil && service.TTL > 0
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
	if r.usesDefaultCheck(service) {
		return r.client.Agent().PassTTL(
			fmt.Sprintf("service:%s", service.ID),
			fmt.Sprintf("refreshed: %s %s", service.Origin.ContainerID[:12], service.ID),
		)
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
