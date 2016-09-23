package main

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/codegangsta/cli"
	dockerapi "github.com/fsouza/go-dockerclient"
	"github.com/gliderlabs/pkg/usage"
	"github.com/gliderlabs/registrator/bridge"
)

var Version string

var versionChecker = usage.NewChecker("registrator", Version)

func assertError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type appConfig struct {
	resyncInterval int
	retryAttempts  int
	retryInterval  int
	bridgeConfig   bridge.Config
}

func globalFlags() []cli.Flag {
	flags := []cli.Flag{
		cli.BoolFlag{
			Name:   "debug",
			EnvVar: "DEBUG",
			Usage:  "Mode of debug verbosity",
		},
		cli.StringFlag{
			Name:   "ip",
			EnvVar: "HOST_IP",
			Usage:  "IP for ports mapped to the host",
		},
		cli.BoolFlag{
			Name:   "internal",
			EnvVar: "INTERNAL",
			Usage:  "Use internal ports instead of published ones",
		},
		cli.IntFlag{
			Name:   "ttl-refresh",
			EnvVar: "TTL_REFRESH",
			Usage:  "Frequency with which service TTLs are refreshed",
		},
		cli.IntFlag{
			Name:   "ttl",
			EnvVar: "TTL",
			Usage:  "TTL for services (default is no expiry)",
		},
		cli.StringFlag{
			Name:   "tags",
			EnvVar: "TAGS",
			Usage:  "Append tags for all registered services",
		},
		cli.IntFlag{
			Name:   "resync",
			EnvVar: "RESYNC",
			Usage:  "Frequency with which services are resynchronized",
		},
		cli.StringFlag{
			Name:   "deregister",
			EnvVar: "DEREGISTER",
			Value:  "always",
			Usage:  "Deregister exited services \"always\" or \"on-success\"",
		},
		cli.IntFlag{
			Name:   "retry-attempts",
			EnvVar: "RETRY_ATTEMPTS",
			Value:  0,
			Usage:  "Max retry attempts to establish a connection with the backend. Use -1 for infinite retries",
		},
		cli.IntFlag{
			Name:   "retry-interval",
			EnvVar: "RETRY_INTERVAL",
			Value:  2000,
			Usage:  "Interval (in millisecond) between retry-attempts.",
		},
		cli.BoolFlag{
			Name:   "cleanup",
			EnvVar: "CLEANUP",
			Usage:  "Remove dangling services",
		},
	}

	return flags
}

func setupApplication(c *cli.Context, config *appConfig) error {
	if len(c.Args()) != 1 {
		return errors.New("Missing required argument for registry URI.")
	}

	*config = appConfig{
		resyncInterval: c.Int("resync"),
		retryAttempts:  c.Int("retry-attempts"),
		retryInterval:  c.Int("retry-interval"),
		bridgeConfig: bridge.Config{
			HostIp:          c.String("ip"),
			Internal:        c.Bool("internal"),
			ForceTags:       c.String("tags"),
			RefreshTtl:      c.Int("ttl"),
			RefreshInterval: c.Int("ttl-refresh"),
			DeregisterCheck: c.String("deregister"),
			Cleanup:         c.Bool("cleanup"),
		},
	}

	if (config.bridgeConfig.RefreshTtl == 0 && config.bridgeConfig.RefreshInterval > 0) || (config.bridgeConfig.RefreshTtl > 0 && config.bridgeConfig.RefreshInterval == 0) {
		return errors.New("--ttl and --ttl-refresh must be specified together or not at all")
	} else if config.bridgeConfig.RefreshTtl < 0 || config.bridgeConfig.RefreshInterval < 0 {
		return errors.New("--ttl and --ttl-refresh must be positive")
	} else if config.bridgeConfig.RefreshTtl > 0 && config.bridgeConfig.RefreshTtl <= config.bridgeConfig.RefreshInterval {
		return errors.New("--ttl must be greater than --ttl-refresh")
	}

	if config.retryInterval <= 0 {
		return errors.New("--retry-interval must be greater than 0")
	}

	if config.bridgeConfig.DeregisterCheck != "always" && config.bridgeConfig.DeregisterCheck != "on-success" {
		return errors.New("--deregister must be \"always\" or \"on-success\"")
	}

	return nil
}

func defaultCmd(c *cli.Context, config *appConfig) {
	if config.bridgeConfig.HostIp != "" {
		log.Println("Forcing host IP to", config.bridgeConfig.HostIp)
	}

	if os.Getenv("DOCKER_HOST") == "" {
		os.Setenv("DOCKER_HOST", "unix:///tmp/docker.sock")
	}

	docker, err := dockerapi.NewClientFromEnv()
	assertError(err)

	b, err := bridge.New(docker, c.Args()[0], config.bridgeConfig)
	assertError(err)

	attempt := 0
	for config.retryAttempts == -1 || attempt <= config.retryAttempts {
		log.Printf("Connecting to backend (%v/%v)", attempt, config.retryAttempts)

		err = b.Ping()
		if err == nil {
			log.Printf("Connected to backend")
			break
		}

		if err != nil && attempt == config.retryAttempts {
			log.Fatal(err)
		}

		time.Sleep(time.Duration(config.retryInterval) * time.Millisecond)
		attempt++
	}

	// Start event listener before listing containers to avoid missing anything
	events := make(chan *dockerapi.APIEvents)
	assertError(docker.AddEventListener(events))
	log.Println("Listening for Docker events ...")

	b.Sync(false)

	quit := make(chan struct{})

	// Start the TTL refresh timer
	if config.bridgeConfig.RefreshInterval > 0 {
		ticker := time.NewTicker(time.Duration(config.bridgeConfig.RefreshInterval) * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					b.Refresh()
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
	}

	// Start the resync timer if enabled
	if config.resyncInterval > 0 {
		resyncTicker := time.NewTicker(time.Duration(config.resyncInterval) * time.Second)
		go func() {
			for {
				select {
				case <-resyncTicker.C:
					b.Sync(true)
				case <-quit:
					resyncTicker.Stop()
					return
				}
			}
		}()
	}

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			go b.Add(msg.ID)
		case "die":
			go b.RemoveOnExit(msg.ID)
		}
	}

	close(quit)
	log.Fatal("Docker event loop closed") // todo: reconnect?
}

func main() {
	app := cli.NewApp()
	app.Name = "Registrator"
	app.Usage = "Service registry bridge for Docker with pluggable adapters http://gliderlabs.com/registrator"
	app.Version = Version
	app.Flags = globalFlags()

	var config appConfig
	app.Before = func(c *cli.Context) error {
		return setupApplication(c, &config)
	}

	app.Action = func(c *cli.Context) {
		defaultCmd(c, &config)
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
