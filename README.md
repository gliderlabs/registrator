# Registrator

Service registry bridge for Docker, sponsored by [Weave](http://weave.works).

Registrator automatically register/deregisters services for Docker containers based on published ports and metadata from the container environment. Registrator supports [pluggable service registries](#adding-support-for-other-service-registries), which currently includes [Consul](http://www.consul.io/), [etcd](https://github.com/coreos/etcd) and [SkyDNS 2](https://github.com/skynetservices/skydns/).

By default, it can register services without any user-defined metadata. This means it works with *any* container, but allows the container author or Docker operator to override/customize the service definitions.

## Getting Registrator

You can get the latest release of Registrator via Docker Hub:

	$ docker pull gliderlabs/registrator:latest

You can pull the last build in `master` with the `master` tag. If you want to get a specific release, you can download the release artifact listed in [Releases](https://github.com/gliderlabs/registrator/releases) and `docker load` them:

	$ curl -s https://dl.gliderlabs.com/registrator/v5.tgz | docker load

## Starting Registrator

Registrator was designed to just be run as a container. You must pass the Docker socket file as a mount to `/tmp/docker.sock`, and it's a good idea to set the hostname to the machine host:

	$ docker run -d \
		-v /var/run/docker.sock:/tmp/docker.sock \
		-h $HOSTNAME gliderlabs/registrator <registry-uri>

By default, when registering a service, registrator will assign the service address by attempting to resolve the current hostname. If you would like to force the service address to be a specific address, you can specify the `-ip` argument.

If the argument `-internal` is passed, registrator will register the docker0 internal ip and port instead of the host mapped ones. (etcd, consul, and skydns2 for now). The `-internal` argument must be passed before the `<registry-uri>` argument.

The `-resync` argument controls how often registrator will query Docker for all containers and reregister all services.  This allows registrator and the service registry to get back in sync if they fall out of sync.  The time is measured in seconds, and if set to zero will not resync.

The consul backend does not support automatic expiry of stale registrations after some TTL. Instead, TTL checks must be configured (see below). For backends that do support TTL expiry, registrator can be started with the `-ttl` and `-ttl-refresh` arguments (both disabled by default).

### Host mode services

Registrator will discover services running in Docker net=host mode provided that their exposed ports are explicitly listed using the -p flag:

	$ docker run --net=host -p 8080:8080 -p 8443:8443 ...

### Registry URIs

The registry backend to use is defined by a URI. The scheme is the supported registry name, and an address. Registries based on key-value stores like etcd and Zookeeper (not yet supported) can specify a key path to use to prefix service definitions. Registries may also use query params for other options. See also [Adding support for other service registries](#adding-support-for-other-service-registries).

#### Consul Service Catalog (recommended)

To use the Consul service catalog, specify a Consul URI without a path. If no host is provided, `127.0.0.1:8500` is used. Examples:

	$ registrator consul://10.0.0.1:8500
	$ registrator consul:

This backend comes with support for specifying service health checks. See [backend specific features](#backend-specific-features).

#### Consul Key-value Store

The Consul backend also lets you just use the key-value store. This mode is enabled by specifying a path. Consul key-value support does not currently use service attributes/tags. Example URIs:

	$ registrator consul:///path/to/services
	$ registrator consul://192.168.1.100/services

Service definitions are stored as:

	<registry-uri-path>/<service-name>/<service-id> = <ip>:<port>

#### Etcd Key-value Store

Etcd support works similar to Consul key-value. It also currently doesn't support service attributes/tags. If no host is provided, `127.0.0.1:4001` is used. Example URIs:

	$ registrator etcd:///path/to/services
	$ registrator etcd://192.168.1.100:4001/services

Service definitions are stored as:

	<registry-uri-path>/<service-name>/<service-id> = <ip>:<port>

#### SkyDNS 2 backend

SkyDNS 2 support uses an etcd key-value store, writing service definitions in a format compatible with SkyDNS 2. The URI provides an etcd host and a DNS domain name. If no host is provided, `127.0.0.1:4001` is used. The DNS domain name may not be omitted. Example URIs:

	$ registrator skydns2:///skydns.local
	$ registrator skydns2://192.168.1.100:4001/staging.skydns.local

Using the second example, a service definition for a container with `service-name` "redis" and `service-id` "redis-1" would be stored in the etcd service at 192.168.1.100:4001 as follows:

	/skydns/local/skydns/staging/<service-name>/<service-id> = {"host":"<ip>","port":<port>}

Note that the default `service-id` includes more than the container name (see below). For legal per-container DNS hostnames, specify the `SERVICE_ID` in the environment of the container, e.g.:

	docker run -d --name redis-1 -e SERVICE_ID=redis-1 -p 6379:6379 redis

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

Additional supported metadata in the same format `SERVICE_<metadata>`.
IGNORE: Any value for ignore tells registrator to ignore this entire container and all associated ports.

Starting with Docker version 1.6 and up, you can use labels instead of environment variables to service definitions.

Since metadata is stored as environment variables or labels, the container author can include their own metadata defined in the Dockerfile. The operator will still be able to override these author-defined defaults.

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

Keep in mind not all of the `Service` object may be used by the registry backend. For example, currently none of them support registering arbitrary attributes. This field is there for future use.

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

### Using labels to define metadata

	$ docker run -d --name redis.0 -p 10000:6379 \
		-l "SERVICE_NAME=db" \
		-l "SERVICE_TAGS=master,backups" \
		-l "SERVICE_REGION=us2" dockerfile/redis

Results in `Service`:

	{
		"ID": "hostname:redis.0:6379",
		"Name": "db",
		"Port": 10000,
		"IP": "192.168.1.102",
		"Tags": ["master", "backups"],
		"Attrs": {"region": "us2"}
	}

## Adding support for other service registries

As you can see by either the Consul or etcd source files, writing a new registry backend is easy. Just follow the example set by those two. It boils down to writing an object that implements this interface:

	type RegistryAdapter interface {
		Ping() error
		Register(service *Service) error
		Deregister(service *Service) error
		Refresh(service *Service) error
	}

Then add a factory which accepts a uri and returns the registry adapter, and register that factory with the bridge like `bridge.Register(new(Factory), "<backend_name>")`.

## Backend specific features

### Consul Health Checks

> All health checking integration is going to change soon, so consider these features deprecated.

When using the Consul's service catalog backend, you can specify a health check associated with a service. Registrator can pull this from your container environment data if provided. Here are some examples:

#### Basic HTTP health check

This feature is only available when using the `check-http` script that comes with the [progrium/consul](https://github.com/progrium/docker-consul#health-checking-with-docker) container for Consul.

	SERVICE_80_CHECK_HTTP=/health/endpoint/path
	SERVICE_80_CHECK_INTERVAL=15s

It works for an HTTP service on any port, not just 80. If its the only service, you can also use `SERVICE_CHECK_HTTP`.

#### Run a health check script in the service container

This feature is only available when using the `check-cmd` script that comes with the [progrium/consul](https://github.com/progrium/docker-consul#health-checking-with-docker) container for Consul.

	SERVICE_9000_CHECK_CMD=/path/to/check/script

This runs the command using this service's container image as a separate container attached to the service's network namespace.

#### Run a regular command from the Consul container

	SERVICE_CHECK_SCRIPT=curl --silent --fail example.com

The default interval for any non-TTL check is 10s, but you can set it with `_CHECK_INTERVAL`. The check command will be
interpolated with the `$SERVICE_IP` and `$SERVICE_PORT` placeholders:

	SERVICE_CHECK_SCRIPT=nc $SERVICE_IP $SERVICE_PORT | grep OK

#### Register a TTL health check

	SERVICE_CHECK_TTL=30s

Remember, this means Consul will be expecting a heartbeat ping within that 30 seconds to keep the service marked as healthy.

## Contributing

As usual, pull requests are welcome. You can also propose releases by opening a PR against the release branch from master. Please be sure to bump the version and update CHANGELOG.md and include your changelog text in the PR body.

Discuss registrator development with us on Freenode in `#gliderlabs`.

## Todo / Contribution Ideas

 * Zookeeper backend
 * discoverd backend
 * Netflix Eureka backend
 * etc...

## Sponsors and Thanks

Ongoing support of this project is made possible by [Weave](http://weave.works), the Docker SDN. Big thanks to Michael Crosby for [skydock](https://github.com/crosbymichael/skydock) and the Consul mailing list for inspiration.

## License

BSD
