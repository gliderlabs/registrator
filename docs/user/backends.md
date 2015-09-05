# Backend Reference

Registrator supports a number of backing registries. In order for Registrator to
be useful, you need to be running one of these. Below are the Registry URIs to
use for supported backends and documentation specific to their features.

See also [Contributing Backends](../dev/backends.md).

## Consul

	consul://<address>:<port>

Consul is the recommended registry since it specifically models services for
service discovery with health checks.

If no address and port is specified, it will default to `127.0.0.1:8500`.

Consul supports tags but no arbitrary service attributes.

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

## Consul KV

	consulkv://<address>:<port>/<prefix>

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

If no address and port is specified, it will default to `127.0.0.1:4001`.

Using the prefix from the Registry URI, service definitions are stored as:

	<prefix>/<service-name>/<service-id> = <ip>:<port>

## SkyDNS 2

	skydns2://<address>:<port>/<domain>

SkyDNS 2 uses etcd, so this backend writes service definitions in a format compatible with SkyDNS 2.
The path may not be omitted and must be a valid DNS domain for SkyDNS.

If no address and port is specified, it will default to `127.0.0.1:4001`.

Using a Registry URI with the domain `cluster.local`, service definitions are stored as:

	/skydns/local/cluster/<service-name>/<service-id> = {"host":"<ip>","port":<port>}

SkyDNS requires the service ID to be a valid DNS hostname, so this backend requires containers to
override service ID to a valid DNS name. Example:

	$ docker run -d --name redis-1 -e SERVICE_ID=redis-1 -p 6379:6379 redis
