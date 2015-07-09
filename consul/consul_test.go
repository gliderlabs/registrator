package consul

import (
	"testing"

	"github.com/gliderlabs/registrator/bridge"
)

func TestBuildChecksInOrder(t *testing.T) {
	service := service()
	service.Attrs["check_http"] = "/test"
	service.Attrs["check_cmd"] = "echo 1"
	service.Attrs["check_ttl"] = "30s"
	service.Attrs["check_script"] = "some script"

	checks := new(ConsulAdapter).buildChecks(service)
	if checks[0].Script != "check-cmd 123456789012 1 echo 1" {
		t.Error("Expected check-cmd but got", checks[0])
	} else if checks[1].Script != "check-http 123456789012 1 /test" {
		t.Error("Expected check-http but got", checks[1])
	} else if checks[2].Script != "some script" {
		t.Error("Expected check script but got", checks[2])
	} else if checks[3].TTL != "30s" {
		t.Error("Expected TTL but got", checks[3])
	}
}

func TestBuildChecksInterpolates(t *testing.T) {
	service := service()
	service.Attrs["check_script"] = "$SERVICE_IP:$SERVICE_PORT"

	script := new(ConsulAdapter).buildChecks(service)[0].Script
	if script != "127.0.0.1:10" {
		t.Error("Unexpected result,", script)
	}
}

func TestIntervalSpecification(t *testing.T) {
	service := service()
	service.Attrs["check_interval"] = "15s"
	service.Attrs["check_script"] = "something"

	interval := new(ConsulAdapter).buildChecks(service)[0].Interval
	if interval != "15s" {
		t.Error("Unexpected result,", interval)
	}
}

func TestRefreshOnlyTTL(t *testing.T) {
	service := service()
	service.Attrs["check_ttl"] = "30s"
	service.Attrs["check_interval"] = "25s"
	verifyTTLCall(t, service, "service:tests")
}

func TestRefreshMultipleChecks(t *testing.T) {
	service := service()
	service.Attrs["check_script"] = "something"
	service.Attrs["check_ttl"] = "30s"
	verifyTTLCall(t, service, "service:tests:2")
}

func verifyTTLCall(t *testing.T, service *bridge.Service, expected string) {
	callCheck := ""
	callNotes := ""
	adapter := ConsulAdapter{}
	adapter.refresh(service, func(check string, notes string) error {
		callCheck = check
		callNotes = notes
		return nil
	})
	if callCheck != expected {
		t.Error("Actual service check called:", callCheck)
	} else if callNotes == "" {
		t.Error("No notes passed.")
	}
}

func service() *bridge.Service {
	service := new(bridge.Service)
	service.ID = "tests"
	service.Origin.ExposedPort = "1"
	service.Origin.ContainerID = "1234567890123"
	service.Origin.HostIP = "127.0.0.1"
	service.Origin.HostPort = "10"
	service.Attrs = make(map[string]string)
	return service
}
