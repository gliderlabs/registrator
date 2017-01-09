package aws

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/gliderlabs/registrator/interfaces"
)

// Mock out a metadata interface and implement basic methods
type testMetadata struct {
	m       map[string]string
	isError error
	ec2Doc  ec2metadata.EC2InstanceIdentityDocument
}

var r *Metadata

func initMetadata() {

	md := &Metadata{
		InstanceID:       "init1",
		PrivateIP:        "i1.2.3.4",
		PublicIP:         "i4.5.6.7",
		PrivateHostname:  "ihost1",
		PublicHostname:   "ihost2",
		AvailabilityZone: "ius-east-1c",
		Region:           "ius-east-1",
	}

	metadataCache = md
	inited = true

}

var _ interfaces.EC2MetadataGetter = (*testMetadata)(nil)

func (t *testMetadata) GetMetadata(key string) (string, error) {
	if t.m[key] != "" {
		return t.m[key], nil
	}
	return "", t.isError
}
func (t *testMetadata) Available() bool {
	return true
}
func (t *testMetadata) GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error) {
	return t.ec2Doc, t.isError
}
func getIdentDoc() ec2metadata.EC2InstanceIdentityDocument {
	doc := ec2metadata.EC2InstanceIdentityDocument{
		Region:           "us-east-1",
		InstanceID:       "i-12341234",
		AvailabilityZone: "c",
	}
	return doc
}

// Test metadata is returned correctly
func TestGetDataOrFailSuccess(t *testing.T) {
	metadata := &testMetadata{}
	metadata.m = map[string]string{"test": "test1"}
	result := getDataOrFail(metadata, "test")
	if result != "test1" {
		t.Error("Metadata not retrieved correctly by GetDataOrFail")
	}
}

// Test metadata returns empty string if the key is missing
func TestGetDataOrFailFail(t *testing.T) {
	metadata := &testMetadata{}
	result := getDataOrFail(metadata, "test")
	if result != "" {
		t.Error("Incorrectly returned non-empty string")
	}
}

// Test for a failure to retrieve region
func TestRetrieveMetadataCatchesError(t *testing.T) {
	metadata := &testMetadata{}
	metadata.m = map[string]string{"test": "test1"}
	metadata.ec2Doc = getIdentDoc()
	metadata.isError = errors.New("This is supposed to fail")

	r := retrieveMetadata(metadata)
	if r.Region != "" {
		t.Error("Region is supposed to be empty.")
	}
}

// Test for a successful retrieval of all data
func TestRetrieveMetadata(t *testing.T) {
	metadata := &testMetadata{}
	metadata.m = map[string]string{
		"test":                        "test1",
		"instance-id":                 "i-12341234",
		"local-ipv4":                  "1.2.3.4",
		"public-ipv4":                 "4.5.6.7",
		"local-hostname":              "host1",
		"public-hostname":             "host2",
		"placement/availability-zone": "us-east-1c",
		"region":                      getIdentDoc().Region,
	}
	metadata.ec2Doc = getIdentDoc()
	metadata.isError = nil
	r = retrieveMetadata(metadata)

	checkVS(t, metadata.m, "region", r.Region)
	checkVS(t, metadata.m, "placement/availability-zone", r.AvailabilityZone)
	checkVS(t, metadata.m, "public-ipv4", r.PublicIP)
	checkVS(t, metadata.m, "local-ipv4", r.PrivateIP)
	checkVS(t, metadata.m, "local-hostname", r.PrivateHostname)
	checkVS(t, metadata.m, "public-hostname", r.PublicHostname)
	checkVS(t, metadata.m, "instance-id", r.InstanceID)

}

// Test metadata is now cached after first retrieval
func TestCacheMetadata(t *testing.T) {
	initMetadata()
	md := GetMetadata()

	if md.InstanceID != "init1" {
		t.Errorf("Metadata %v InstanceName: does not match expected init1.\n", md)
	}

}

// Check one metadata value vs result and print errors if failed
func checkVS(t *testing.T, m map[string]string, k string, val string) {
	if val != m[k] {
		t.Errorf("Metadata %s: expected %v, actual: %s ", k, m[k], val)
	}
}
