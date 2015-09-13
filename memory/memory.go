package memory

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "memory")
}

// Factory is a factory for InMemoryAdapters
type Factory struct{}

// New creates a new InMemoryAdapter
func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	adapter := &InMemoryAdapter{
		services: make(map[string]*bridge.Service),
	}

	go func() {
		http.HandleFunc("/service/", adapter.getService)
		http.HandleFunc("/services", adapter.getServices)
		err := http.ListenAndServe(":8500", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	return adapter
}

// InMemoryAdapter is a backend that maintains a list of services in memory and
// exposes then via a simple HTTP server
type InMemoryAdapter struct {
	sync.RWMutex
	services map[string]*bridge.Service
}

func (m *InMemoryAdapter) getService(w http.ResponseWriter, req *http.Request) {
	m.RLock()
	defer m.RUnlock()

	serviceName := strings.Replace(req.URL.Path, "/service/", "", -1)
	services := map[string]*bridge.Service{}

	for k, v := range m.services {
		if v.Name == serviceName {
			services[k] = v
		}
	}

	if len(services) == 0 {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	raw, err := json.Marshal(services)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func (m *InMemoryAdapter) getServices(w http.ResponseWriter, req *http.Request) {
	m.RLock()
	defer m.RUnlock()
	raw, err := json.Marshal(m.services)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

// Register a new service
func (m *InMemoryAdapter) Register(service *bridge.Service) (err error) {
	m.Lock()
	defer m.Unlock()
	m.services[service.ID] = service
	return
}

// Deregister a service
func (m *InMemoryAdapter) Deregister(service *bridge.Service) (err error) {
	m.Lock()
	defer m.Unlock()
	delete(m.services, service.ID)
	return
}

// Ping the adapter
func (m *InMemoryAdapter) Ping() (err error) {
	return
}

// Refresh a service
func (m *InMemoryAdapter) Refresh(service *bridge.Service) (err error) {
	return m.Register(service)
}
