# Registrator

Service registry bridge for Docker

Registrator automatically register/deregisters services for Docker containers based on published ports and metadata from the container environment. Registrator supports [pluggable service registries](#adding-support-for-other-service-registries), which currently includes [Consul](http://www.consul.io/), [etcd](https://github.com/coreos/etcd) and [SkyDNS 2](https://github.com/skynetservices/skydns/).

By default, it can register services without any user-defined metadata. This means it works with *any* container, but allows the container author or Docker operator to override/customize the service definitions.

## Starting Registrator

Registrator assumes the default Docker socket at `file:///var/run/docker.sock` or you can override it with `DOCKER_HOST`. The only  mandatory argument is a registry URI, which specifies and configures the registry backend to use.

	$ registrator <registry-uri>

By default, when registering a service, registrator will assign the service address by attempting to resolve the current hostname. If you would like to force the service address to be a specific address, you can specify the `-ip` argument.

If the argument `-internal` is passed, registrator will register the docker0 internal ip and port instead of the host mapped ones. (etcd only for now)

The consul backend does not support automatic expiry of stale registrations after some TTL. Instead, TTL checks must be configured (see below). For backends that do support TTL expiry, registrator can be started with the `-ttl` and `-ttl-refresh` arguments (both disabled by default).

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

#### Vulcand backend

Vulcand support uses the vulcand API to register new endpoints on vulcand upstreams. Example URIs:

	$ registrator vulcand://vulcand.local:8182
	$ registrator vulcand://172.17.42.1:8182

If you want to use this Vulcand Backend, you should configure your Vulcand first, like add a host, upstream, location and a listener.

	$ vulcanctl --vulcan "http://vulcand.local:8182" host add --name myhost.tld
	$ vulcanctl --vulcan "http://vulcand.local:8182" upstream add --id infopage
	$ vulcanctl --vulcan "http://vulcand.local:8182" upstream add --id authbackend
	$ vulcanctl --vulcan "http://vulcand.local:8182" location add --id slash --host myhost.tld --path '/.*' --upstream infopage
	$ vulcanctl --vulcan "http://vulcand.local:8182" location add --id slash --host myhost.tld --path '/auth.*' --upstream authbackend
	$ vulcanctl --vulcan "http://vulcand.local:8182" listener add --host myhost.tld --proto 'http' --net 'tcp' --addr '0.0.0.0:80'

After that step you're able to use this vulcand registry service to register your endpoints automatically. The right upstream found via the hostname of the running docker container.

In this example we bind the port 80 from the container to the local docker0 interface, to avoid making it available from the outside - but also make it available for our vulcand. If you don't bind the port anywhere, it can't be registered to vulcand!

	$ docker run -d -p 172.17.42.1:80:80 --name authbackend-1 coreos/example:1.0.0
	$ docker run -d -p 172.17.42.1:81:80 --name authbackend-2 coreos/example:1.0.0
	$ docker run -d -p 172.17.42.1:82:80 --name authbackend-3 coreos/example:1.0.0
	$ docker run -d -p 172.17.42.1:83:80 --name authbackend-4-backup-only coreos/example:1.0.0
	$ docker run -d -p 172.17.42.1:84:80 --name infopage-1 coreos/example:2.0.0
	$ docker run -d -p 172.17.42.1:85:80 --name infopage-2 coreos/example:2.0.0
	$ docker run -d -p 172.17.42.1:86:80 --name infopage-3 coreos/example:2.0.0

We now registered 4 containers of example:1.0.0 to the authbackend upstream, and 3 example:2.0.0 to the infopage upstream.

Keep in mind, that you should't use any __-__ inside the upstream name, because it won't work then.

Allways name your containers with the following scheme: upstreamname-whateveryouwant

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

## Adding support for other service registries

As you can see by either the Consul or etcd source files, writing a new registry backend is easy. Just follow the example set by those two. It boils down to writing an object that implements this interface:

	type ServiceRegistry interface {
		Register(service *Service) error
		Deregister(service *Service) error
		Refresh(service *Service) error
	}

Then add your constructor (for example `NewZookeeperRegistry`) to the factory function `NewServiceRegistry` in `registrator.go`.

## Backend specific features

### Consul Health Checks

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

The default interval for any non-TTL check is 10s, but you can set it with `_CHECK_INTERVAL`.

#### Register a TTL health check

	SERVICE_CHECK_TTL=30s

Remember, this means Consul will be expecting a heartbeat ping within that 30 seconds to keep the service marked as healthy.


## Todo / Contribution Ideas

 * Zookeeper backend
 * discoverd backend
 * Netflix Eureka backend
 * etc...

## Sponsors and Thanks

This project was made possible by [DigitalOcean](http://digitalocean.com). Big thanks to Michael Crosby for [skydock](https://github.com/crosbymichael/skydock) and the Consul mailing list for inspiration.

## License

BSD
