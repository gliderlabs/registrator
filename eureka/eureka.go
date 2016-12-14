package eureka

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/gliderlabs/registrator/aws"
	"github.com/gliderlabs/registrator/bridge"
	eureka "github.com/hudl/fargo"
)

const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "eureka")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	client := eureka.EurekaConnection{}
	if uri.Host != "" {
		client = eureka.NewConn("http://" + uri.Host + uri.Path)
	} else {
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
	var awsMetadata *aws.Metadata
	uniqueId := service.IP + ":" + strconv.Itoa(service.Port) + "_" + service.Origin.ContainerID

	registration.HostName = uniqueId
	registration.App = service.Name
	registration.Port = service.Port

	if service.Attrs["eureka_status"] == string(eureka.DOWN) {
		registration.Status = eureka.DOWN
	} else {
		registration.Status = eureka.UP
	}

	// Set the renewal interval in seconds, or default 30
	if service.Attrs["eureka_leaseinfo_renewalintervalinsecs"] != "" {
		v, err := strconv.Atoi(service.Attrs["eureka_leaseinfo_renewalintervalinsecs"])
		if err != nil {
			log.Println("eureka: Renewal interval must be valid int", err)
		} else {
			registration.LeaseInfo.RenewalIntervalInSecs = int32(v)
		}
	} else {
		registration.LeaseInfo.RenewalIntervalInSecs = 30
	}

	// Set the lease expiry timeout, or default 90
	if service.Attrs["eureka_leaseinfo_durationinsecs"] != "" {
		v, err := strconv.Atoi(service.Attrs["eureka_leaseinfo_durationinsecs"])
		if err != nil {
			log.Println("eureka: Lease duration must be valid int", err)
		} else {
			registration.LeaseInfo.DurationInSecs = int32(v)
		}
	} else {
		registration.LeaseInfo.DurationInSecs = 90
	}

	// Set any arbitrary metadata.
	for k, v := range service.Attrs {
		if strings.HasPrefix(k, "eureka_metadata_") {
			key := strings.TrimPrefix(k, "eureka_metadata_")
			registration.SetMetadataString(key, string(v))
		}
	}

	// If you are not running locally, check AWS API for metadata
	if service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		awsMetadata = aws.GetMetadata()
		registration.DataCenterInfo.Name = eureka.Amazon
		registration.DataCenterInfo.Metadata = eureka.AmazonMetadataType{
			InstanceID:       awsMetadata.InstanceID,
			AvailabilityZone: awsMetadata.AvailabilityZone,
			PublicHostname:   awsMetadata.PublicHostname,
			PublicIpv4:       awsMetadata.PublicIP,
			LocalHostname:    awsMetadata.PrivateHostname,
			HostName:         awsMetadata.PrivateHostname,
			LocalIpv4:        awsMetadata.PrivateIP,
		}
	} else {
		registration.DataCenterInfo.Name = eureka.MyOwn
	}

	// If flag is set, register the AWS public IP as the endpoint instead of the private one
	if service.Attrs["eureka_register_aws_public_ip"] != "" && service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		v, err := strconv.ParseBool(service.Attrs["eureka_register_aws_public_ip"])
		if err != nil {
			log.Printf("eureka: eureka_register_aws_public_ip must be valid boolean, was %v : %s", v, err)
		} else {
			registration.IPAddr = ShortHandTernary(service.Attrs["eureka_ipaddr"], awsMetadata.PublicIP)
			registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], awsMetadata.PublicIP)
		}
	} else {
		registration.IPAddr = ShortHandTernary(service.Attrs["eureka_ipaddr"], service.IP)
		registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], service.IP)
	}

	// If specified, lookup the ELBv2 (application load balancer) DNS name and port, use these to register with eureka, instead of the container itself
	if service.Attrs["eureka_use_elbv2_endpoint"] != "" && service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		v, err := strconv.ParseBool(service.Attrs["eureka_use_elbv2_endpoint"])
		if err != nil {
			log.Printf("eureka: eureka_use_elbv2_endpoint must be valid boolean, was %v : %s", v, err)
		} else {
			elbMetadata := aws.GetELBV2ForContainer(awsMetadata.InstanceID, int64(service.Port))
			registration.HostName = elbMetadata.DNSName
			registration.Port = int(elbMetadata.Port)
		}
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
	registration.App = service.Name
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
	if string1 != "" {
		return string1
	} else {
		return string2
	}
}
