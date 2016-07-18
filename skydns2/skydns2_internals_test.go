package skydns2

import (
	"testing"

	"github.com/gliderlabs/registrator/bridge"
)

func TestServiceRecord(t *testing.T) {
	var tests = []struct {
		srv      bridge.Service
		expected string
	}{
		{
			bridge.Service{IP: "127.0.0.1", Port: 80},
			`{"host":"127.0.0.1","port":80}`,
		},
		{
			bridge.Service{IP: "127.0.0.1", Port: 80, Attrs: map[string]string{"priority": "20"}},
			`{"host":"127.0.0.1","port":80,"priority":20}`,
		},
		{
			bridge.Service{IP: "127.0.0.1", Port: 80, Attrs: map[string]string{"weight": "100"}},
			`{"host":"127.0.0.1","port":80,"weight":100}`,
		},
		{
			bridge.Service{IP: "127.0.0.1", Port: 80, Attrs: map[string]string{"priority": "20", "weight": "100"}},
			`{"host":"127.0.0.1","port":80,"priority":20,"weight":100}`,
		},
	}

	for _, test := range tests {
		record, err := serviceRecord(&test.srv)

		if err != nil {
			t.Fatal("Failed to create service record, error:", err)
		}

		if test.expected != record {
			t.Fatal("Actual result != expected result")
		}
	}
}
