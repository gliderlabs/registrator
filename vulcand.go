package main

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/mailgun/vulcand/api"
  "github.com/mailgun/vulcand/plugin"
)

type VulcandRegistry struct {
	client *api.Client
}

func NewVulcandRegistry(uri *url.URL) ServiceRegistry {
	var url string
	var registry *plugin.Registry
	
	if uri.Host != "" {
		url = uri.String()
	}
	
	return &VulcandRegistry{client: api.NewClient(url, registry)}
}

func (r *VulcandRegistry) Register(service *Service) error {
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)

	upstreamId := getUpstreamId(service.Name)
	id := service.ID
	u := "http://" + addr + ":" + port

	_, err := r.client.AddEndpoint(upstreamId, id, u)
	return err
}

func (r *VulcandRegistry) Deregister(service *Service) error {
	upstreamId := getUpstreamId(service.Name)
	id := service.ID

	_, err := r.client.DeleteEndpoint(upstreamId, id)
	return err
}

func (r *VulcandRegistry) Refresh(service *Service) error {
	return r.Register(service)
}


func getUpstreamId(name string) string {    
  upstreamId := name
  if(strings.ContainsAny(name, "-")){
    upstreamId = strings.Split(name, "-")[0]
  }
  return upstreamId
}
