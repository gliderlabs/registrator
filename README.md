# Registrator

Service registry bridge for Docker

Registrator listens for Docker events and register/deregisters services for containers based on published ports and metadata from the container environment. Registrator supports [pluggable service registries](#adding-support-for-other-service-registries), which currently includes [Consul](http://www.consul.io/) and [etcd](https://github.com/coreos/etcd). 

By default, it can register services without any user-defined metadata. This means it works with *any* container, but allows the container author or Docker operator to override/customize the service definitions.

Registrator pairs well with [ambassadord](https://github.com/progrium/ambassadord) and together are part of upcoming opinionated discovery/routing solution [Consulate](https://github.com/progrium/consulate).

## Starting Registrator

Registrator assumes the default Docker socket at `file:///var/run/docker.sock` or you can override it with `DOCKER_HOST`. The only  mandatory argument is a registry URI, which specifies and configures the registry backend to use.

	$ registrator <registry-uri>

By default, when registering a service, registrator will assign the service address by attempting to resolve the current hostname. If you would like to force the service address to be a specific address, you can specify the `-ip` argument.

Registrator was designed to just be run as a container. You must pass the Docker socket file as a mount to `/tmp/docker.sock`, and it's a good idea to set the hostname to the machine host:

	$ docker run -d \
		-v /var/run/docker.sock:/tmp/docker.sock \
		-h $HOSTNAME progrium/registrator <registry-uri>

### Registry URIs

The registry backend to use is defined by a URI. The scheme is the supported registry name, and an address. Registries based on key-value stores like etcd and Zookeeper (not yet supported) can specify a key path to use to prefix service definitions. Registries may also use query params for other options. See also [Adding support for other service registries](#adding-support-for-other-service-registries).

#### Consul Service Catalog (recommended)

To use the Consul service catalog, specify a Consul URI without a path. If no host is provided, `127.0.0.1:8500` is used. Examples:

	$ registrator consul://10.0.0.1:8500
	$ registrator consul:

#### Consul Key-value Store

The Consul backend also lets you just use the key-value store. This mode is enabled by specifying a path. Consul key-value support does not currently use service attributes/tags. Example URIs:

	$ registrator consul:///path/to/services
	$ registrator consul://192.168.1.100/services

Service definitions are stored as:

	<registry-uri-path>/<service-name>/<service-id> = <ip>:<port>

#### Etcd Key-value Store

Etcd support works similar to Consul key-value. It also currently doesn't support service attributes/tags. If no host is provided, `127.0.0.1:4001` is used. Example URIs:

	$ registrator etcd:///path/to/services
	$ registrator etcd://192.168.1.100/services

Service definitions are stored as:

	<registry-uri-path>/<service-name>/<service-id> = <ip>:<port>

## How it works

Services are registered and deregistered based on container start and die events from Docker. The service definitions are created with information from the container, including user-defined metadata in the container environment.

For each published port of a container, a `Service` object is created and passed to the `ServiceRegistry` to register. A `Service` object looks like this with defaults explained in the comments:

	type Service struct {
		ID    string               // <hostname>:<container-name>:<internal-port>[:udp if udp]
		Name  string               // <basename(container-image)>[-<internal-port> if >1 published ports]
		Port  int                  // <host-port>
		IP    string               // <host-ip> || <resolve(hostname)> if 0.0.0.0
		Tags  []string             // empty, or includes 'udp' if udp
		Attrs map[string]string    // any remaining service metadata from environment
	}

Most of these (except `IP` and `Port`) can be overridden by container environment metadata variables prefixed with `SERVICE_` or `SERVICE_<internal-port>_`. You use a port in the key name to refer to a particular port's service. Metadata variables without a port in the name are used as the default for all services or can be used to conveniently refer to the single exposed service. 

Since metadata is stored as environment variables, the container author can include their own metadata defined in the Dockerfile. The operator will still be able to override these author-defined defaults.

### Single service with defaults

	$ docker run -d --name redis.0 -p 10000:6379 dockerfile/redis

Results in `Service`:

	{
		"ID": "hostname:redis.0:6379",
		"Name": "redis",
		"Port": 10000,
		"IP": "192.168.1.102",
		"Tags": [],
		"Attrs": {}
	}

### Single service with metadata

	$ docker run -d --name redis.0 -p 10000:6379 \
		-e "SERVICE_NAME=db" \
		-e "SERVICE_TAGS=master,backups" \
		-e "SERVICE_REGION=us2" dockerfile/redis

Results in `Service`:

	{
		"ID": "hostname:redis.0:6379",
		"Name": "db",
		"Port": 10000,
		"IP": "192.168.1.102",
		"Tags": ["master", "backups"],
		"Attrs": {"region": "us2"}
	}

### Multiple services with defaults

	$ docker run -d --name nginx.0 -p 4443:443 -p 8000:80 progrium/nginx

Results in two `Service` objects:

	[
		{
			"ID": "hostname:nginx.0:443",
			"Name": "nginx-443",
			"Port": 4443,
			"IP": "192.168.1.102",
			"Tags": [],
			"Attrs": {},
		},
		{
			"ID": "hostname:nginx.0:80",
			"Name": "nginx-80",
			"Port": 8000,
			"IP": "192.168.1.102",
			"Tags": [],
			"Attrs": {}
		}
	]

### Multiple services with metadata

	$ docker run -d --name nginx.0 -p 4443:443 -p 8000:80 \
		-e "SERVICE_443_NAME=https" \
		-e "SERVICE_443_ID=https.12345" \
		-e "SERVICE_443_SNI=enabled" \
		-e "SERVICE_80_NAME=http" \
		-e "SERVICE_TAGS=www" progrium/nginx

Results in two `Service` objects:

	[
		{
			"ID": "https.12345",
			"Name": "https",
			"Port": 4443,
			"IP": "192.168.1.102",
			"Tags": ["www"],
			"Attrs": {"sni": "enabled"},
		},
		{
			"ID": "hostname:nginx.0:80",
			"Name": "http",
			"Port": 8000,
			"IP": "192.168.1.102",
			"Tags": ["www"],
			"Attrs": {}
		}
	]

## Adding support for other service registries

As you can see by either the Consul or etcd source files, writing a new registry backend is easy. Just follow the example set by those two. It boils down to writing an object that implements this interface:

	type ServiceRegistry interface {
		Register(service *Service) error
		Deregister(service *Service) error
	}

Then add your constructor (for example `NewZookeeperRegistry`) to the factory function `NewServiceRegistry` in `registrator.go`.

## Todo / Contribution Ideas

 * Consul backend: support custom checks with SERVICE_CHECK_SCRIPT and SERVICE_CHECK_INTERVAL variables
 * Zookeeper backend
 * SkyDNS backend
 * discoverd backend
 * Netflix Eureka backend
 * etc...

## Sponsors and Thanks

This project was made possible by [DigitalOcean](http://digitalocean.com). Big thanks to Michael Crosby for [skydock](https://github.com/crosbymichael/skydock) and the Consul mailing list for inspiration.

## License

BSD