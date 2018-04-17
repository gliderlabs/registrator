# Backend Reference

Registrator supports a number of backing registries. In order for Registrator to
be useful, you need to be running one of these. Below are the Registry URIs to
use for supported backends and documentation specific to their features.

See also [Contributing Backends](../dev/backends.md).

## Consul

	consul://<address>:<port>
	consul-unix://<filepath>
	consul-tls://<address>:<port>

Consul is the recommended registry since it specifically models services for
service discovery with health checks.

If no address and port is specified, it will default to `127.0.0.1:8500`.

Consul supports tags but no arbitrary service attributes.

When using the `consul-tls` scheme, registrator communicates with Consul through TLS. You must set the following environment variables:
 * `CONSUL_CACERT` : CA file location
 * `CONSUL_TLSCERT` : Certificate file location
 * `CONSUL_TLSKEY` : Key location

### Consul HTTP Check

This feature is only available when using Consul 0.5 or newer. Containers
specifying these extra metadata in labels or environment will be used to
register an HTTP health check with the service.

```bash
SERVICE_80_CHECK_HTTP=/health/endpoint/path
SERVICE_80_CHECK_INTERVAL=15s
SERVICE_80_CHECK_TIMEOUT=1s		# optional, Consul default used otherwise
```

It works for services on any port, not just 80. If its the only service,
you can also use `SERVICE_CHECK_HTTP`.

### Consul HTTPS Check

This feature is only available when using Consul 0.5 or newer. Containers
specifying these extra metedata in labels or environment will be used to
register an HTTPS health check with the service.

```bash
SERVICE_443_CHECK_HTTPS=/health/endpoint/path
SERVICE_443_CHECK_INTERVAL=15s
SERVICE_443_CHECK_TIMEOUT=1s		# optional, Consul default used otherwise
```

### Consul TCP Check

This feature is only available when using Consul 0.6 or newer. Containers
specifying these extra metadata in labels or environment will be used to
register an TCP health check with the service.

```bash
SERVICE_443_CHECK_TCP=true
SERVICE_443_CHECK_INTERVAL=15s
SERVICE_443_CHECK_TIMEOUT=3s		# optional, Consul default used otherwise
```

### Consul Script Check

This feature is tricky because it lets you specify a script check to run from
Consul. If running Consul in a container, you're limited to what you can run
from that container. For example, curl must be installed for this to work:

```bash
SERVICE_CHECK_SCRIPT=curl --silent --fail example.com
```

The default interval for any non-TTL check is 10s, but you can set it with
`_CHECK_INTERVAL`. The check command will be interpolated with the `$SERVICE_IP`
and `$SERVICE_PORT` placeholders:

```bash
SERVICE_CHECK_SCRIPT=nc $SERVICE_IP $SERVICE_PORT | grep OK
```

### Consul TTL Check

You can also register a TTL check with Consul. Keep in mind, this means Consul
will expect a regular heartbeat ping to its API to keep the service marked
healthy.

```bash
SERVICE_CHECK_TTL=30s
```

### Consul Initial Health Check Status

By default when a service is registered against Consul, the state is set to "critical". You can specify the initial health check status.

```bash
SERVICE_CHECK_INITIAL_STATUS=passing
```

## Consul KV

	consulkv://<address>:<port>/<prefix>
	consulkv-unix://<filepath>:/<prefix>

This is a separate backend to use Consul's key-value store instead of its native
service catalog. This behaves more like etcd since it has similar semantics, but
currently doesn't support TTLs.

If no address and port is specified, it will default to `127.0.0.1:8500`.

Using the prefix from the Registry URI, service definitions are stored as:

	<prefix>/<service-name>/<service-id> = <ip>:<port>

## Etcd

	etcd://<address>:<port>/<prefix>

Etcd works similar to Consul KV, except supports service TTLs. It also currently
doesn't support service attributes/tags.

If no address and port is specified, it will default to `127.0.0.1:2379`.

Using the prefix from the Registry URI, service definitions are stored as:

	<prefix>/<service-name>/<service-id> = <ip>:<port>

## SkyDNS 2

	skydns2://<address>:<port>/<domain>

SkyDNS 2 uses etcd, so this backend writes service definitions in a format compatible with SkyDNS 2.
The path may not be omitted and must be a valid DNS domain for SkyDNS.

If no address and port is specified, it will default to `127.0.0.1:2379`.

Using a Registry URI with the domain `cluster.local`, service definitions are stored as:

	/skydns/local/cluster/<service-name>/<service-id> = {"host":"<ip>","port":<port>}

SkyDNS requires the service ID to be a valid DNS hostname, so this backend requires containers to
override service ID to a valid DNS name. Example:

	$ docker run -d --name redis-1 -e SERVICE_ID=redis-1 -p 6379:6379 redis

## Zookeeper Store

The Zookeeper backend lets you publish ephemeral znodes into zookeeper. This mode is enabled by specifying a zookeeper path.  The zookeeper backend supports publishing a json znode body complete with defined service attributes/tags as well as the service name and container id. Example URIs:

	$ registrator zookeeper://zookeeper.host/basepath
	$ registrator zookeeper://192.168.1.100:9999/basepath

Within the base path specified in the zookeeper URI, registrator will create the following path tree containing a JSON entry for the service:

	<service-name>/<service-port> = <JSON>

The JSON will contain all infromation about the published container service. As an example, the following container start:

     docker run -i -p 80 -e 'SERVICE_80_NAME=www' -t ubuntu:14.04 /bin/bash

Will result in the zookeeper path and JSON znode body:

    /basepath/www/80 = {"Name":"www","IP":"192.168.1.123","PublicPort":49153,"PrivatePort":80,"ContainerID":"9124853ff0d1","Tags":[],"Attrs":{}}

## AWS Service Discovery

	aws-sd://<namespaceID>

AWS service discovery expects a namespace to already exist. You can read more about creating a namespace from the Amazon [Documentation](https://docs.aws.amazon.com/Route53/latest/APIReference/API_autonaming_CreatePrivateDnsNamespace.html).

This backend uses the service name to look up services from the specified namespace (e.g. linkerd-4140). It is recommended that you use docker labels on your containers to specify `SERVICE_NAME` (e.g. linkerd), and run Registrator in explicit mode.

Setting the environment variable `AUTOCREATE_SERVICES` to true will allow Registrator to create service entries in AWS Service Discovery. This is not recommended for production. Ideally the AWS services should be [manually](https://docs.aws.amazon.com/Route53/latest/APIReference/API_autonaming_CreateService.html) specified, with the name of the service being the same as the `SERVICE_NAME` label of the docker containers. If the service uses multiple ports, multiple AWS services need to be created. The AWS service names should include the port, i.e. `SERVICE_NAME-<PORT>`.

The environment variable `OPERATION_TIMEOUT` can be used to specify how long to continue checking the AWS RegisterInstance and DeregisterInstance status. It defaults to 10 seconds.

If running from an ecs container, you can use the AWS meta-data endpoint to obtain the IP address to pass to registrator. This can be specified in the task definition as the `entryPoint`.

	["sh", "-c", "registrator -e -ip $(curl 169.254.169.254/latest/meta-data/local-ipv4) aws-sd://<NAMESPACE_ID>"]

### Warnings
Changes made to AWS task definitions may result in unclean exit codes for containers. Unclean exits are ignored by Registrator. E.g. if updating the `SERVICE_NAME` label, the containers will most likely not be deregistered due to an unclean shutdown. It is recommended that you stop the containers cleanly and restart them with the new definition. Otherwise, you can use the `-cleanup -resync` options to periodically clean up dangling services.

Due to a 64 character limit on the instance ID field in AWS service Discovery, you may wish to use the registrator container with the `-hash-id` flag. This will replace the generated container names in the unique ID, such as those created by AWS ECS, with 40 character long hashes.