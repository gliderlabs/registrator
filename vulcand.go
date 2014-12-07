package main

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"log"

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
		url = strings.Replace(uri.String(), "vulcand://", "http://", 1)
	}
	
	return &VulcandRegistry{client: api.NewClient(url, registry)}
}

func (r *VulcandRegistry) Register(service *Service) error {
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)

	upstreamId := getUpstreamId(service.ID)
	id := service.ID
	u := "http://" + addr

	_, err := r.client.AddEndpoint(upstreamId, id, u)
  return err
}

func (r *VulcandRegistry) Deregister(service *Service) error {
	upstreamId := getUpstreamId(service.ID)
	id := service.ID

	_, err := r.client.DeleteEndpoint(upstreamId, id)
  return err
}

func (r *VulcandRegistry) Refresh(service *Service) error {
	return r.Register(service)
}


func getUpstreamId(name string) string {    
  upstreamId := strings.Split(name, ":")[1]
  if(strings.ContainsAny(upstreamId, "-")){
    upstreamId = strings.Split(upstreamId, "-")[0]
  }
	log.Println("getUpstreamId() ", name, upstreamId)
  return upstreamId
}
