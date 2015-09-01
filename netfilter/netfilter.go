package netfilter

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "netfilter")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	var chain, set string
	if uri.Host != "" {
		chain = uri.Host
		set = strings.Replace(uri.Path, "/", "", -1)
	} else {
		chain = "FORWARD_direct"
		set = "containerports"
	}
	FirewalldInit()
	if firewalldRunning {
		OnReloaded(func() { iptablesInit(chain, set) })
	}
	ipsetInit(set)
	iptablesInit(chain, set)
	return &NetfilterAdapter{Chain: chain, Set: set}
}

type NetfilterAdapter struct {
	Chain string
	Set   string
}

func (r *NetfilterAdapter) Ping() error {
	return nil
}

func (r *NetfilterAdapter) Register(service *bridge.Service) error {
	if strings.Contains(service.IP, ":") {
		return ipsetHost("add", r.Set, service.IP, service.Origin.PortType, strconv.Itoa(service.Port))
	} else {
		return nil
	}
}

func (r *NetfilterAdapter) Deregister(service *bridge.Service) error {
	if strings.Contains(service.IP, ":") {
		return ipsetHost("del", r.Set, service.IP, service.Origin.PortType, strconv.Itoa(service.Port))
	} else {
		return nil
	}
}

func (r *NetfilterAdapter) Refresh(service *bridge.Service) error {
	return nil
}
