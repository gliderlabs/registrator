package f5

import (
	"net"
	"net/url"
	"log"
	"strconv"
	"strings"
	"regexp"
	"os"

	"github.com/gliderlabs/registrator/bridge"
	bigip "github.com/scottdware/go-bigip"
)

func init() {
	bridge.Register(new(Factory), "bigip")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	var user, password string
	if uri.User == nil {
		log.Fatal("Username/password required. bigip://user:password@host")
	} else {
		user = uri.User.Username()
		password,_ = uri.User.Password()
	}

	REQUIRE_NAME := os.Getenv("REQUIRE_NAME")
	
	return &BigIpAdapter {
		client: bigip.NewSession(uri.Host, user, password), 
		namedPoolOnly: strings.ToLower(REQUIRE_NAME) == "true",
	}	
}

type BigIpAdapter struct {
	namedPoolOnly bool //only add nodes with an explictly defined POOL_NAME
	client *bigip.BigIP
}

func (r *BigIpAdapter) sanitizeName(name string) string {
	newName := strings.Replace(name, ":", "_", -1)
	//force name to start with a letter (F5 naming requirement)
	if match,_ := regexp.MatchString("^[a-zA-Z].*", newName); !match {
		return "A" + newName
	}
	return newName
}

func (r *BigIpAdapter) hasNamedPool(service *bridge.Service) bool {
	v, ok := service.Attrs["name"]
	return ok && v != ""
}

func (r *BigIpAdapter) buildPoolName(service *bridge.Service) string {
	v, ok := service.Attrs["name"]
	if !ok || v == "" {
		return r.sanitizeName(service.Name)
	}
	return r.sanitizeName(v)
}

func (r *BigIpAdapter) buildNode(service *bridge.Service) (string,string) {
	nodeName := r.sanitizeName(strings.Join(strings.Split(service.ID, ":")[0:2], "_"))
	port := strconv.Itoa(service.Port)
	poolMember := net.JoinHostPort(nodeName, port)
	return nodeName, poolMember
}

func (r *BigIpAdapter) Ping() error {
	_, err := r.client.Pools()
	return err
}

func (r *BigIpAdapter) Register(service *bridge.Service) error {
	if r.namedPoolOnly && !r.hasNamedPool(service) {
		log.Printf("ignored: %s no SERVICE_NAME defined", service.ID)
		return nil
	}

	poolName := r.buildPoolName(service)
	pool, err := r.client.GetPool(poolName)
	if err != nil {
    	return err
    }
	if pool == nil {
    	log.Printf("Creating pool %s", poolName)
    	err := r.client.CreatePool(poolName)
    	if err != nil {
    		return err
    	}
    }

    nodeName,poolMember := r.buildNode(service)
    node, err := r.client.GetNode(nodeName)
    if err != nil {
    	return err
    }
    if node == nil || node.Address != service.IP {
    	if node != nil {
    		err := r.Deregister(service)
    		if err != nil {
		    	return err
		    }
    	}
		err := r.client.CreateNode(nodeName, service.IP)
		if err != nil {
			return err
		}
		log.Printf("Created node %s -> %s", nodeName, service.IP)
	}

    poolMembers, err := r.client.PoolMembers(poolName)
    if err != nil {
    	return err
    }
    for _, member := range poolMembers {
        if strings.HasPrefix(member, nodeName) {
        	err := r.client.DeletePoolMember(poolName, member)
			if err != nil {
				return err
			}
			break
        }
    }
    err = r.client.AddPoolMember(poolName, poolMember)
    if err != nil {
    	return err
    }

    //if check_port := service.Attrs["check_port"]; check_port != "" {
    	//TODO: add monitor to node support?
    	// how do we upgrade monitor configs? seems like this would be better to just specify per node 
    	// but this could lead to blowing out the monitor configs on the bigIp?
    	// docs say health check support is getting revamped in registrator anyway, so 
    	// we'll just leave this as TBD
    //	f5.CreateMonitor("web_http", "http", 5, 16, "GET /\r\n", "200 OK")
    //	f5.AddMonitorToPool("web_http", "web_80_pool")
    //}

	return err
}

func (r *BigIpAdapter) Deregister(service *bridge.Service) error {
	if r.namedPoolOnly && !r.hasNamedPool(service) {
		return nil
	}

	poolName := r.buildPoolName(service)
	nodeName,poolMember := r.buildNode(service)

	err := r.client.DeletePoolMember(poolName, poolMember)
	if err != nil {
		return err
	}

	err = r.client.DeleteNode(nodeName)
	if err != nil {
		return err
	}
	return err
}

func (r *BigIpAdapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}