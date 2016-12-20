package aws

import (
	"fmt"
	"log"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/gliderlabs/registrator/bridge"
	eureka "github.com/hudl/fargo"
)

// LBInfo represents a ELBv2 endpoint
type LBInfo struct {
	DNSName string
	Port    int64
}

// GetELBV2ForContainer returns an LBInfo struct with the load balancer DNS name and listener port for a given instanceId and port
// if an error occurs, or the target is not found, an empty LBInfo is returned. Return the DNS:port pair as an identifier to put in the container's registration metadata
// Pass it the instanceID for the docker host, and the the host port to lookup the associated ELB.
func GetELBV2ForContainer(instanceID string, port int64) LBInfo {

	var lb []*string
	var lbPort *int64
	info := LBInfo{}

	sess, err := session.NewSession()
	if err != nil {
		fmt.Printf("Failed to create session connecting to AWS: %s\n", err)
		return info
	}
	svc := elbv2.New(sess)

	// Loop through target group pages and check for port and instanceID
	// TODO: This needs to handle lots of target groups efficiently
	params := &elbv2.DescribeTargetGroupsInput{
		PageSize: awssdk.Int64(1000),
	}
	tgs, err := svc.DescribeTargetGroups(params)

	if err != nil {
		log.Printf("An error occurred using DescribeTargetGroups: %s \n", err.Error())
		return info
	}

	// Check each target group for a matching port and instanceID
	// Assumption: that that there is only one LB for the target group (though the data structure allows more)
	for _, tg := range tgs.TargetGroups {
		params4 := &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: awssdk.String(*tg.TargetGroupArn),
		}
		tarH, err := svc.DescribeTargetHealth(params4)

		for _, thd := range tarH.TargetHealthDescriptions {
			if *thd.Target.Port == port && *thd.Target.Id == instanceID {
				lb = tg.LoadBalancerArns
				lbPort = tg.Port
			}

		}
		if err != nil {
			log.Printf("An error occurred using DescribeTargetHealth: %s \n", err.Error())
			return info
		}
		fmt.Printf("LB is: %v\n", *lb[0])
		fmt.Printf("LB Port is: %v\n", *lbPort)
	}

	params2 := &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: lb,
	}
	lbData, err := svc.DescribeLoadBalancers(params2)

	if err != nil {
		log.Printf("An error occurred using DescribeLoadBalancers: %s \n", err.Error())
		return info
	}
	fmt.Printf("LB DNS: %s\n", *lbData.LoadBalancers[0].DNSName)

	info.DNSName = *lb[0]
	info.Port = *lbPort
	return info
}

// CheckELBFlags - Helper function to check if the correct config flags are set to use ELBs
func CheckELBFlags(service *bridge.Service) bool {
	if service.Attrs["eureka_use_elbv2_endpoint"] != "" && service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		v, err := strconv.ParseBool(service.Attrs["eureka_use_elbv2_endpoint"])
		if err != nil {
			log.Printf("eureka: eureka_use_elbv2_endpoint must be valid boolean, was %v : %s", v, err)
			return false
		} else {
			return true
		}
	} else {
		return false
	}
}

// RegisterELBv2 - If specified, also register an ELBv2 (application load balancer, ALB) endpoint in eureka, and alter service name
// for container registrations.  This will mean traffic is directed to the ALB rather than directly to containers
// though they are still registered in eureka for information purposes
func RegisterELBv2(service *bridge.Service, registration *eureka.Instance, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		log.Printf("Found ELBv2 flags, will attempt to register LB for: %s\n", registration.HostName)
		awsMetadata := GetMetadata()
		elbMetadata := GetELBV2ForContainer(awsMetadata.InstanceID, int64(registration.Port))
		if elbMetadata.DNSName == "" {
			log.Printf("Unable to find associated ELBv2 for: %s\n", registration.HostName)
			return
		}
		elbEndpoint := elbMetadata.DNSName + ":" + string(elbMetadata.Port)

		registration.SetMetadataString("has_elbv2", "true")
		registration.SetMetadataString("elbv2_endpoint", elbEndpoint)

		elbReg := new(eureka.Instance)
		// Put a little metadata in here as required - setting hostname to the ELB endpoint prevents double registration
		elbReg.DataCenterInfo.Name = eureka.Amazon

		elbReg.DataCenterInfo.Metadata = eureka.AmazonMetadataType{
			PublicHostname: elbEndpoint,
			HostName:       elbEndpoint,
		}

		elbReg.Port = int(elbMetadata.Port)
		elbReg.IPAddr = elbMetadata.DNSName
		elbReg.App = service.Name
		elbReg.VipAddress = elbReg.IPAddr
		elbReg.HostName = elbEndpoint
		elbReg.DataCenterInfo.Name = eureka.Amazon
		elbReg.SetMetadataString("is_elbv2", "true")
		client.RegisterInstance(elbReg)
	}
}

// DeregisterELBv2 - If specified, and all containers are gone, also deregister the ELBv2 (application load balancer, ALB) endpoint in eureka.
//
func DeregisterELBv2(service *bridge.Service, regDNSName string, regPort int64, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		// Check if there are any containers around with this ALB still attached
		log.Printf("Found ELBv2 flags, will check if it needs to be deregistered too, for: %s:%v\n", regDNSName, string(regPort))
		albName := regDNSName + ":" + string(regPort)
		appName := "CONTAINER_" + service.Name
		app, err := client.GetApp(appName)
		if app != nil {
			for _, instance := range app.Instances {
				val, err := instance.Metadata.GetString("elbv2_endpoint")
				if err == nil && val == albName {
					log.Printf("Eureka entry still present for one or more ALB linked containers: %s\n", val)
					return
				}
			}
		}
		if err != nil {
			log.Printf("Unable to retrieve app metadata for %s: %s\n", appName, err)
		}

		if app == nil {
			elbReg := new(eureka.Instance)
			elbReg.IPAddr = regDNSName
			elbReg.App = service.Name
			elbReg.HostName = elbReg.IPAddr
			client.DeregisterInstance(elbReg)
		}
	}
}

// HeartbeatELBv2 - Send a heartbeat to eureka for this ELBv2 registration.  Every host running registrator will send heartbeats, meaning they will
// be received more frequently than the --ttl-refresh interval if there are multiple hosts running registrator.
//
func HeartbeatELBv2(service *bridge.Service, registration *eureka.Instance, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		awsMetadata := GetMetadata()
		elbMetadata := GetELBV2ForContainer(awsMetadata.InstanceID, int64(registration.Port))
		log.Printf("Heartbeating ELBv2: %s:%s (for container: %s)\n", elbMetadata.DNSName, string(elbMetadata.Port), registration.HostName)
		elbReg := new(eureka.Instance)
		elbReg.IPAddr = elbMetadata.DNSName
		elbReg.App = service.Name
		elbReg.HostName = elbReg.IPAddr
		client.HeartBeatInstance(elbReg)
	}
}
