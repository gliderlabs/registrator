package skydns2

import (
	"errors"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "skydns2")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}

	if len(uri.Path) < 2 {
		log.Fatal("skydns2: dns domain required e.g.: skydns2://<host>/<domain>")
	}

	return &Skydns2Adapter{client: etcd.NewClient(urls), path: domainPath(uri.Path[1:]), domain: uri.Path[1:]}
}

type Skydns2Adapter struct {
	client *etcd.Client
	path   string
	domain string
}

func (r *Skydns2Adapter) Ping() error {
	rr := etcd.NewRawRequest("GET", "version", nil, nil)
	_, err := r.client.SendRequest(rr)
	if err != nil {
		return err
	}
	return nil
}

func (r *Skydns2Adapter) Register(service *bridge.Service) error {
	port := strconv.Itoa(service.Port)
	record := `{"host":"` + service.IP + `","port":` + port + `}`
	_, err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
		return err
	}

	// Only setup reverse DNS mappings if the service's IP is unique.
	// Anything else does not make sense right now as SkyDNS 2 only supports one
	// host per entry type and overwriting the reverse DNS entry for every service
	// exposed on the host may lead to confusing look ups.
	if service.Origin.ExposedIP != service.IP {
		return nil
	}

	raddr, err := reverseDomainName(service.Origin.ExposedIP)
	if err != nil {
		log.Println("skydns2: failed to determine reverse DNS entry for service:", err)
		return err
	}

	record = `{"host":"` + r.serviceDomainName(service) + `"}`
	_, err = r.client.Set(domainPath(raddr), record, uint64(service.TTL))
	if err != nil {
		log.Println("skydns2: failed to register reverse DNS entry:", err)
	}
	return err
}

func (r *Skydns2Adapter) Deregister(service *bridge.Service) error {
	_, err := r.client.Delete(r.servicePath(service), false)
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}

	// A mapping was only setup if the service's IP is unique
	if service.Origin.ExposedIP != service.IP {
		return err
	}

	raddr, err := reverseDomainName(service.IP)
	if err != nil {
		log.Println("skydns2: failed to determine reverse DNS entry for service:", err)
		return err
	}

	_, err = r.client.Delete(domainPath(raddr), false)
	if err != nil {
		log.Println("skydns2: Failed to deregister reverse DNS entry")
	}
	return err
}

func (r *Skydns2Adapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *Skydns2Adapter) servicePath(service *bridge.Service) string {
	return r.path + "/" + service.Name + "/" + service.ID
}

func (r *Skydns2Adapter) serviceDomainName(service *bridge.Service) string {
	return service.ID + "." + service.Name + "." + r.domain
}

func domainPath(domain string) string {
	components := strings.Split(domain, ".")
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return "/skydns/" + strings.Join(components, "/")
}

func reverseDomainName(ip string) (string, error) {
	const IPv4Domain = "in-addr.arpa."
	const IPv6Domain = "ip6.arpa."

	addr := net.ParseIP(ip)
	if addr == nil {
		return "", errors.New("Invalid IP Address")
	}

	ip4addr := addr.To4()
	if ip4addr != nil {
		domainName := make([]byte, 0, len(ip4addr)*4+len(IPv4Domain))
		for x := len(ip4addr) - 1; x >= 0; x-- {
			domainName = strconv.AppendUint(domainName, uint64(ip4addr[x]), 10)
			domainName = append(domainName, '.')
		}
		domainName = append(domainName, IPv4Domain...)

		return string(domainName), nil
	}

	ip16addr := addr.To16()
	if ip16addr != nil {
		domainName := make([]byte, 0, len(ip16addr)*4+len(IPv6Domain))
		for x := len(ip16addr) - 1; x >= 0; x-- {
			component := uint64(ip16addr[x])

			domainName = strconv.AppendUint(domainName, component&0x0f, 16)
			domainName = append(domainName, '.')
			domainName = strconv.AppendUint(domainName, component>>4, 16)
			domainName = append(domainName, '.')
		}
		domainName = append(domainName, IPv6Domain...)

		return string(domainName), nil
	}

	return "", errors.New("Unsupported IP address format")
}
