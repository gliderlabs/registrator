package aws

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/gliderlabs/registrator/bridge"
	eureka "github.com/hudl/fargo"
)

// Test_GetELBV2ForContainer - Test expected values are returned
func Test_GetELBV2ForContainer(t *testing.T) {

	// Setup cache
	lbWant := LBInfo{
		DNSName: "",
		Port:    int64(1234),
	}
	lbCache["123123412"] = &lbWant

	type args struct {
		containerID string
		instanceID  string
		port        int64
	}
	tests := []struct {
		name       string
		args       args
		wantLbinfo *LBInfo
		wantErr    bool
	}{
		{
			name:       "should match",
			args:       args{containerID: "123123412", instanceID: "instance-123", port: int64(1234)},
			wantErr:    false,
			wantLbinfo: &lbWant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLbinfo, err := GetELBV2ForContainer(tt.args.containerID, tt.args.instanceID, tt.args.port, true)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetELBV2ForContainer() error = %+v, wantErr %+v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotLbinfo, tt.wantLbinfo) {
				t.Errorf("GetELBV2ForContainer() = %+v, want %+v", gotLbinfo, tt.wantLbinfo)
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

	svcFalse3 := bridge.Service{
		Attrs: map[string]string{
			"eureka_elbv2_hostname":      "my-name",
			"eureka_datacenterinfo_name": "AMAZON",
		},
	}

	svcFalse4 := bridge.Service{
		Attrs: map[string]string{
			"eureka_elbv2_hostname":      "my-name",
			"eureka_use_elbv2_endpoint":  "false",
			"eureka_elbv2_port":          "1234",
			"eureka_datacenterinfo_name": "AMAZON",
		},
	}

	svcTrue := bridge.Service{
		Attrs: map[string]string{
			"eureka_use_elbv2_endpoint":  "true",
			"eureka_datacenterinfo_name": "AMAZON",
		},
	}

	svcTrue2 := bridge.Service{
		Attrs: map[string]string{
			"eureka_elbv2_hostname":      "my-name",
			"eureka_elbv2_port":          "1234",
			"eureka_datacenterinfo_name": "AMAZON",
		},
	}

	svcTrue3 := bridge.Service{
		Attrs: map[string]string{
			"eureka_elbv2_hostname":      "my-name",
			"eureka_use_elbv2_endpoint":  "true",
			"eureka_elbv2_port":          "1234",
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
			name: "use endpoint set to false",
			args: args{service: &svcFalse},
			want: false,
		},
		{
			name: "datacentre is set to MyOwn",
			args: args{service: &svcFalse2},
			want: false,
		},
		{
			name: "elb hostname, but not port is set",
			args: args{service: &svcFalse3},
			want: false,
		},
		{
			name: "elb hostname, and port are set, but endpoint is false",
			args: args{service: &svcFalse4},
			want: false,
		},
		{
			name: "elb endpoint set to true",
			args: args{service: &svcTrue},
			want: true,
		},
		{
			name: "elb hostname and port are set",
			args: args{service: &svcTrue2},
			want: true,
		},
		{
			name: "elb hostname and port are set, as is use endpoint",
			args: args{service: &svcTrue3},
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
		Origin: bridge.ServicePort{
			ContainerID: "123123412",
		},
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
		HostName:       "hostname_identifier",
		Status:         eureka.UP,
	}

	// Init LB info cache
	lbCache["123123412"] = &LBInfo{
		DNSName: "lb-dnsname",
		Port:    9001,
	}

	wantedAwsInfo := eureka.AmazonMetadataType{
		PublicHostname: "dns-name",
		HostName:       "dns-name",
		InstanceID:     "endpoint",
	}
	wantedDCInfo := eureka.DataCenterInfo{
		Name:     eureka.Amazon,
		Metadata: wantedAwsInfo,
	}

	wanted := eureka.Instance{
		DataCenterInfo: wantedDCInfo,
		Port:           int(lbCache["123123412"].Port),
		App:            svc.Name,
		IPAddr:         lbCache["123123412"].DNSName,
		VipAddress:     lbCache["123123412"].DNSName,
		HostName:       reg.HostName,
		Status:         eureka.UP,
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
			val := got.Metadata.GetMap()["has-elbv2"]
			if val != "true" {
				t.Errorf("setRegInfo() = %+v, \n Wanted has-elbv2=true in metadata, was %+v", got, val)
			}
			val2 := got.Metadata.GetMap()["elbv2-endpoint"]
			wantVal := lbCache["123123412"].DNSName + "_" + strconv.Itoa(int(lbCache["123123412"].Port))
			if val2 != wantVal {
				t.Errorf("setRegInfo() = %+v, \n Wanted elbv2-endpoint=%v in metadata, was %+v", got, wantVal, val)
			}
			//Overwrite metadata before comparing data structure - we've directly checked the flag we are looking for
			got.Metadata = eureka.InstanceMetadata{}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setRegInfo() = %+v, \nwant %+v\n", got, tt.want)
			}
		})
	}
}

// Test_setRegInfoExplicitEndpoint - Test that registration struct is returned as expected when you set the host and port
// and that they are used rather than the load balancer lookup
func Test_setRegInfoExplicitEndpoint(t *testing.T) {
	initMetadata() // Used from metadata_test.go

	svc := bridge.Service{
		Attrs: map[string]string{
			"eureka_use_elbv2_endpoint":  "false",
			"eureka_elbv2_hostname":      "hostname-i-set",
			"eureka_elbv2_port":          "65535",
			"eureka_datacenterinfo_name": "AMAZON",
		},
		Name: "app",
		Origin: bridge.ServicePort{
			ContainerID: "123123412",
		},
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
		HostName:       "hostname_identifier",
		Status:         eureka.UP,
	}

	// Init LB info cache
	// if things are working correctly, this won't be used for this test
	lbCache["123123412"] = &LBInfo{
		DNSName: "lb-dnsname",
		Port:    9001,
	}

	wantedAwsInfo := eureka.AmazonMetadataType{
		PublicHostname: "dns-name",
		HostName:       "dns-name",
		InstanceID:     "endpoint",
	}
	wantedDCInfo := eureka.DataCenterInfo{
		Name:     eureka.Amazon,
		Metadata: wantedAwsInfo,
	}

	expectedPort, _ := strconv.Atoi(svc.Attrs["eureka_elbv2_port"])
	wanted := eureka.Instance{
		DataCenterInfo: wantedDCInfo,
		Port:           expectedPort,
		App:            svc.Name,
		IPAddr:         svc.Attrs["eureka_elbv2_hostname"],
		VipAddress:     svc.Attrs["eureka_elbv2_hostname"],
		HostName:       reg.HostName,
		Status:         eureka.UP,
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
			val := got.Metadata.GetMap()["has-elbv2"]
			if val != "true" {
				t.Errorf("setRegInfo() = %+v, \n Wanted has-elbv2=true in metadata, was %+v", got, val)
			}
			val2 := got.Metadata.GetMap()["elbv2-endpoint"]
			wantVal := svc.Attrs["eureka_elbv2_hostname"] + "_" + svc.Attrs["eureka_elbv2_port"]
			if val2 != wantVal {
				t.Errorf("setRegInfo() = %+v, \n Wanted elbv2-endpoint=%v in metadata, was %+v", got, wantVal, val2)
			}
			//Overwrite metadata before comparing data structure - we've directly checked the flag we are looking for
			got.Metadata = eureka.InstanceMetadata{}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setRegInfo() = %+v, \nwant %+v\n", got, tt.want)
			}
		})
	}

}
