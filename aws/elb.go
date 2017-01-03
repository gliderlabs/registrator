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

// getELBV2ForContainer returns an LBInfo struct with the load balancer DNS name and listener port for a given instanceId and port
// if an error occurs, or the target is not found, an empty LBInfo is returned. Return the DNS:port pair as an identifier to put in the container's registration metadata
// Pass it the instanceID for the docker host, and the the host port to lookup the associated ELB.
func getELBV2ForContainer(instanceID string, port int64) (lbinfo *LBInfo, err error) {

	var lb []*string
	var lbPort *int64
	info := &LBInfo{}

	sess, err := session.NewSession()
	if err != nil {
		message := fmt.Errorf("Failed to create session connecting to AWS: %s", err)
		return nil, message
	}

	// Need to set the region here - we'll get it from instance metadata
	awsMetadata := GetMetadata()
	svc := elbv2.New(sess, awssdk.NewConfig().WithRegion(awsMetadata.Region))

	// Loop through target group pages and check for port and instanceID
	// TODO: This needs to handle lots of target groups efficiently
	params := &elbv2.DescribeTargetGroupsInput{
		PageSize: awssdk.Int64(400),
	}
	tgs, err := svc.DescribeTargetGroups(params)

	if err != nil {
		log.Printf("An error occurred using DescribeTargetGroups: %s \n", err.Error())
		return nil, err
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
			return nil, err
		}
	}

	params2 := &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: lb,
	}
	lbData, err := svc.DescribeLoadBalancers(params2)

	if err != nil {
		log.Printf("An error occurred using DescribeLoadBalancers: %s \n", err.Error())
		return nil, err
	}
	log.Printf("LB Endpoint is: %s:%s\n", *lbData.LoadBalancers[0].DNSName, strconv.FormatInt(*lbPort, 10))

	info.DNSName = *lbData.LoadBalancers[0].DNSName
	info.Port = *lbPort
	return info, err
}

// CheckELBFlags - Helper function to check if the correct config flags are set to use ELBs
func CheckELBFlags(service *bridge.Service) bool {
	if service.Attrs["eureka_use_elbv2_endpoint"] != "" && service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		v, err := strconv.ParseBool(service.Attrs["eureka_use_elbv2_endpoint"])
		if err != nil {
			log.Printf("eureka: eureka_use_elbv2_endpoint must be valid boolean, was %v : %s", v, err)
			return false
		}
		return true
	}
	return false
}

// Helper function to create a registration struct, and change container registration
func setRegInfo(service *bridge.Service, registration *eureka.Instance) *eureka.Instance {

	awsMetadata := GetMetadata()
	elbMetadata, err := getELBV2ForContainer(awsMetadata.InstanceID, int64(registration.Port))

	if err != nil {
		log.Printf("Unable to find associated ELBv2 for: %s, Error: %s\n", registration.HostName, err)
		return nil
	}

	elbStrPort := strconv.FormatInt(elbMetadata.Port, 10)
	elbEndpoint := elbMetadata.DNSName + "_" + elbStrPort

	registration.SetMetadataString("has-elbv2", "true")
	registration.SetMetadataString("elbv2-endpoint", elbEndpoint)

	elbReg := new(eureka.Instance)

	// Put a little metadata in here as required - setting InstanceID to the ELB endpoint prevents double registration
	elbReg.DataCenterInfo.Name = eureka.Amazon

	elbReg.DataCenterInfo.Metadata = eureka.AmazonMetadataType{
		PublicHostname: elbMetadata.DNSName,
		HostName:       elbMetadata.DNSName,
		InstanceID:     elbEndpoint,
	}

	elbReg.Port = int(elbMetadata.Port)
	elbReg.IPAddr = elbMetadata.DNSName
	elbReg.App = service.Name
	elbReg.VipAddress = elbReg.IPAddr
	elbReg.HostName = elbMetadata.DNSName
	elbReg.DataCenterInfo.Name = eureka.Amazon
	elbReg.SetMetadataString("is-elbv2", "true")
	return elbReg
}

// RegisterELBv2 - If specified, also register an ELBv2 (application load balancer, ALB) endpoint in eureka, and alter service name
// for container registrations.  This will mean traffic is directed to the ALB rather than directly to containers
// though they are still registered in eureka for information purposes
func RegisterELBv2(service *bridge.Service, registration *eureka.Instance, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		log.Printf("Found ELBv2 flags, will attempt to register LB for: %s\n", registration.HostName)
		elbReg := setRegInfo(service, registration)
		if elbReg != nil {
			client.RegisterInstance(elbReg)
		}
	}
}

// DeregisterELBv2 - If specified, and all containers are gone, also deregister the ELBv2 (application load balancer, ALB) endpoint in eureka.
//
func DeregisterELBv2(service *bridge.Service, regDNSName string, regPort int64, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		// Check if there are any containers around with this ALB still attached
		elbStrPort := strconv.FormatInt(regPort, 10)
		log.Printf("Found ELBv2 flags, will check if it needs to be deregistered too, for: %s:%v\n", regDNSName, elbStrPort)
		albName := regDNSName + "_" + elbStrPort
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
			log.Printf("Removing eureka entry for ELBv2: %s\n", albName)
			elbReg := new(eureka.Instance)
			elbReg.IPAddr = regDNSName
			elbReg.App = service.Name
			elbReg.HostName = albName // This uses the full endpoint identifier so eureka can find it to remove
			client.DeregisterInstance(elbReg)
		}
	}
}

// HeartbeatELBv2 - Send a heartbeat to eureka for this ELBv2 registration.  Every host running registrator will send heartbeats, meaning they will
// be received more frequently than the --ttl-refresh interval if there are multiple hosts running registrator.
//
func HeartbeatELBv2(service *bridge.Service, registration *eureka.Instance, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		log.Printf("Heartbeating ELBv2 for container: %s)\n", registration.HostName)

		elbReg := setRegInfo(service, registration)
		if elbReg != nil {
			client.HeartBeatInstance(elbReg)
		}
	}
}
