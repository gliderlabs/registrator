package interfaces

import "github.com/aws/aws-sdk-go/aws/ec2metadata"

// EC2MetadataGetter Interface to help with test mocking
type EC2MetadataGetter interface {
	GetMetadata(string) (string, error)
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
}
