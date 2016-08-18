package bridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	dockerapi "github.com/fsouza/go-dockerclient"
)

func containerMetaDataGlobalOnly() *dockerapi.Config {
	cfg := new(dockerapi.Config)
	cfg.Env = []string {
		"SERVICE_NAME=something",
	}
	return cfg
}

func containerMetaDataGlobalAF() *dockerapi.Config {
	cfg := new(dockerapi.Config)
	cfg.Env = []string {
		"SERVICE_NAME=something",
		"SERVICE_NAME_IPV4=lollerskates",
		"SERVICE_NAME_IPV6=loll:ersk:ates",
	}
	return cfg
}

func containerMetaDataPort80() *dockerapi.Config {
	cfg := new(dockerapi.Config)
	cfg.Env = []string {
		"SERVICE_NAME=something",
		"SERVICE_NAME_IPV6=loll:ersk:ates",
		"SERVICE_80_NAME=somethingelse",
	}
	return cfg
}

func containerMetaDataPort80AF() *dockerapi.Config {
	cfg := new(dockerapi.Config)
	cfg.Env = []string {
		"SERVICE_NAME=something",
		"SERVICE_NAME_IPV4=ermahgerd",
		"SERVICE_NAME_IPV4=ermahgerd6",
		"SERVICE_80_NAME=somethingelse",
		"SERVICE_80_NAME_IPV4=lollerskates",
		"SERVICE_80_NAME_IPV6=loll:ersk:ates",
	}
	return cfg
}

func TestServiceMetaDataCustomName(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataGlobalOnly(), "80", false)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "something" })
}

func TestServiceMetaDataCustomPortName(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataPort80(), "80", false)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "somethingelse" })
}

func TestServiceMetaDataDifferentPort(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataPort80(), "443", false)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "something" })
}

func TestServiceMetaDataDifferentPortIPv6(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataPort80(), "443", true)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "loll:ersk:ates" })
}

func TestServiceMetaDataIPv4Override(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataGlobalAF(), "80", false)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "lollerskates" })
}

func TestServiceMetaDataIPv6Override(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataGlobalAF(), "80", true)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "loll:ersk:ates" })
}

func TestServiceMetaDataCustomPortIPv4Override(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataPort80AF(), "80", false)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "lollerskates" })
}

func TestServiceMetaDataCustomPortIPv6Override(t *testing.T) {
	metadata, _ := serviceMetaData(containerMetaDataPort80AF(), "80", true)

	assert.NotNil(t, metadata)
	assert.Equal(t, metadata, map[string]string { "name": "loll:ersk:ates" })
}
