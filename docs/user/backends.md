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
    

## Eureka

The Eureka backend (based on uses a few conventions that can be overridden with container attributes.  
By default, containers will be registered with a datacenter of Amazon, with a public 
hostname and IP of the host IP address, and a local hostname and IP of either the
docker-assigned internal IP address if the `-internal` flag is set or
the host ip if not.

The following defaults are set and can be overridden with service attributes:
```
	SERVICE_EUREKA_STATUS = UP
	SERVICE_EUREKA_VIP = Service IP (ignored if using SERVICE_EUREKA_REGISTER_AWS_PUBLIC_IP)
	SERVICE_EUREKA_IPADDR = Service IP (ignored if using SERVICE_EUREKA_REGISTER_AWS_PUBLIC_IP)
	SERVICE_EUREKA_LEASEINFO_RENEWALINTERVALINSECS = 30
	SERVICE_EUREKA_LEASEINFO_DURATIONINSECS = 90
	SERVICE_EUREKA_DATACENTERINFO_NAME = Amazon
```


To set custom eureka metadata for your own purposes, you can use service attributes prefixed with SERVICE_EUREKA_METADATA_, e.g.:
```
	SERVICE_EUREKA_METADATA_MYROUTES=/route1*|/route2*
	SERVICE_EUREKA_METADATA_BE_AWESOME=true
```
These will appear in eureka inside a metadata tag.  See https://github.com/hudl/fargo/blob/master/metadata.go for some ideas on how to use them.



### AWS Datacenter Metadata Population

If the Amazon Datacenter type is used, the following additional values are supported:
```	
	
If the Amazon Datacenter type is used, the following additional values are supported:
```	
SERVICE_EUREKA_DATACENTERINFO_AUTO_POPULATE=false (if set to true, will attempt to populate datacenter info automatically)
SERVICE_EUREKA_DATACENTERINFO_PUBLICHOSTNAME = Host IP (ignored if using automatic population)
SERVICE_EUREKA_DATACENTERINFO_PUBLICIPV4 = Host IP (ignored if using automatic population)
SERVICE_EUREKA_DATACENTERINFO_LOCALIPV4 = Host or Container IP (depending on -internal flag, ignored if using automatic population)
SERVICE_EUREKA_DATACENTERINFO_LOCALHOSTNAME = Host or Container IP (depending on -internal flag, ignored if using automatic population)
SERVICE_EUREKA_REGISTER_AWS_PUBLIC_IP = false (if true, set VIP and IPADDR values to AWS public IP, ignored if NOT using automatic population)
SERVICE_EUREKA_LOOKUP_ELBV2_ENDPOINT = false (if true, an entry will be added for an ELBv2 connected to a container target group in ECS - see below for more details)
SERVICE_EUREKA_ELBV2_HOSTNAME = If set, will explicitly be used as the ELBv2 hostname - see below section.
SERVICE_EUREKA_ELBV2_PORT = If set, will be explicitly used as the ELBv2 Port - see below.

```
AWS datacenter metadata will be automatically populated.  _However_, the `InstanceID` will instead match `hostName`, which is the unique identifier for the container (Host_Port).  This is due to limitations in the eureka server.  
Instead, a new metadata tag, `aws_instanceID` has the underlying host instanceID.

For any of this to work, it requires properly functioning IAM roles for your container host.  On ECS, this will just work  - however, if you are running custom container hosts on EC2 with kubernetes or the like, then it may need further setup for the AWS metadata to work.

Have a look at this to help with that: https://github.com/jtblin/kube2iam - the concept is a metadata proxy, which will allow containers to see the underlying AWS metadata.


### Using ELBv2

If you are using ECS (EC2 Container Service) and run containers in target groups, with an Application Load Balancer in front of them (described by Amazon as ELBv2) then you can have registrator add the load 
balancer reference to eureka too.  It may work with custom EC2 instances behind a target group too, but has not been tested.  This has the following properties at present:

- It alters the Port, ipAddr and vipAddr entries in eureka to match the ELBv2 endpoint instead of the container.
- You will end up with multiple entries in eureka with the same endpoint, one for each container.  HostName is still set to the container IP and port combo
- Extra information added in metadata about being attached to the ELB; the `elbv2_endpoint` metadata and `has_elbv2` flag.

#### Automatic Lookups

If you set the flag `SERVICE_EUREKA_LOOKUP_ELBV2_ENDPOINT=true` AND you have `SERVICE_EUREKA_DATACENTERINFO_NAME = Amazon` then this feature is enabled.  

It will attempt to connect to the AWS service using the IAM role of the container host.  In ECS, this should just work.  It will find the region associated with the container host, and connect using that region.

#### Manual Endpoint Specification

If you specify `SERVICE_EUREKA_ELBV2_HOSTNAME=` and `SERVICE_EUREKA_ELBV2_PORT=` values on the container, then these will be used, rather than a lookup attempted.

If you specify the lookup flag, and also add these settings, the manual ones will take precedent.



#### IAM Policy
In order for this to work (you will receive a log error if not) the IAM role attached to the ECS host must have something like the following additional policy:
```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "Stmt1482328407000",
            "Effect": "Allow",
            "Action": [
                "elasticloadbalancing:DescribeInstanceHealth",
                "elasticloadbalancing:DescribeListeners",
                "elasticloadbalancing:DescribeLoadBalancerAttributes",
                "elasticloadbalancing:DescribeLoadBalancerPolicyTypes",
                "elasticloadbalancing:DescribeLoadBalancerPolicies",
                "elasticloadbalancing:DescribeLoadBalancers",
                "elasticloadbalancing:DescribeRules",
                "elasticloadbalancing:DescribeSSLPolicies",
                "elasticloadbalancing:DescribeTags",
                "elasticloadbalancing:DescribeTargetGroupAttributes",
                "elasticloadbalancing:DescribeTargetGroups",
                "elasticloadbalancing:DescribeTargetHealth"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}
```

