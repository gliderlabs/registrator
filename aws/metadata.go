package aws

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

type Metadata struct {
	InstanceID       string
	PrivateIP        string
	PublicIP         string
	PrivateHostname  string
	PublicHostname   string
	AvailabilityZone string
}

func getDataOrFail(svc *ec2metadata.EC2Metadata, key string) string {
	val, err := svc.GetMetadata(key)
	if err != nil {
		fmt.Printf("Unable to retrieve %s from the EC2 instance: %s\n", key, err)
		return ""
	}
	return val
}

func GetMetadata() *Metadata {
	log.Println("Attempting to retrieve AWS metadata.")
	sess, err := session.NewSession()
	if err != nil {
		fmt.Printf("Unable to connect to the EC2 metadata service: %s\n", err)
	}
	svc := ec2metadata.New(sess)
	m := new(Metadata)
	if svc.Available() {
		m.InstanceID = getDataOrFail(svc, "instance-id")
		m.PrivateIP = getDataOrFail(svc, "local-ipv4")
		m.PublicIP = getDataOrFail(svc, "public-ipv4")
		m.PrivateHostname = getDataOrFail(svc, "local-hostname")
		m.PublicHostname = getDataOrFail(svc, "public-hostname")
		m.AvailabilityZone = getDataOrFail(svc, "placement/availability-zone")
	} else {
		fmt.Println("AWS metadata not available :(")
	}
	return m
}
