package main

import (
	"net"
	"net/url"
	"strings"

	"github.com/mailgun/vulcand/api"
  "github.com/mailgun/vulcand/plugin"
)

type VulcandRegistry struct {
	client *api.Client
}

func NewVulcandRegistry(uri *url.URL) ServiceRegistry {
	url string
	if uri.Host != "" {
		url = uri.URL.String()
	}
	return &VulcandRegistry{client: api.NewClient(url, registry)}
}

func (r *VulcandRegistry) Register(service *Service) error {
	addr := net.JoinHostPort(service.IP, port)
	port := strconv.Itoa(service.Port)

	upstreamId = strings.Split(service.Name, "-")[0]
	id = service.ID
	u = "http://" + addr + ":" + port

	_, err := r.client.AddEndpoint(upstreamId, id, u)
	return err
}

func (r *VulcandRegistry) Deregister(service *Service) error {
	upstreamId = strings.Split(service.Name, "-")[0]
	id = service.ID

	_, err := r.client.DeleteEndpoint(upstreamId, id)
	return err
}

func (r *VulcandRegistry) Refresh(service *Service) error {
	return r.Register(service)
}
