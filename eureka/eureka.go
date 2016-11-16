package eureka

import (
	"log"
	"net/url"
	"github.com/gliderlabs/registrator/bridge"
	eureka "github.com/hudl/fargo"
	"strconv"
)
const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "eureka")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	client := eureka.EurekaConnection{}
	if uri.Host != "" {
		client = eureka.NewConn("http://"+uri.Host+uri.Path)
	}else {
		client = eureka.NewConn("http://eureka:8761")
	}

	return &EurekaAdapter{client: client}
}

type EurekaAdapter struct {
	client eureka.EurekaConnection
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

func instanceInformation(service *bridge.Service) *eureka.Instance {

	registration := new(eureka.Instance)
	uniqueId := service.IP + ":" + strconv.Itoa(service.Port)

	registration.HostName   = uniqueId
	registration.App        = service.Name
	registration.Port       = service.Port
	registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], service.Name)

	if(service.Attrs["eureka_status"] == string(eureka.DOWN)) {
		registration.Status = eureka.DOWN
	} else {
		registration.Status = eureka.UP
	}

	if(service.Attrs["eureka_leaseinfo_renewalintervalinsecs"] != "") {
		v, err := strconv.Atoi(service.Attrs["eureka_leaseinfo_renewalintervalinsecs"])
		if(err != nil) {
			log.Println("eureka: Renewal interval must be valid int", err)
		} else {
			registration.LeaseInfo.RenewalIntervalInSecs = int32(v)
		}
	} else {
		registration.LeaseInfo.RenewalIntervalInSecs = 30
	}

	if(service.Attrs["eureka_leaseinfo_durationinsecs"] != "") {
		v, err := strconv.Atoi(service.Attrs["eureka_leaseinfo_durationinsecs"])
		if(err != nil) {
			log.Println("eureka: Lease duration must be valid int", err)
		} else {
			registration.LeaseInfo.DurationInSecs = int32(v)
		}
	} else {
		registration.LeaseInfo.DurationInSecs = 90
	}

	if service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		registration.DataCenterInfo.Name = eureka.Amazon
		registration.DataCenterInfo.Metadata = eureka.AmazonMetadataType {
			InstanceID:	uniqueId,
			PublicHostname: ShortHandTernary(service.Attrs["eureka_datacenterinfo_publichostname"], service.Origin.HostIP),
			PublicIpv4:     ShortHandTernary(service.Attrs["eureka_datacenterinfo_publicipv4"], service.Origin.HostIP),
			LocalHostname:  ShortHandTernary(service.Attrs["eureka_datacenterinfo_localipv4"], service.IP),
			LocalIpv4:      ShortHandTernary(service.Attrs["eureka_datacenterinfo_localhostname"], service.IP),
		}
	} else {
		registration.DataCenterInfo.Name = eureka.MyOwn
	}


	return registration
}

func (r *EurekaAdapter) Register(service *bridge.Service) error {
	registration := instanceInformation(service)
	return r.client.RegisterInstance(registration)
}

func (r *EurekaAdapter) Deregister(service *bridge.Service) error {
	registration := new(eureka.Instance)
	registration.HostName = service.IP + ":" + strconv.Itoa(service.Port)
	log.Println("Deregistering ", registration.HostName)
	return r.client.DeregisterInstance(registration)
}

func (r *EurekaAdapter) Refresh(service *bridge.Service) error {
	log.Println("Heartbeating...")
	registration := instanceInformation(service)
	err := r.client.HeartBeatInstance(registration)
	log.Println("Done heartbeating for: ", registration.HostName)
	return err
}

func (r *EurekaAdapter) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}

func ShortHandTernary(string1 string, string2 string) string {
	if(string1 != "") {
		return string1
	} else {
		return string2
	}
}
