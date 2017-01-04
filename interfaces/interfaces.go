package interfaces

import (
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	fargo "github.com/hudl/fargo"
)

// InstanceData - Type to wrap fargo.Instance for testing
type InstanceData struct {
	fargo.Instance
	// SetMetadataString(string, string)
	// Id() string
}

// RegistrationData - Wrap registration instance to facilitate testing
type RegistrationData struct {
	Instance InstanceData
}

// EurekaConnector - Wraps fargo.EurekaConnection to facilitate testing
type EurekaConnector interface {
	HeartBeatInstance(*InstanceData)
	GetApp(string)
	DeregisterInstance(*InstanceData)
}

// EC2MetadataGetter Interface to help with test mocking
type EC2MetadataGetter interface {
	GetMetadata(string) (string, error)
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
}
