package eureka

import (
	"github.com/gliderlabs/registrator/bridge"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	eureka "github.com/hudl/fargo"
	"log"
	"net/url"
	"strconv"
	"strings"
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

type AWSMetadata struct {
	InstanceID string
	PrivateIP string
	PublicIP string
	PrivateHostname string
	PublicHostname string
	AvailabilityZone string
} 

func getDataOrFail(svc *ec2metadata.EC2Metadata, key string) string {
	val, err := svc.GetMetadata(key)
	if err != nil {
		log.Printf("Unable to retrieve %s from the EC2 instance: %s\n", key, err)
		return ""
	}
	return val
}

func getAWSMetadata() *AWSMetadata {
	log.Println("Attempting to retrieve AWS metadata.")
	sess, err := session.NewSession()
	if err != nil {
		log.Printf("Unable to connect to the EC2 metadata service: %s\n", err)
	}
	svc := ec2metadata.New(sess)
	m := new(AWSMetadata)
	if svc.Available() {
		m.InstanceID = getDataOrFail(svc, "instance-id")
		m.PrivateIP = getDataOrFail(svc, "local-ipv4")
		m.PublicIP = getDataOrFail(svc, "public-ipv4")
		m.PrivateHostname = getDataOrFail(svc, "local-hostname")
		m.PublicHostname = getDataOrFail(svc, "public-hostname")
		m.AvailabilityZone = getDataOrFail(svc, "placement/availability-zone")
	} else {
		log.Println("AWS metadata not available :(")
	}
	return m
}

func instanceInformation(service *bridge.Service) *eureka.Instance {

	registration := new(eureka.Instance)
	uniqueId := service.IP + ":" + strconv.Itoa(service.Port) + "_" + service.Origin.ContainerID

	registration.HostName = uniqueId
	registration.App = service.Name
	registration.Port = service.Port

	if service.Attrs["eureka_status"] == string(eureka.DOWN) {
		registration.Status = eureka.DOWN
	} else {
		registration.Status = eureka.UP
	}

	if service.Attrs["eureka_register_aws_public_ip"] != "" {
		v, err := strconv.ParseBool(service.Attrs["eureka_register_aws_public_ip"])
		if err != nil {
			log.Printf("eureka: eureka_register_aws_public_ip must be valid boolean, was %s : %s", v, err)
		} else {
			awsMetadata := getAWSMetadata()
			registration.IPAddr = ShortHandTernary(service.Attrs["eureka_ipaddr"], awsMetadata.PublicIP)
			registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], awsMetadata.PublicIP)
		}
	} else {
		registration.IPAddr = ShortHandTernary(service.Attrs["eureka_ipaddr"], service.IP)
		registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], service.IP)
	}

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

	if service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		awsMetadata := getAWSMetadata()
		registration.DataCenterInfo.Name = eureka.Amazon
		registration.DataCenterInfo.Metadata = eureka.AmazonMetadataType{
			InstanceID:       	awsMetadata.InstanceID,
			AvailabilityZone:	awsMetadata.AvailabilityZone,
			PublicHostname:		awsMetadata.PublicHostname,
			PublicIpv4:     	awsMetadata.PublicIP,
			LocalHostname:  	awsMetadata.PrivateHostname,
			HostName:       	awsMetadata.PrivateHostname,
			LocalIpv4:      	awsMetadata.PrivateIP,
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