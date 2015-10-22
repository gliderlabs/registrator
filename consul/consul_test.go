package consul

import (
	"github.com/gliderlabs/registrator/bridge"
	"testing"
)

func TestHTTPCheckOnIPv4(t *testing.T) {
	service := &bridge.Service{}
	service.IP = "192.168.1.1"
	service.Port = 80
	service.Attrs = map[string]string{"check_http": "/foo"}
	adapter := ConsulAdapter{}
	check := adapter.buildCheck(service)
	if check.HTTP != "http://192.168.1.1:80/foo" {
		t.Error("Bad http url")
	}
}

func TestHTTPCheckOnIPv6(t *testing.T) {
	service := &bridge.Service{}
	service.IP = "2a00:1450:4013:c01::5e"
	service.Port = 80
	service.Attrs = map[string]string{"check_http": "/foo"}
	adapter := ConsulAdapter{}
	check := adapter.buildCheck(service)
	if check.HTTP != "http://[2a00:1450:4013:c01::5e]:80/foo" {
		t.Error("Bad http url")
	}
}
