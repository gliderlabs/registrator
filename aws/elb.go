package aws

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/gliderlabs/registrator/bridge"
	fargo "github.com/hudl/fargo"
)

// LBInfo represents a ELBv2 endpoint
type LBInfo struct {
	DNSName string
	Port    int64
}

type lookupValues struct {
	InstanceID string
	Port       int64
}

type cacheEntry struct {
	lb *LBInfo
	sync.Mutex
}

type lbCache struct {
	m map[string]cacheEntry
	sync.Mutex
}

var cache lbCache

type fn func(lookupValues) (*LBInfo, error)

//
// Return a *LBInfo cache entry if it exists, or run the provided function to return data to add to cache
// The complexity is purely to make the cache thread safe.
//
func getOrAddCacheEntry(key string, f fn, i lookupValues) (*LBInfo, error) {
	cache.Lock()
	if cache.m == nil {
		cache.m = make(map[string]cacheEntry)
	}
	var populated = true
	if _, ok := cache.m[key]; !ok {
		cache.m[key] = cacheEntry{lb: &LBInfo{}}
		populated = false
	}
	entry := cache.m[key]
	entry.Lock()
	defer entry.Unlock()
	cache.Unlock()
	var err error
	if !populated {
		var o *LBInfo
		o, err = f(i)
		if err != nil {
			log.Print("An error occurred trying to add data to the cache:", err)
		}
		entry.lb = o
	}
	value := entry.lb
	return value, err
}

// RemoveLBCache : Delete any cache of load balancer for this containerID
func RemoveLBCache(key string) {
	cache.Lock()
	delete(cache.m, key)
	cache.Unlock()
}

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

// GetELBV2ForContainer returns an LBInfo struct with the load balancer DNS name and listener port for a given instanceId and port
// if an error occurs, or the target is not found, an empty LBInfo is returned.
// Pass it the instanceID for the docker host, and the the host port to lookup the associated ELB.
//
func GetELBV2ForContainer(containerID string, instanceID string, port int64) (lbinfo *LBInfo, err error) {
	i := lookupValues{InstanceID: instanceID, Port: port}
	return getOrAddCacheEntry(containerID, getLB, i)
}

//
// Does the real work of retrieving the load balancer details, given a lookupValues struct.
//
func getLB(l lookupValues) (lbinfo *LBInfo, err error) {
	instanceID := l.InstanceID
	port := l.Port

	// We need to have small random wait here, because it takes a little while for new containers to appear in target groups
	// to avoid any wait, the endpoints can be specified manually as eureka_elbv2_hostname and eureka_elbv2_port vars
	rand.NewSource(time.Now().UnixNano())
	period := time.Second * time.Duration(rand.Intn(10)+20)
	time.Sleep(period)

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
	return info, nil
}

// CheckELBFlags - Helper function to check if the correct config flags are set to use ELBs
// We accept two possible configurations here - either eureka_lookup_elbv2_endpoint can be set,
// for automatic lookup, or eureka_elbv2_hostname and eureka_elbv2_port can be set manually
// to avoid the 10-20s wait for lookups
func CheckELBFlags(service *bridge.Service) bool {

	isAws := service.Attrs["eureka_datacenterinfo_name"] != fargo.MyOwn
	var hasExplicit bool
	var useLookup bool

	if service.Attrs["eureka_elbv2_hostname"] != "" && service.Attrs["eureka_elbv2_port"] != "" {
		v, err := strconv.ParseUint(service.Attrs["eureka_elbv2_port"], 10, 16)
		if err != nil {
			log.Printf("eureka: eureka_elbv2_port must be valid 16-bit unsigned int, was %v : %s", v, err)
			hasExplicit = false
		}
		hasExplicit = true
		useLookup = true
	}

	if service.Attrs["eureka_lookup_elbv2_endpoint"] != "" {
		v, err := strconv.ParseBool(service.Attrs["eureka_lookup_elbv2_endpoint"])
		if err != nil {
			log.Printf("eureka: eureka_lookup_elbv2_endpoint must be valid boolean, was %v : %s", v, err)
			useLookup = false
		}
		useLookup = v
	}

	if (hasExplicit || useLookup) && isAws {
		return true
	}
	return false
}

// CheckELBOnlyReg - Helper function to check if only the ELB should be registered (no containers)
func CheckELBOnlyReg(service *bridge.Service) bool {

	if service.Attrs["eureka_elbv2_only_registration"] != "" {
		v, err := strconv.ParseBool(service.Attrs["eureka_elbv2_only_registration"])
		if err != nil {
			log.Printf("eureka: eureka_elbv2_only_registration must be valid boolean, was %v : %s", v, err)
			return true
		}
		return v
	}
	return true
}

// GetUniqueID Note: Helper function reimplemented here to avoid segfault calling it on fargo.Instance struct
func GetUniqueID(instance fargo.Instance) string {
	return instance.HostName + "_" + strconv.Itoa(instance.Port)
}

// Helper function to alter registration info and add the ELBv2 endpoint
// useCache parameter is passed to getELBV2ForContainer
func setRegInfo(service *bridge.Service, registration *fargo.Instance) *fargo.Instance {

	awsMetadata := GetMetadata()
	var elbEndpoint string

	// We've been given the ELB endpoint, so use this
	if service.Attrs["eureka_elbv2_hostname"] != "" && service.Attrs["eureka_elbv2_port"] != "" {
		log.Printf("Found ELBv2 hostname=%v and port=%v options, using these.", service.Attrs["eureka_elbv2_hostname"], service.Attrs["eureka_elbv2_port"])
		registration.Port, _ = strconv.Atoi(service.Attrs["eureka_elbv2_port"])
		registration.HostName = service.Attrs["eureka_elbv2_hostname"]
		registration.IPAddr = ""
		registration.VipAddress = ""
		elbEndpoint = service.Attrs["eureka_elbv2_hostname"] + "_" + service.Attrs["eureka_elbv2_port"]

	} else {
		// We don't have the ELB endpoint, so look it up.
		elbMetadata, err := GetELBV2ForContainer(service.Origin.ContainerID, awsMetadata.InstanceID, int64(registration.Port))

		if err != nil {
			log.Printf("Unable to find associated ELBv2 for: %s, Error: %s\n", registration.HostName, err)
			return nil
		}

		elbStrPort := strconv.FormatInt(elbMetadata.Port, 10)
		elbEndpoint = elbMetadata.DNSName + "_" + elbStrPort
		registration.Port = int(elbMetadata.Port)
		registration.IPAddr = ""
		registration.HostName = elbMetadata.DNSName
	}

	if CheckELBOnlyReg(service) {
		// Remove irrelevant metadata from an ELB only registration
		registration.DataCenterInfo.Metadata = fargo.AmazonMetadataType{
			InstanceID:     GetUniqueID(*registration), // This is deliberate - due to limitations in uniqueIDs
			PublicHostname: registration.HostName,
			HostName:       registration.HostName,
		}
		registration.SetMetadataString("container-id", "")
		registration.SetMetadataString("container-name", "")
		registration.SetMetadataString("aws-instance-id", "")
	}

	registration.SetMetadataString("has-elbv2", "true")
	registration.SetMetadataString("elbv2-endpoint", elbEndpoint)
	registration.VipAddress = registration.IPAddr
	return registration
}

// RegisterWithELBv2 - If called, and flags are active, register an ELBv2 endpoint instead of the container directly
// This will mean traffic is directed to the ALB rather than directly to containers
func RegisterWithELBv2(service *bridge.Service, registration *fargo.Instance, client fargo.EurekaConnection) error {
	if CheckELBFlags(service) {
		log.Printf("Found ELBv2 flags, will attempt to register LB for: %s\n", GetUniqueID(*registration))
		elbReg := setRegInfo(service, registration)
		if elbReg != nil {
			err := client.ReregisterInstance(elbReg)
			if err == nil {
				registrations[service.Origin.ContainerID] = true
			}
			return nil
		}
	}
	return fmt.Errorf("unable to register ELBv2 - flags are not set")
}

// HeartbeatELBv2 - Heartbeat an ELB registration
func HeartbeatELBv2(service *bridge.Service, registration *fargo.Instance, client fargo.EurekaConnection) error {
	if CheckELBFlags(service) {
		log.Printf("Heartbeating ELBv2: %s\n", GetUniqueID(*registration))
		elbReg := setRegInfo(service, registration)
		if elbReg != nil {
			err := client.HeartBeatInstance(elbReg)
			return err
		}
	}
	return fmt.Errorf("unable to heartbeat ELBv2 - flags are not set")
}
