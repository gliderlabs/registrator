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

var lbCache = make(map[string]*LBInfo)
var registrations = make(map[string]bool)

// Helper function to retrieve all target groups
func getAllTargetGroups(svc *elbv2.ELBV2) ([]*elbv2.DescribeTargetGroupsOutput, error) {
	var tgs []*elbv2.DescribeTargetGroupsOutput
	var e error
	var mark *string

	// Get first page of groups
	tg, e := getTargetGroupsPage(svc, mark)

	if e != nil {
		return nil, e
	}
	tgs = append(tgs, tg)
	mark = tg.NextMarker

	// Page through all remaining target groups generating a slice of DescribeTargetGroupOutputs
	for mark != nil {
		tg, e = getTargetGroupsPage(svc, mark)
		tgs = append(tgs, tg)
		mark = tg.NextMarker
		if e != nil {
			return nil, e
		}
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
// if an error occurs, or the target is not found, an empty LBInfo is returned.
// Return the DNS:port pair as an identifier to put in the container's registration metadata
// Pass it the instanceID for the docker host, and the the host port to lookup the associated ELB.
// useCache parameter, if true, will retrieve ELBv2 details from memory, rather than calling AWS.
// this is only really safe to use for heartbeat calls, as details can change dynamically
func getELBV2ForContainer(containerID string, instanceID string, port int64, useCache bool) (lbinfo *LBInfo, err error) {

	// Retrieve from basic cache (for heartbeats)
	cacheKey := containerID
	if val, ok := lbCache[cacheKey]; ok && useCache {
		log.Println("Retrieving value from cache.")
		return val, nil
	}

	var lbArns []*string
	var lbPort *int64
	var tgArn string
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
	if err != nil || tgslice == nil {
		message := fmt.Errorf("Failed to retrieve Target Groups: %s", err)
		return nil, message
	}

	// Check each target group's target list for a matching port and instanceID
	// TODO Assumption: that that there is only one LB for the target group (though the data structure allows more)
	for _, tgs := range tgslice {
		for _, tg := range tgs.TargetGroups {

			thParams := &elbv2.DescribeTargetHealthInput{
				TargetGroupArn: awssdk.String(*tg.TargetGroupArn),
			}

			tarH, err := svc.DescribeTargetHealth(thParams)
			if err != nil {
				log.Printf("An error occurred using DescribeTargetHealth: %s \n", err.Error())
				return nil, err
			}

			for _, thd := range tarH.TargetHealthDescriptions {
				if *thd.Target.Port == port && *thd.Target.Id == instanceID {
					lbArns = tg.LoadBalancerArns
					tgArn = *tg.TargetGroupArn
					break
				}
			}
		}
		if lbArns != nil && tgArn != "" {
			break
		}
	}

	if err != nil || lbArns == nil {
		message := fmt.Errorf("failed to retrieve load balancer ARN")
		return nil, message
	}

	// Loop through the load balancer listeners to get the listener port for the target group
	lsnrParams := &elbv2.DescribeListenersInput{
		LoadBalancerArn: lbArns[0],
	}
	lnrData, err := svc.DescribeListeners(lsnrParams)
	if err != nil {
		log.Printf("An error occurred using DescribeListeners: %s \n", err.Error())
		return nil, err
	}
	for _, listener := range lnrData.Listeners {
		for _, act := range listener.DefaultActions {
			if *act.TargetGroupArn == tgArn {
				log.Printf("Found matching listener: %v", *listener.ListenerArn)
				lbPort = listener.Port
				break
			}
		}
	}
	if lbPort == nil {
		message := fmt.Errorf("error: Unable to identify listener port for ELBv2")
		return nil, message
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
	log.Printf("LB Endpoint for Instance:%v Port:%v, Target Group:%v, is: %s:%s\n", instanceID, port, tgArn, *lbData.LoadBalancers[0].DNSName, strconv.FormatInt(*lbPort, 10))

	info.DNSName = *lbData.LoadBalancers[0].DNSName
	info.Port = *lbPort

	// Add to a basic cache for heartbeats
	lbCache[cacheKey] = info

	return info, nil
}

// CheckELBFlags - Helper function to check if the correct config flags are set to use ELBs
func CheckELBFlags(service *bridge.Service) bool {
	if service.Attrs["eureka_use_elbv2_endpoint"] != "" && service.Attrs["eureka_datacenterinfo_name"] != eureka.MyOwn {
		v, err := strconv.ParseBool(service.Attrs["eureka_use_elbv2_endpoint"])
		if err != nil {
			log.Printf("eureka: eureka_use_elbv2_endpoint must be valid boolean, was %v : %s", v, err)
			return false
		}
		return v
	}
	return false
}

// Helper function to create a registration struct, and change container registration
// useCache parameter is passed to getELBV2ForContainer
func setRegInfo(service *bridge.Service, registration *eureka.Instance, useCache bool) *eureka.Instance {

	awsMetadata := GetMetadata()

	elbMetadata, err := getELBV2ForContainer(service.Origin.ContainerID, awsMetadata.InstanceID, int64(registration.Port), useCache)

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
	elbReg.Status = eureka.UP
	return elbReg
}

// RegisterELBv2 - If specified, also register an ELBv2 (application load balancer, ALB) endpoint in eureka, and alter service name
// for container registrations.  This will mean traffic is directed to the ALB rather than directly to containers
// though they are still registered in eureka for information purposes
func RegisterELBv2(service *bridge.Service, registration *eureka.Instance, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		log.Printf("Found ELBv2 flags, will attempt to register LB for: %s\n", registration.HostName)
		elbReg := setRegInfo(service, registration, false)
		if elbReg != nil {
			err := client.RegisterInstance(elbReg)
			if err != nil {
				registrations[service.Origin.ContainerID] = true
			}
		}
	}
}

// DeregisterELBv2 - If specified, and all containers are gone, also deregister the ELBv2 (application load balancer, ALB) endpoint in eureka.
//
func DeregisterELBv2(service *bridge.Service, albEndpoint string, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {

		if albEndpoint == "" {
			log.Printf("No endpoint available when attempting deregister.  ELBv2 will remain registered. Container: %v, IP: %v Port: %v", service.Origin.ContainerID, service.IP, service.Port)
			return
		}

		// Check if there are any containers around with this ALB still attached
		log.Printf("Found ELBv2 flags, will check if it needs to be deregistered too, for: %v\n", albEndpoint)
		appName := "CONTAINER_" + service.Name

		app, err := client.GetApp(appName)
		if err != nil {
			log.Printf("Unable to retrieve app metadata for %s: %s\n", appName, err)
			delete(registrations, service.Origin.ContainerID)
			delete(lbCache, service.Origin.ContainerID)
			return
		}

		if app != nil {
			for _, instance := range app.Instances {
				val, err := instance.Metadata.GetString("elbv2_endpoint")
				if err == nil && val == albEndpoint {
					log.Printf("Eureka entry still present for one or more ALB linked containers: %s\n", val)
					delete(registrations, service.Origin.ContainerID)
					delete(lbCache, service.Origin.ContainerID)
					return
				}
			}
		}

		if app == nil {
			log.Printf("Removing eureka entry for ELBv2: %s\n", albEndpoint)
			elbReg := new(eureka.Instance)
			elbReg.App = service.Name
			elbReg.HostName = albEndpoint // This uses the full endpoint identifier so eureka can find it to remove
			client.DeregisterInstance(elbReg)
			delete(registrations, service.Origin.ContainerID)
			delete(lbCache, service.Origin.ContainerID)
		}
	}
}

// HeartbeatELBv2 - Send a heartbeat to eureka for this ELBv2 registration.  Every host running registrator will send heartbeats, meaning they will
// be received more frequently than the --ttl-refresh interval if there are multiple hosts running registrator.
//
func HeartbeatELBv2(service *bridge.Service, registration *eureka.Instance, client eureka.EurekaConnection) {
	if CheckELBFlags(service) {
		log.Printf("Heartbeating ELBv2 for container: %s)\n", registration.HostName)

		if reg, ok := registrations[service.Origin.ContainerID]; !ok || !reg {
			log.Printf("ELBv2 failed previous registration.  Attempting register now.")
			RegisterELBv2(service, registration, client)
		}

		elbReg := setRegInfo(service, registration, true) // Can safely use cache when heartbeating
		if elbReg != nil {
			client.HeartBeatInstance(elbReg)
		}
	}
}
