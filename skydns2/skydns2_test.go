package skydns2

import (
	"github.com/gliderlabs/registrator/bridge"
	"net/url"
	"testing"
)

func TestReverseDomainNameIPv4(t *testing.T) {
	const ExpectedDomainName = "1.0.0.127.in-addr.arpa."

	domainName, err := reverseDomainName("127.0.0.1")
	if err != nil {
		t.Error("Unexpected reverseDomainName call failure:", err)
	}
	if domainName != ExpectedDomainName {
		t.Error("Unexpected result:", domainName, "!=", ExpectedDomainName)
	}
}

func TestReverseDomainNameIPv6(t *testing.T) {
	const ExpectedDomainName = "b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa."

	domainName, err := reverseDomainName("2001:db8::567:89ab")
	if err != nil {
		t.Error("Unexpected reverseDomainName call failure", err)
	}
	if domainName != ExpectedDomainName {
		t.Error("Unexpected result:", domainName, "!=", ExpectedDomainName)
	}
}

func TestReverseDomainNameInvalid(t *testing.T) {
	domainName, err := reverseDomainName("#")
	if err == nil {
		t.Error(`Expected reverse domain name construction to fail for invalid IP "#"`)
	}
	if domainName != "" {
		t.Error("Expected empty domain name when construction failed but got", domainName)
	}
}

func TestServiceDomainName(t *testing.T) {
	const ExpectedDomainName = "1.test.skydns.local"

	service := bridge.Service{ID: "1", Name: "test"}
	u, _ := url.Parse("skydns2://127.0.0.1:4001/skydns.local")
	adapter := new(Factory).New(u).(*Skydns2Adapter)

	domainName := adapter.serviceDomainName(&service)

	if domainName != ExpectedDomainName {
		t.Error("Unexpected result:", domainName, "!=", ExpectedDomainName)
	}
}
