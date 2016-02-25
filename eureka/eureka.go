package eureka

import (
	"log"
	"net/url"
	"github.com/gliderlabs/registrator/bridge"
	Eureka "github.com/hudl/fargo"
)
const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "eureka")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	client := Eureka.EurekaConnection{}
	if uri.Host != "" {
		client = Eureka.NewConn("http://"+uri.Host+uri.Path)
	}else {
		client = Eureka.NewConn("http://eureka:8761")
	}

	return &EurekaAdapter{client: client}
}

type EurekaAdapter struct {
	client Eureka.EurekaConnection
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *EurekaAdapter) Ping() error {

	eurekaApps, err := r.client.GetApps()
	if err != nil {
		return err
	}
	log.Println("eureka: current apps ", len(eurekaApps))

	return nil
}

func instanceInformation(service *bridge.Service) *Eureka.Instance {

	registration := new(Eureka.Instance)

	registration.HostName   = service.IP
	registration.App        = service.Name
	registration.Port       = service.Port
	registration.Status     = ReturnIfSet(service.Attrs["eureka_status"], string(Eureka.UP))
	registration.VipAddress = ReturnIfSet(service.Attrs["eureka_vip"], service.Name)

	registration.LeaseInfo.RenewalIntervalInSecs = ReturnIfSet(service.Attrs["eureka_leaseinfo_renewalintervalinsecs"], 30)
	registration.LeaseInfo.DurationInSecs        = ReturnIfSet(service.Attrs["eureka_leaseinfo_durationinsecs"], 90)

	if service.Attrs["eureka_datacenterinfo_name"] != Eureka.MyOwn {
		registration.DataCenterInfo.Name = Eureka.Amazon
		registration.DataCenterInfo.Metadata = Eureka.AmazonMetadataType {
			PublicHostname: ReturnIfSet(service.Attrs["eureka_datacenterinfo_publichostname"], service.Origin.HostIP),
			PublicIpv4:     ReturnIfSet(service.Attrs["eureka_datacenterinfo_publicipv4"], service.Origin.HostIP),
			LocalHostname:  ReturnIfSet(service.Attrs["eureka_datacenterinfo_localipv4"], service.IP),
			LocalIpv4:      ReturnIfSet(service.Attrs["eureka_datacenterinfo_localhostname"], service.IP),
		}
	} else {
		registration.DataCenterInfo.Name = Eureka.MyOwn
	}


	return registration
}

func (r *EurekaAdapter) Register(service *bridge.Service) error {
	registration := instanceInformation(service)
	return r.client.RegisterInstance(registration)
}

func (r *EurekaAdapter) Deregister(service *bridge.Service) error {
	registration := new(Eureka.Instance)
	registration.HostName = service.ID
	return r.client.DeregisterInstance(registration)
}

func (r *EurekaAdapter) Refresh(service *bridge.Service) error {
	registration := instanceInformation(service)
	return r.client.ReregisterInstance(registration)
}

func (r *EurekaAdapter) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}

func ReturnIfSet(string1 string, string2 string) string {
	if(string1 != "") {
		return string1
	} else {
		return string2
	}
}