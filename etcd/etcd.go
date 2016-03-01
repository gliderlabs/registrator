package etcd

import (
	"bytes"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	etcd2 "github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
	etcd "gopkg.in/coreos/go-etcd.v0/etcd"
)

const templatePrefix = "ETCD_TMPL"

func init() {
	bridge.Register(new(Factory), "etcd")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	} else {
		urls = append(urls, "http://127.0.0.1:4001")
	}

	// Find all environment variables that start with ETCD_TMPL and turn them into templates. If there are no
	// templates defined then the etcd module will fall back to the default behavior of:
	// <path>/<service.Name>/<service.ID>
	templates := []*template.Template{}
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, templatePrefix) {
			text := strings.SplitN(env, "=", 2)[1]
			templates = append(templates, template.Must(template.New("etcd template").Parse(text)))
		}
	}

	res, err := http.Get(urls[0] + "/version")
	if err != nil {
		log.Fatal("etcd: error retrieving version", err)
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	if match, _ := regexp.Match("0\\.4\\.*", body); match == true {
		log.Println("etcd: using v0 client")
		return &EtcdAdapter{client: etcd.NewClient(urls), templates: templates, path: uri.Path}
	}

	return &EtcdAdapter{client2: etcd2.NewClient(urls), templates: templates, path: uri.Path}
}

type EtcdAdapter struct {
	client  *etcd.Client
	client2 *etcd2.Client

	templates []*template.Template

	path string
}

func (r *EtcdAdapter) Ping() error {
	var err error
	if r.client != nil {
		rr := etcd.NewRawRequest("GET", "version", nil, nil)
		_, err = r.client.SendRequest(rr)
	} else {
		rr := etcd2.NewRawRequest("GET", "version", nil, nil)
		_, err = r.client2.SendRequest(rr)
	}

	if err != nil {
		return err
	}
	return nil
}

func (r *EtcdAdapter) Register(service *bridge.Service) error {
	var err error
	if len(r.templates) < 1 {
		// Default behavior if no templates are registered
		path := r.path + "/" + service.Name + "/" + service.ID
		port := strconv.Itoa(service.Port)
		addr := net.JoinHostPort(service.IP, port)

		if r.client != nil {
			_, err = r.client.Set(path, addr, uint64(service.TTL))
		} else {
			_, err = r.client2.Set(path, addr, uint64(service.TTL))
		}
	} else {
		toSet, err := r.executeTemplates(service)
		if err == nil {
			for key, value := range toSet {
				if r.client != nil {
					_, err = r.client.Set(key, value, uint64(service.TTL))
				} else {
					_, err = r.client2.Set(key, value, uint64(service.TTL))
				}
				if err != nil {
					break
				}
			}
		}

	}

	if err != nil {
		log.Println("etcd: failed to register service:", err)
	}
	return err
}

func (r *EtcdAdapter) Deregister(service *bridge.Service) error {
	var err error
	if len(r.templates) < 1 {
		// Default behavior if no templates are registered
		path := r.path + "/" + service.Name + "/" + service.ID

		if r.client != nil {
			_, err = r.client.Delete(path, false)
		} else {
			_, err = r.client2.Delete(path, false)
		}
	} else {
		toSet, err := r.executeTemplates(service)
		if err == nil {
			for key, value := range toSet {
				if r.client != nil {
					_, err = r.client.Delete(key, false)
				} else {
					_, err = r.client2.Delete(key, false)
				}
				if err != nil {
					break
				}
			}
		}

	}

	if err != nil {
		log.Println("etcd: failed to deregister service:", err)
	}
	return err
}

func (r *EtcdAdapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *EtcdAdapter) executeTemplates(service *bridge.Service) (map[string]string, error) {
	results := make(map[string]string, len(r.templates))
	buf := &bytes.Buffer{}
	for _, t := range r.templates {
		// Execute the template with the service as the data item
		buf.Reset()
		err := t.Execute(buf, service)
		if err != nil {
			return nil, err
		}

		// The template needs to return "<key> <value>". The key must conform to the etcd spec and not contain any
		// spaces, so we use the first space as the split between the two. If nothing is returned, then that says
		// not to use that template
		pair := strings.SplitN(buf.String(), " ", 2)
		if 2 == len(pair) {
			key := strings.TrimSpace(pair[0])
			value := strings.TrimSpace(pair[1])
			if len(key) > 0 && len(value) > 0 {
				results[key] = value
			}
		}
	}

	return results, nil
}
