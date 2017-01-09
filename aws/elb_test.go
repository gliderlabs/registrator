package aws

import (
	"reflect"
	"testing"

	"github.com/gliderlabs/registrator/bridge"
	eureka "github.com/hudl/fargo"
)

// Test_getELBV2ForContainer - Test expected values are returned
func Test_getELBV2ForContainer(t *testing.T) {

	// Setup cache
	lbWant := LBInfo{
		DNSName: "",
		Port:    int64(1234),
	}
	lbCache["instance-123_1234"] = &lbWant

	type args struct {
		instanceID string
		port       int64
	}
	tests := []struct {
		name       string
		args       args
		wantLbinfo *LBInfo
		wantErr    bool
	}{
		{
			name:       "should match",
			args:       args{instanceID: "instance-123", port: int64(1234)},
			wantErr:    false,
			wantLbinfo: &lbWant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLbinfo, err := getELBV2ForContainer(tt.args.instanceID, tt.args.port, true)
			if (err != nil) != tt.wantErr {
				t.Errorf("getELBV2ForContainer() error = %+v, wantErr %+v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotLbinfo, tt.wantLbinfo) {
				t.Errorf("getELBV2ForContainer() = %+v, want %+v", gotLbinfo, tt.wantLbinfo)
			}
		})
	}
}

// TestCheckELBFlags - Test that ELBv2 flags are evaulated correctly
func TestCheckELBFlags(t *testing.T) {

	svcFalse := bridge.Service{
		Attrs: map[string]string{
			"eureka_use_elbv2_endpoint":  "false",
			"eureka_datacenterinfo_name": "AMAZON",
		},
	}

	svcFalse2 := bridge.Service{
		Attrs: map[string]string{
			"eureka_use_elbv2_endpoint":  "true",
			"eureka_datacenterinfo_name": "MyOwn",
		},
	}

	svcTrue := bridge.Service{
		Attrs: map[string]string{
			"eureka_use_elbv2_endpoint":  "true",
			"eureka_datacenterinfo_name": "AMAZON",
		},
	}

	type args struct {
		service *bridge.Service
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should be false",
			args: args{service: &svcFalse},
			want: false,
		},
		{
			name: "should be false again",
			args: args{service: &svcFalse2},
			want: false,
		},
		{
			name: "should be true",
			args: args{service: &svcTrue},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckELBFlags(tt.args.service); got != tt.want {
				t.Errorf("CheckELBFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test_setRegInfo - Test that registration struct is returned as expected
func Test_setRegInfo(t *testing.T) {
	initMetadata() // Used from metadata_test.go

	svc := bridge.Service{
		Attrs: map[string]string{
			"eureka_use_elbv2_endpoint":  "false",
			"eureka_datacenterinfo_name": "AMAZON",
		},
		Name: "app",
	}

	awsInfo := eureka.AmazonMetadataType{
		PublicHostname: "dns-name",
		HostName:       "dns-name",
		InstanceID:     "endpoint",
	}

	dcInfo := eureka.DataCenterInfo{
		Name:     eureka.Amazon,
		Metadata: awsInfo,
	}

	reg := eureka.Instance{
		DataCenterInfo: dcInfo,
		Port:           5001,
		IPAddr:         "4.3.2.1",
		App:            "app",
		VipAddress:     "4.3.2.1",
		HostName:       "hostname",
	}

	// Init LB info cache
	lbCache["init1_5001"] = &LBInfo{
		DNSName: "lb-dnsname",
		Port:    9001,
	}

	wantedAwsInfo := eureka.AmazonMetadataType{
		PublicHostname: "lb-dnsname",
		HostName:       "lb-dnsname",
		InstanceID:     "lb-dnsname_9001",
	}
	wantedDCInfo := eureka.DataCenterInfo{
		Name:     eureka.Amazon,
		Metadata: wantedAwsInfo,
	}

	wanted := eureka.Instance{
		DataCenterInfo: wantedDCInfo,
		Port:           9001,
		App:            svc.Name,
		IPAddr:         "lb-dnsname",
		VipAddress:     "lb-dnsname",
		HostName:       "lb-dnsname",
	}

	type args struct {
		service      *bridge.Service
		registration *eureka.Instance
	}
	tests := []struct {
		name string
		args args
		want *eureka.Instance
	}{
		{
			name: "Should match data",
			args: args{service: &svc, registration: &reg},
			want: &wanted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setRegInfo(tt.args.service, tt.args.registration, true)
			val := got.Metadata.GetMap()["is-elbv2"]
			if val != "true" {
				t.Errorf("setRegInfo() = %+v, \n Wanted is-elbv2=true in metadata, was %+v", got, val)
			}
			//Overwrite metadata before comparing data structure - we've directly checked the flag we are looking for
			got.Metadata = eureka.InstanceMetadata{}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setRegInfo() = %+v, \nwant %+v\n", got, tt.want)
			}
		})
	}
}
