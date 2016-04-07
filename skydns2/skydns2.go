package skydns2

import (
	"log"
	"net/url"
	"strconv"
	"strings"
	"bytes"

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

	return &Skydns2Adapter{client: etcd.NewClient(urls), path: domainPath(uri.Path[1:])}
}

type Skydns2Adapter struct {
	client *etcd.Client
	path   string
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
	record := `{"host":"` + service.IP + `","port":"` + port + `"`
	if len(service.Tags) > 0 {
		record = record + `","tags":` + tagsToQuotedStringArray(service)
	}
	if len(service.Attrs) > 0 {
		record = record + `,"attrs":` + attrsToJson(service)
	}
	record = record + `}`
	_, err := r.client.Set(r.servicePath(service), record, uint64(service.TTL))
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Deregister(service *bridge.Service) error {
	_, err := r.client.Delete(r.servicePath(service), false)
	if err != nil {
		log.Println("skydns2: failed to register service:", err)
	}
	return err
}

func (r *Skydns2Adapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *Skydns2Adapter) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}

func (r *Skydns2Adapter) servicePath(service *bridge.Service) string {
	return r.path + "/" + service.Name + "/" + service.ID
}

func domainPath(domain string) string {
	components := strings.Split(domain, ".")
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return "/skydns/" + strings.Join(components, "/")
}

func Map(vs []string, f func(string) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}

func attrsToJson(service *bridge.Service) string {
	// Using buffer for concatenation is probably overkill
	// ..but just to be on the sure side
	var buffer bytes.Buffer
	buffer.WriteString(`{`)
	for key, value := range service.Attrs {
		buffer.WriteString(quoteString(key))
		buffer.WriteString(`:`)
		buffer.WriteString(quoteString(value))
		buffer.WriteString(`,`)
	}
	json := buffer.String()
	return strings.TrimSuffix(json, `,`) + `}`
}

func tagsToQuotedStringArray(service *bridge.Service) string {
	return `[`+ strings.Join(Map(service.Tags, quoteString), `,`) + `]`
}

func quoteString(input string) string {
	return `"` + input + `"`
}
