package bridge

import (
	"errors"
	"net/url"

	dockerapi "github.com/fsouza/go-dockerclient"
)

var (
	// ErrCallNotSupported is thrown when a method is not implemented/supported by the current backend
	ErrCallNotSupported = errors.New("The current call is not supported with this backend")

	// ErrBackendNotSupported is thrown when the backend k/v store is not supported by libkv
	ErrBackendNotSupported = errors.New("backend storage not supported, please choose one of")
)

// Initialize creates a new Backent object, initializing the client
type Initialize func(uri *url.URL) (RegistryAdapter, error)

type RegistryAdapter interface {
	Ping() error
	Register(service *Service) error
	Deregister(service *Service) error
	Refresh(service *Service) error
	Services() ([]*Service, error)
}

type Config struct {
	HostIP          string
	Internal        bool
	UseIPFromLabel  string
	ForceTags       string
	RefreshTTL      int
	RefreshInterval int
	DeregisterCheck string
	Cleanup         bool
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

type DeadContainer struct {
	TTL      int
	Services []*Service
}

type ServicePort struct {
	HostPort          string
	HostIP            string
	ExposedPort       string
	ExposedIP         string
	PortType          string
	ContainerHostname string
	ContainerID       string
	ContainerName     string
	container         *dockerapi.Container
}
