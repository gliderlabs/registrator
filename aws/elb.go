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

// Helper function to retrieve all target groups
func getAllTargetGroups(svc *elbv2.ELBV2) ([]*elbv2.DescribeTargetGroupsOutput, error) {
	var tgs []*elbv2.DescribeTargetGroupsOutput
	var e error
	var mark *string

	// Get first page of groups
	tgs[0], e = getTargetGroupsPage(svc, mark)
	mark = tgs[0].NextMarker

	// Page through all remaining target groups generating a slice of DescribeTargetGroupOutputs
	i := 1
	for mark != nil {
		tgs[i], e = getTargetGroupsPage(svc, mark)
		mark = tgs[i].NextMarker
		if e != nil {
			return nil, e
		}
		i++
	}
	return tgs, e
}

// Helper function to get a page of target groups
func getTargetGroupsPage(svc *elbv2.ELBV2, marker *string) (*elbv2.DescribeTargetGroupsOutput, error) {
	params := &elbv2.DescribeTargetGroupsInput{
		PageSize: awssdk.Int64(400),
		Marker:   marker,
	}
	tg, e := svc.DescribeTargetGroups(params)

	if e != nil {
		log.Printf("An error occurred using DescribeTargetGroups: %s \n", e.Error())
		return nil, e
	}
	return tg, nil
}

// getELBV2ForContainer returns an LBInfo struct with the load balancer DNS name and listener port for a given instanceId and port
// if an error occurs, or the target is not found, an empty LBInfo is returned. Return the DNS:port pair as an identifier to put in the container's registration metadata
// Pass it the instanceID for the docker host, and the the host port to lookup the associated ELB.
func getELBV2ForContainer(instanceID string, port int64) (lbinfo *LBInfo, err error) {

	var lbArns []*string
	var lbPort *int64
	var tgArn *string
	info := &LBInfo{}

	sess, err := session.NewSession()
	if err != nil {
		message := fmt.Errorf("Failed to create session connecting to AWS: %s", err)
		return nil, message
	}

	// Need to set the region here - we'll get it from instance metadata
	awsMetadata := GetMetadata()
	svc := elbv2.New(sess, awssdk.NewConfig().WithRegion(awsMetadata.Region))

	// TODO Note: There could be thousands of these, and we need to check them all.  Seems to be no
	// other way to retrieve a TG via instance/port with current API
	tgslice, err := getAllTargetGroups(svc)
	if err != nil {
		message := fmt.Errorf("Failed to retrieve Target Groups: %s", err)
		return nil, message
	}

	// Check each target group's target list for a matching port and instanceID
	// Assumption: that that there is only one LB for the target group (though the data structure allows more)
	for _, tgs := range tgslice {
		for _, tg := range tgs.TargetGroups {
			td := []*elbv2.TargetDescription{{
				Id:   &instanceID,
				Port: &port,
			}}

			thParams := &elbv2.DescribeTargetHealthInput{
				TargetGroupArn: awssdk.String(*tg.TargetGroupArn),
				Targets:        td,
			}

			tarH, err := svc.DescribeTargetHealth(thParams)

			for _, thd := range tarH.TargetHealthDescriptions {
				if *thd.Target.Port == port && *thd.Target.Id == instanceID {
					lbArns = tg.LoadBalancerArns
					tgArn = tg.TargetGroupArn
				}
			}
			if err != nil {
				log.Printf("An error occurred using DescribeTargetHealth: %s \n", err.Error())
				return nil, err
			}
		}
	}

	// Loop through the load balancer listeners to get the listener port for the target group
	lsnrParams := &elbv2.DescribeListenersInput{
		LoadBalancerArn: lbArns[0],
	}
	lnrData, err := svc.DescribeListeners(lsnrParams)
	for _, listener := range lnrData.Listeners {
		for _, act := range listener.DefaultActions {
			if act.TargetGroupArn == tgArn {
				lbPort = listener.Port
			}
		}
	}

	// Get more information on the load balancer to retrieve the DNSName
	lbParams := &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: lbArns,
	}
	lbData, err := svc.DescribeLoadBalancers(lbParams)

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
