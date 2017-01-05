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
	Region           string
}

// IEC2Metadata Interface to help with test mocking
type IEC2Metadata interface {
	GetMetadata(string) (string, error)
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
}

// Test retrieval of metadata key and print an error if not, returning empty string
func getDataOrFail(svc IEC2Metadata, key string) string {
	val, err := svc.GetMetadata(key)
	if err != nil {
		log.Printf("Unable to retrieve %s from the EC2 instance: %s\n", key, err)
		return ""
	}
	return val
}

// GetMetadata - retrieve metadata from AWS about the current host, using IAM role
func GetMetadata() *Metadata {
	sess, err := session.NewSession()
	if err != nil {
		fmt.Printf("Unable to connect to the EC2 metadata service: %s\n", err)
	}
	svc := ec2metadata.New(sess)
	return retrieveMetadata(svc)
}

// RetrieveMetadata - retrieve metadata from AWS about the current host, using IAM role
func retrieveMetadata(svc IEC2Metadata) *Metadata {
	log.Println("Attempting to retrieve AWS metadata.")

	m := new(Metadata)

	if svc.Available() {
		ident, err := svc.GetInstanceIdentityDocument()
		if err != nil {
			m.Region = ""
		} else {
			m.Region = ident.Region
		}
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
