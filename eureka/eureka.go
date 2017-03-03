package eureka

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	aws "github.com/gliderlabs/registrator/aws"
	"github.com/gliderlabs/registrator/bridge"
	fargo "github.com/hudl/fargo"
)

const DefaultInterval = "10s"

func init() {
	bridge.Register(new(Factory), "eureka")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	client := fargo.EurekaConnection{}
	if uri.Host != "" {
		client = fargo.NewConn("http://" + uri.Host + uri.Path)
	} else {
		client = fargo.NewConn("http://eureka:8761")
	}
	return &EurekaAdapter{client: client}
}

type EurekaAdapter struct {
	client fargo.EurekaConnection
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

// Note: This is a function that is passed to the fargo library to determine how each registration is identified in eureka
func GetUniqueID(instance fargo.Instance) string {
	return instance.HostName + "_" + strconv.Itoa(instance.Port)
}

// Helper function to check a boolean metadata flag
func checkBooleanFlag(service *bridge.Service, flag string) bool {
	if service.Attrs[flag] != "" {
		v, err := strconv.ParseBool(service.Attrs[flag])
		if err != nil {
			log.Printf("eureka: %s must be valid boolean, was %v : %s", flag, v, err)
			return false
		}
		return true
	}
	return false
}

func instanceInformation(service *bridge.Service) *fargo.Instance {

	registration := new(fargo.Instance)
	var awsMetadata *aws.Metadata

	registration.UniqueID = GetUniqueID
	registration.App = service.Name
	registration.Port = service.Port

	if service.Attrs["eureka_status"] == string(fargo.DOWN) {
		registration.Status = fargo.DOWN
	} else {
		registration.Status = fargo.UP
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

	// Metadata flag for a container
	registration.SetMetadataString("is-container", string("true"))
	registration.SetMetadataString("container-id", service.Origin.ContainerID)
	registration.SetMetadataString("container-name", service.Origin.ContainerName)

	// If AWS metadata collection is enabled, use it
	if service.Attrs["eureka_datacenterinfo_name"] != fargo.MyOwn && checkBooleanFlag(service, "eureka_datacenterinfo_auto_populate") {
		awsMetadata = aws.GetMetadata()
		registration.HostName = awsMetadata.PrivateHostname
		// Set the instanceID here, because we don't want eureka to use it as a uniqueID
		registration.SetMetadataString("aws-instance-id", awsMetadata.InstanceID)
		registration.DataCenterInfo.Name = fargo.Amazon
		registration.DataCenterInfo.Metadata = fargo.AmazonMetadataType{
			AvailabilityZone: awsMetadata.AvailabilityZone,
			PublicHostname:   awsMetadata.PublicHostname,
			PublicIpv4:       awsMetadata.PublicIP,
			InstanceID:       GetUniqueID(*registration), // This is deliberate - due to limitations in uniqueIDs
			LocalHostname:    awsMetadata.PrivateHostname,
			HostName:         awsMetadata.PrivateHostname,
			LocalIpv4:        awsMetadata.PrivateIP,
		}
		// Here we don't want auto population of metadata from AWS.  We'll use what we have from registrator, or overrides
	} else if service.Attrs["eureka_datacenterinfo_name"] != fargo.MyOwn && !checkBooleanFlag(service, "eureka_datacenterinfo_auto_populate") {
		registration.DataCenterInfo.Name = fargo.Amazon
		registration.HostName = ShortHandTernary(service.Attrs["eureka_datacenterinfo_localhostname"], service.IP)
		registration.DataCenterInfo.Metadata = fargo.AmazonMetadataType{
			InstanceID:     GetUniqueID(*registration), // This is deliberate - due to limitations in uniqueIDs
			PublicHostname: ShortHandTernary(service.Attrs["eureka_datacenterinfo_publichostname"], service.Origin.HostIP),
			PublicIpv4:     ShortHandTernary(service.Attrs["eureka_datacenterinfo_publicipv4"], service.Origin.HostIP),
			LocalHostname:  ShortHandTernary(service.Attrs["eureka_datacenterinfo_localhostname"], service.IP),
			HostName:       ShortHandTernary(service.Attrs["eureka_datacenterinfo_localhostname"], service.IP),
			LocalIpv4:      ShortHandTernary(service.Attrs["eureka_datacenterinfo_localipv4"], service.IP),
		}
	} else {
		registration.DataCenterInfo.Name = fargo.MyOwn
		// We don't have a uniqueID, so manipulate hostname to provide it there.
		registration.HostName = service.IP
		registration.HostName = GetUniqueID(*registration)
	}

	// If flag is set, register the AWS public IP as the endpoint instead of the private one
	if checkBooleanFlag(service, "eureka_register_aws_public_ip") && checkBooleanFlag(service, "eureka_datacenterinfo_auto_populate") && service.Attrs["eureka_datacenterinfo_name"] != fargo.MyOwn {
		registration.IPAddr = ShortHandTernary(service.Attrs["eureka_ipaddr"], awsMetadata.PublicIP)
		registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], awsMetadata.PublicIP)
	} else {
		registration.IPAddr = ShortHandTernary(service.Attrs["eureka_ipaddr"], service.IP)
		registration.VipAddress = ShortHandTernary(service.Attrs["eureka_vip"], service.IP)
	}

	return registration
}

func (r *EurekaAdapter) Register(service *bridge.Service) error {
	registration := instanceInformation(service)
	var instance error
	if aws.CheckELBFlags(service) {
		instance = aws.RegisterWithELBv2(service, registration, r.client)
	} else {
		instance = r.client.RegisterInstance(registration)
	}
	return instance
}

func (r *EurekaAdapter) Deregister(service *bridge.Service) error {
	registration := instanceInformation(service)
	if aws.CheckELBFlags(service) {
		aws.RemoveLBCache(service.Origin.ContainerID)
	}
	log.Println("Deregistering", GetUniqueID(*registration))
	instance := r.client.DeregisterInstance(registration)
	return instance
}

func (r *EurekaAdapter) Refresh(service *bridge.Service) error {
	registration := instanceInformation(service)
	if aws.CheckELBFlags(service) {
		aws.HeartbeatELBv2(service, registration, r.client)
		return nil
	} else {
		err := r.client.HeartBeatInstance(registration)
		log.Println("Done heartbeating for:", GetUniqueID(*registration))
		return err
	}
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
