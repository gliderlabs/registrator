# Run Reference

Registrator is designed to be run once on every host. You *could* run a single
Registrator for your cluster, but you get better scaling properties and easier
configuration by ensuring Registrator runs on every host. Assuming some level of
automation, running everywhere is ironically simpler than running once somewhere.

## Running Registrator

    docker run [docker options] gliderlabs/registrator[:tag] [options] <registry uri>

Registrator requires and recommends some Docker options, has its own set of options
and then requires a Registry URI. Here is a typical way to run Registrator:

    $ docker run -d \
        --name=registrator \
        --net=host \
        --volume=/var/run/docker.sock:/tmp/docker.sock \
        gliderlabs/registrator:latest \
          consul://localhost:8500

## Docker Options

Option                                           | Required    | Description
------                                           | --------    | -----------
`--volume=/var/run/docker.sock:/tmp/docker.sock` | yes         | Allows Registrator to access Docker API
`--net=host`                                     | recommended | Helps Registrator get host-level IP and hostname

An alternative to host network mode would be to set the container hostname to the host
hostname (`-h $HOSTNAME`) and using the `-ip` Registrator option below.

## Registrator Options

Option                           | Since | Description
------                           | ----- | -----------
`-cleanup`                       | v7    | Cleanup dangling services
`-deregister <mode>`             | v6    | Deregister exited services "always" or "on-success". Default: always
`-internal`                      |       | Use exposed ports instead of published ports
`-ip <ip address>`               |       | Force IP address used for registering services
`-resync <seconds>`              | v6    | Frequency all services are resynchronized. Default: 0, never
`-retry-attempts <number>`       | v7    | Max retry attempts to establish a connection with the backend
`-retry-interval <milliseconds>` | v7    | Interval (in millisecond) between retry-attempts
`-tags <tags>`                   | v5    | Force comma-separated tags on all registered services
`-ttl <seconds>`                 |       | TTL for services. Default: 0, no expiry (supported backends only)
`-ttl-refresh <seconds>`         |       | Frequency service TTLs are refreshed (supported backends only)
`-useIpFromLabel <label>`        |       | Uses the IP address stored in the given label, which is assigned to a container, for registration with Consul
`-useIpFromEnv <env>`        	 |       | Uses the IP address from the given environment variable, which is assigned to a container, for registration with Consul

If the `-internal` option is used, Registrator will register the docker0
internal IP and port instead of the host mapped ones.

By default, when registering a service, Registrator will assign the service
address by attempting to resolve the current hostname. If you would like to
force the service address to be a specific address, you can specify the `-ip`
argument.

For registry backends that support TTL expiry, Registrator can both set and
refresh service TTLs with `-ttl` and `-ttl-refresh`.

If you want unlimited retry-attempts use `-retry-attempts -1`.

The `-resync` options controls how often Registrator will query Docker for all
containers and reregister all services.  This allows Registrator and the service
registry to get back in sync if they fall out of sync. Use this option with caution
as it will notify all the watches you may have registered on your services, and
may rapidly flood your system (e.g. consul-template makes extensive use of watches).

## Consul ACL token

If consul is configured to require an ACL token, Registrator needs to know about it,
or you will see warnings in the consul docker container

    [WARN] consul.catalog: Register of service 'redis' on 'hostname' denied due to ACLs

The ACL token is passed in through docker in an environment variable called `CONSUL_HTTP_TOKEN`.

    $ docker run -d \
        --name=registrator \
        --net=host \
        --volume=/var/run/docker.sock:/tmp/docker.sock \
        -e CONSUL_HTTP_TOKEN=<your acl token> \
        gliderlabs/registrator:latest \
          consul://localhost:8500

## Registry URI

    <backend>://<address>[/<path>]

The registry backend to use is defined by a URI. The scheme is the supported
registry name. The address is a host or host and port used to connect to the
registry. Some registries support a path definition used, for example, as the prefix to use
in service definitions for key-value based registries.

For full reference of supported backends, see [Registry Backends](backends.md).
