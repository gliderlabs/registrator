package consul

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gliderlabs/registrator/bridge"
	consulapi "github.com/hashicorp/consul/api"
)

const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "consul")
}

func (r *ConsulAdapter) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.Origin.HostIP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", service.Origin.HostPort, -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
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
	registration.Checks = r.buildChecks(service)
	return r.client.Agent().ServiceRegister(registration)
}

type Check struct {
	Service *bridge.Service
	Type    string
	Value   string
}
type Checks []*Check

func (c Checks) Len() int           { return len(c) }
func (c Checks) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c Checks) Less(i, j int) bool { return c[i].Type < c[j].Type }

/*
 * Checks are ordered in Consul. There needs to be a deterministic way of
 * re-attaching them for refreshes. The simplest solution is to order them
 * alphabetically.
 */
func filterChecks(service *bridge.Service) Checks {
	checks := make(Checks, 0)
	for k, v := range service.Attrs {
		if strings.Index(k, "check_") == 0 && k != "check_interval" && v != "" {
			check := new(Check)
			check.Service = service
			check.Type = k
			check.Value = v
			checks = append(checks, check)
		}
	}
	sort.Sort(checks)
	return checks
}

func interval(service *bridge.Service) string {
	interval := DefaultInterval
	if service.Attrs["check_interval"] != "" {
		interval = service.Attrs["check_interval"]
	}
	return interval
}

func scriptCheck(interval string, script string) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	check.Script = script
	check.Interval = interval
	return check
}

func ttlCheck(ttl string) *consulapi.AgentServiceCheck {
	check := new(consulapi.AgentServiceCheck)
	check.TTL = ttl
	return check
}

func (r *ConsulAdapter) buildChecks(service *bridge.Service) consulapi.AgentServiceChecks {
	interval := interval(service)
	checks := make(consulapi.AgentServiceChecks, 0)

	for _, c := range filterChecks(service) {
		switch c.Type {
		case "check_http":
			checks = append(checks, scriptCheck(interval, fmt.Sprintf("check-http %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, c.Value)))
		case "check_cmd":
			checks = append(checks, scriptCheck(interval, fmt.Sprintf("check-cmd %s %s %s", service.Origin.ContainerID[:12], service.Origin.ExposedPort, c.Value)))
		case "check_script":
			checks = append(checks, scriptCheck(interval, r.interpolateService(c.Value, service)))
		case "check_ttl":
			checks = append(checks, ttlCheck(c.Value))
		default:
			break
		}
	}

	if len(checks) < 1 {
		return nil
	}
	return checks
}

func (r *ConsulAdapter) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

// Used for testing.
type refresher func(string, string) error

func (r *ConsulAdapter) refresh(service *bridge.Service, refresher refresher) error {
	checks := filterChecks(service)
	count := len(checks)
	for i, c := range checks {
		if c.Type == "check_ttl" {
			checkId := ""
			if count > 1 {
				checkId = fmt.Sprintf(":%d", i+1)
			}
			ttl := fmt.Sprintf("service:%s%s", service.ID, checkId)
			log.Println("refreshing:", ttl)
			// Because checks are passed in a map, there will only be one TTL check.
			return refresher(ttl, time.Now().Format(time.RFC850))
		}
	}
	return nil
}

func (r *ConsulAdapter) Refresh(service *bridge.Service) error {
	return r.refresh(service, r.client.Agent().PassTTL)
}
