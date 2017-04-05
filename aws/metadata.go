package aws

import (
	"fmt"
	"log"
	"sync"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gliderlabs/registrator/interfaces"
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

var metadataCache *Metadata
var once sync.Once

// SetMetadata - Set the metadata and init - mainly for testing
func SetMetadata(md *Metadata) {
	metadataCache = md
	once.Do(func() { return })
}

// Test retrieval of metadata key and print an error if not, returning empty string
func getDataOrFail(svc interfaces.EC2MetadataGetter, key string) string {
	val, err := svc.GetMetadata(key)
	if err != nil {
		log.Printf("Unable to retrieve %s from the EC2 instance: %s\n", key, err)
		return ""
	}
	return val
}

// GetMetadata - retrieve metadata from AWS about the current host, using IAM role
func GetMetadata() *Metadata {

	// Initialize metadata exactly once for thread safety
	once.Do(func() {
		sess, err := session.NewSession()
		if err != nil {
			log.Printf("Unable to connect to the EC2 metadata service: %s\n", err)
		}
		svc := ec2metadata.New(sess)
		metadataCache = retrieveMetadata(svc)
	})
	return metadataCache
}

// RetrieveMetadata - retrieve metadata from AWS about the current host, using IAM role
func retrieveMetadata(svc interfaces.EC2MetadataGetter) *Metadata {
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
