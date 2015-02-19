//go:generate go-extpoints . RegistryFactory
package bridge

import (
	"net/url"

	dockerapi "github.com/fsouza/go-dockerclient"
)

type RegistryFactory interface {
	New(uri *url.URL) ServiceRegistry
}

type ServiceRegistry interface {
	Ping() error
	Register(service *Service) error
	Deregister(service *Service) error
	Refresh(service *Service) error
}

type Config struct {
	HostIp          string
	Internal        bool
	ForceTags       string
	RefreshTtl      int
	RefreshInterval int
}

type Service struct {
	ID    string
	Name  string
	Port  int
	IP    string
	Tags  []string
	Attrs map[string]string
	TTL   int

	Origin ServicePort
}

type ServicePort struct {
	HostPort          string
	HostIP            string
	ExposedPort       string
	ExposedIP         string
	PortType          string
	ContainerHostname string
	ContainerID       string
	container         *dockerapi.Container
}
