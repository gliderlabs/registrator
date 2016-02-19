package main

import (
	"flag"
	"os"
	"testing"

	"github.com/codegangsta/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestCli(t *testing.T) {
	suite.Run(t, new(CliSuite))
}

type CliSuite struct {
	suite.Suite
	globalSet *flag.FlagSet
}

func (suite *CliSuite) SetupTest() {
	os.Clearenv()
	flags := globalFlags()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range flags {
		f.Apply(set)
	}
	suite.globalSet = set
}

func (suite *CliSuite) TestRegisterFlagOk() {
	var config appConfig
	ep := "consul://1.2.3.4:55"
	suite.globalSet.Parse([]string{ep})
	ctx := cli.NewContext(nil, suite.globalSet, nil)
	err := setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "Should not throw error")
	assert.Equal(suite.T(), ep, ctx.Args()[0], "EP not equals")
}

func (suite *CliSuite) TestRegisterFlagError() {
	var config appConfig
	ctx := cli.NewContext(nil, suite.globalSet, nil)
	err := setupApplication(ctx, &config)
	assert.NotNil(suite.T(), err, "Should throw error register empty")
	assert.EqualError(suite.T(), err, "Missing required argument for registry URI.", "Should throw error register empty")
}

func (suite *CliSuite) TestRefreshFlag() {
	var config appConfig
	suite.globalSet.Parse([]string{"consul://1.2.3.4:55"})
	ctx := cli.NewContext(nil, suite.globalSet, nil)
	err := setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "Should not throw error")

	suite.globalSet.Set("ttl", "10")
	suite.globalSet.Set("ttl-refresh", "0")
	err = setupApplication(ctx, &config)
	assert.EqualError(suite.T(), err, "--ttl and --ttl-refresh must be specified together or not at all", "ttl and ttl-refresh must be set")

	suite.globalSet.Set("ttl", "0")
	suite.globalSet.Set("ttl-refresh", "10")
	err = setupApplication(ctx, &config)
	assert.EqualError(suite.T(), err, "--ttl and --ttl-refresh must be specified together or not at all", "ttl and ttl-refresh must be set")

	suite.globalSet.Set("ttl", "1")
	suite.globalSet.Set("ttl-refresh", "10")
	err = setupApplication(ctx, &config)
	assert.EqualError(suite.T(), err, "--ttl must be greater than --ttl-refresh", "ttl-refresh < ttl")

	suite.globalSet.Set("ttl", "-1")
	suite.globalSet.Set("ttl-refresh", "10")
	err = setupApplication(ctx, &config)
	assert.EqualError(suite.T(), err, "--ttl and --ttl-refresh must be positive", "ttl-refresh and ttl must be > 0")

	suite.globalSet.Set("ttl", "10")
	suite.globalSet.Set("ttl-refresh", "-1")
	err = setupApplication(ctx, &config)
	assert.EqualError(suite.T(), err, "--ttl and --ttl-refresh must be positive", "ttl-refresh and ttl must be > 0")

	suite.globalSet.Set("ttl", "10")
	suite.globalSet.Set("ttl-refresh", "1")
	err = setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "ttl > ttl-refresh must be ok")
}

func (suite *CliSuite) TestRetryIntervalFlag() {
	var config appConfig
	suite.globalSet.Parse([]string{"consul://1.2.3.4:55"})
	ctx := cli.NewContext(nil, suite.globalSet, nil)
	err := setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "Should not throw error")

	suite.globalSet.Set("retry-interval", "-1")
	err = setupApplication(ctx, &config)
	assert.NotNil(suite.T(), err, "retry-interval -1 is invalid")
	assert.EqualError(suite.T(), err, "--retry-interval must be greater than 0", "retry-interval -1 is invalid")

	suite.globalSet.Set("retry-interval", "1")
	err = setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "retry-interval 1 must be ok")
}

func (suite *CliSuite) TestDeregisterFlag() {
	var config appConfig
	suite.globalSet.Parse([]string{"consul://1.2.3.4:55"})
	ctx := cli.NewContext(nil, suite.globalSet, nil)
	err := setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "Should not throw error")

	suite.globalSet.Set("deregister", "foo")
	err = setupApplication(ctx, &config)
	assert.NotNil(suite.T(), err, "invalid deregister flag")
	assert.EqualError(suite.T(), err, "--deregister must be \"always\" or \"on-success\"", "invalid deregister flag")

	suite.globalSet.Set("deregister", "always")
	err = setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "always is a valid flag")

	suite.globalSet.Set("deregister", "on-success")
	err = setupApplication(ctx, &config)
	assert.Nil(suite.T(), err, "on-success is a valid flag")
}

func (suite *CliSuite) TestDefaultVars() {
	app := cli.NewApp()
	app.Flags = globalFlags()

	var config appConfig
	app.Before = func(c *cli.Context) error {
		return setupApplication(c, &config)
	}

	app.Action = func(ctx *cli.Context) {
		assert.NotNil(suite.T(), config, "config can not be nil")
		assert.NotNil(suite.T(), config.bridgeConfig, "bridge config can not be nil")
		assert.Equal(suite.T(), "", config.bridgeConfig.HostIp, "")
		assert.Equal(suite.T(), false, config.bridgeConfig.Internal, "")
		assert.Equal(suite.T(), 0, config.bridgeConfig.RefreshInterval, "")
		assert.Equal(suite.T(), 0, config.bridgeConfig.RefreshTtl, "")
		assert.Equal(suite.T(), "", config.bridgeConfig.ForceTags, "")
		assert.Equal(suite.T(), 0, config.resyncInterval, "")
		assert.Equal(suite.T(), "always", config.bridgeConfig.DeregisterCheck, "")
		assert.Equal(suite.T(), 0, config.retryAttempts, "")
		assert.Equal(suite.T(), 2000, config.retryInterval, "")
		assert.Equal(suite.T(), false, config.bridgeConfig.Cleanup, "")
	}

	app.Run([]string{"registrator", "consul://1.2.3.4:55"})
}

func setEnvVars() {
	os.Setenv("HOST_IP", "1.2.3.4")
	os.Setenv("INTERNAL", "true")
	os.Setenv("TTL_REFRESH", "111")
	os.Setenv("TTL", "222")
	os.Setenv("TAGS", "var_tags")
	os.Setenv("RESYNC", "333")
	os.Setenv("DEREGISTER", "on-success")
	os.Setenv("RETRY_ATTEMPTS", "444")
	os.Setenv("RETRY_INTERVAL", "555")
	os.Setenv("CLEANUP", "true")
}

func (suite *CliSuite) TestEnvVars() {
	setEnvVars()
	app := cli.NewApp()
	app.Flags = globalFlags()

	var config appConfig
	app.Before = func(c *cli.Context) error {
		return setupApplication(c, &config)
	}

	app.Action = func(ctx *cli.Context) {
		assert.NotNil(suite.T(), config, "config can not be nil")
		assert.NotNil(suite.T(), config.bridgeConfig, "bridge config can not be nil")
		assert.Equal(suite.T(), "1.2.3.4", config.bridgeConfig.HostIp, "")
		assert.Equal(suite.T(), true, config.bridgeConfig.Internal, "")
		assert.Equal(suite.T(), 111, config.bridgeConfig.RefreshInterval, "")
		assert.Equal(suite.T(), 222, config.bridgeConfig.RefreshTtl, "")
		assert.Equal(suite.T(), "var_tags", config.bridgeConfig.ForceTags, "")
		assert.Equal(suite.T(), 333, config.resyncInterval, "")
		assert.Equal(suite.T(), "on-success", config.bridgeConfig.DeregisterCheck, "")
		assert.Equal(suite.T(), 444, config.retryAttempts, "")
		assert.Equal(suite.T(), 555, config.retryInterval, "")
		assert.Equal(suite.T(), true, config.bridgeConfig.Cleanup, "")
	}

	app.Run([]string{"registrator", "consul://1.2.3.4:55"})
}

func (suite *CliSuite) TestFlagsOverEnvVars() {
	setEnvVars()
	app := cli.NewApp()
	app.Flags = globalFlags()

	var config appConfig
	app.Before = func(c *cli.Context) error {
		return setupApplication(c, &config)
	}

	app.Action = func(ctx *cli.Context) {
		assert.NotNil(suite.T(), config, "config can not be nil")
		assert.NotNil(suite.T(), config.bridgeConfig, "bridge config can not be nil")
		assert.Equal(suite.T(), "5.5.5.5", config.bridgeConfig.HostIp, "")
		assert.Equal(suite.T(), true, config.bridgeConfig.Internal, "")
		assert.Equal(suite.T(), 111, config.bridgeConfig.RefreshInterval, "")
		assert.Equal(suite.T(), 222, config.bridgeConfig.RefreshTtl, "")
		assert.Equal(suite.T(), "bar", config.bridgeConfig.ForceTags, "")
		assert.Equal(suite.T(), 777, config.resyncInterval, "")
		assert.Equal(suite.T(), "on-success", config.bridgeConfig.DeregisterCheck, "")
		assert.Equal(suite.T(), 444, config.retryAttempts, "")
		assert.Equal(suite.T(), 555, config.retryInterval, "")
		assert.Equal(suite.T(), true, config.bridgeConfig.Cleanup, "")
	}

	app.Run([]string{"registrator", "--ip", "5.5.5.5", "--tags", "bar", "--resync", "777", "consul://1.2.3.4:55"})
}
