package main

import (
	"bytes"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/coreos/go-etcd/etcd"
)

const templatePrefix = "ETCD_TMPL"

type EtcdTemplateRegistry struct {
	client *etcd.Client
	templates []*template.Template
	path   string
}

func NewEtcdTemplateRegistry(uri *url.URL) ServiceRegistry {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}

	// Here's part of the magic. Find all environment variables that start with ETCD_TMPL and turn them into templates
	templates := []*template.Template {}
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, templatePrefix) {
			text := strings.SplitN(env, "=", 2)[1]
			templates = append(templates, template.Must(template.New("etcd template").Parse(text)))
		}
	}

	return &EtcdTemplateRegistry{client: etcd.NewClient(urls), templates: templates, path: uri.Path}
}

func (r *EtcdTemplateRegistry) Register(service *Service) error {
	toSet, err := r.executeTemplates(service)
	if err != nil {
		return err
	}

	for key, value := range toSet {
		_, err = r.client.Set(key, value, uint64(service.TTL))
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *EtcdTemplateRegistry) Deregister(service *Service) error {
	toSet, err := r.executeTemplates(service)
	if err != nil {
		return err
	}

	for key, _ := range toSet {
		_, err = r.client.Delete(key, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *EtcdTemplateRegistry) Refresh(service *Service) error {
	return r.Register(service)
}

func (r *EtcdTemplateRegistry) executeTemplates(service *Service) (map[string]string, error) {
	results := make(map[string]string, len(r.templates))
	buf := &bytes.Buffer {}
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
			if len(key) > 0 && len (value) > 0 {
				results[key] = value
			}
		}
	}

	return results, nil
}