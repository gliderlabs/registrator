package zookeeper

import (
	"encoding/json"
	"log"
	"net/url"
	"strconv"
	"time"

	"github.com/gliderlabs/registrator/bridge"
	"github.com/samuel/go-zookeeper/zk"
)

func init() {
	bridge.Register(new(Factory), "zookeeper")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	c, _, err := zk.Connect([]string{uri.Host}, (time.Second * 10))
	if err != nil {
		panic(err)
	}
	exists, _, err := c.Exists(uri.Path)
	if err != nil {
		log.Println("zookeeper: error checking if base path exists:", err)
	}
	if !exists {
		c.Create(uri.Path, []byte{}, 0, zk.WorldACL(zk.PermAll))
	}
	return &ZkAdapter{client: c, path: uri.Path}
}

type ZkAdapter struct {
	client *zk.Conn
	path   string
}

type ZnodeBody struct {
	Name        string
	IP          string
	PublicPort  int
	PrivatePort int
	ContainerID string
	Tags        []string
	Attrs       map[string]string
}

func (r *ZkAdapter) Register(service *bridge.Service) error {
	privatePort, _ := strconv.Atoi(service.Origin.ExposedPort)
	acl := zk.WorldACL(zk.PermAll)

	exists, _, err := r.client.Exists(r.path + "/" + service.Name)
	if err != nil {
		log.Println("zookeeper: error checking if exists: ", err)
	} else {
		if !exists {
			_, err := r.client.Create(r.path+"/"+service.Name, []byte{}, 0, acl)
			if err != nil {
				log.Println("zookeeper: failed to create base service node: ", err)
			} else {
				zbody := &ZnodeBody{Name: service.Name, IP: service.IP, PublicPort: service.Port, PrivatePort: privatePort, Tags: service.Tags, Attrs: service.Attrs, ContainerID: service.Origin.ContainerHostname}
				body, err := json.Marshal(zbody)
				if err != nil {
					log.Println("zookeeper: failed to json encode service body: ", err)
				} else {
					path := r.path + "/" + service.Name + "/" + service.Origin.ExposedPort
					_, err = r.client.Create(path, body, 1, acl)
					if err != nil {
						log.Println("zookeeper: failed to register service: ", err)
					}
				} // json encode error check
			} // create service path error check
		} // service path exists
	} // service path exists error check
	return err
}

func (r *ZkAdapter) Ping() error {
	_, _, err := r.client.Exists("/")
	if err != nil {
		log.Println("zookeeper: error on ping check for Exists(/): ", err)
		return err
	}
	return nil
}

func (r *ZkAdapter) Deregister(service *bridge.Service) error {
	basePath := r.path + "/" + service.Name
	// Delete the service-port znode
	servicePortPath := basePath + "/" + service.Origin.ExposedPort
	err := r.client.Delete(servicePortPath, -1) // -1 means latest version number
	if err != nil {
		log.Println("zookeeper: failed to deregister service port entry: ", err)
	}
	// Check if all service-port znodes are removed.
	children, _, err := r.client.Children(basePath)
	if len(children) == 0 {
		// Delete the service name znode
		err := r.client.Delete(basePath, -1)
		if err != nil {
			log.Println("zookeeper: failed to delete service path: ", err)
		}
	}
	return err
}

func (r *ZkAdapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *ZkAdapter) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}
