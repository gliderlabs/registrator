package bridge

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestMatchPorts(t *testing.T) {
	var filters Filters

	filters.Clear()
	if err := filters.Append("192.168.1.1:80"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ := filters.Match("192.168.1.1", 80, false)
	assert.True(t, result, "test single ip:port match")
	result, _, _ = filters.Match("192.168.1.1", 81, false)
	assert.False(t, result, "test single ip:port notmatch")
	result, _, _ = filters.Match("192.168.1.2", 80, false)
	assert.False(t, result, "test single ip:port notmatch")

	filters.Clear()
	if err := filters.Append("192.168.1.1:*"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 80, false)
	assert.True(t, result, "test single ip with wildcard port")
	result, _, _ = filters.Match("192.168.1.1", 81, false)
	assert.True(t, result, "test single ip with wildcard port")
	result, _, _ = filters.Match("192.168.1.2", 80, false)
	assert.False(t, result, "test single ip with wildcard port")

	filters.Clear()
	if err := filters.Append("192.168.1.0/24:*"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 80, false)
	assert.True(t, result, "test ip range with wildcard port")
	result, _, _ = filters.Match("192.168.1.254", 81, false)
	assert.True(t, result, "test ip range with wildcard port")

	filters.Clear()
	if err := filters.Append("0.0.0.0:*"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 80, false)
	assert.True(t, result, "test wildcard ip with wildcard port")
	result, _, _ = filters.Match("192.168.1.2", 82, false)
	assert.True(t, result, "test wildcard ip with wildcard port")

	filters.Clear()
	if err := filters.Append("0.0.0.0:8080"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 80, false)
	assert.False(t, result, "test wildcard ip with port")
	result, _, _ = filters.Match("192.168.1.2", 8080, false)
	assert.True(t, result, "test wildcard ip with port")

	filters.Clear()
	if err := filters.Append("0.0.0.0:80-82"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 80, false)
	assert.True(t, result, "test wildcard ip with port range")
	result, _, _ = filters.Match("192.168.1.2", 81, false)
	assert.True(t, result, "test wildcard ip with port range")
	result, _, _ = filters.Match("192.168.1.3", 82, false)
	assert.True(t, result, "test wildcard ip with port range")
	result, _, _ = filters.Match("192.168.1.1", 83, false)
	assert.False(t, result, "test wildcard ip with port range")

	// check host/container ip
	filters.Clear()
	if err := filters.Append("host:80-82"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 81, false)
	assert.True(t, result)
	result, _, _ = filters.Match("192.168.1.1", 81, true)
	assert.False(t, result)
	filters.Clear()
	if err := filters.Append("container:80-82"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 81, true)
	assert.True(t, result)
	result, _, _ = filters.Match("192.168.1.1", 81, false)
	assert.False(t, result)


	// check with multi-value
	filters.Clear()
	if err := filters.Append("192.168.1.1:80,192.168.1.2:81,0.0.0.0:83-84"); err != nil {
		t.Logf("error: %s\n", err.Error())
	}
	result, _, _ = filters.Match("192.168.1.1", 80, false)
	assert.True(t, result)
	result, _, _ = filters.Match("192.168.1.2", 81, false)
	assert.True(t, result)
	result, _, _ = filters.Match("192.168.1.3", 83, false)
	assert.True(t, result)
	result, _, _ = filters.Match("10.0.0.1", 84, false)
	assert.True(t, result)
	result, _, _ = filters.Match("192.168.1.3", 81, false)
	assert.False(t, result)

}
