# docksul

A Docker-Consul bridge that automatically registers containers with published ports as Consul services. As Docker containers are started, docksul will inspect them for published ports and register them as services with Consul. As containers stop, the services are deregistered. If the default service descriptions are unsuitable, you can customize them with environment variables on the container.

Although available standalone, docksul is used as a component of Consulate and it's recommended you use Consulate instead unless you know what you're doing. 

## Starting docksul

docksul assumes the default Docker socket at `file:///var/run/docker.sock` or you can override it with `DOCKER_HOST`. It also uses `0.0.0.0:8500` for Consul, but you can override it by passing an IP and port as an argument. 

	$ docksul [consul-addr]

You can run it as a container, but you must pass the Docker socket file as a mount to `/tmp/docker.sock`:

	$ docker run -d -v /var/run/docker.sock:/tmp/docker.sock progrium/docksul [consul-addr]

## How it works

### One published port, the simple case

If a container publishes one port, one service will be created using the host port. By default, the service will be named after the base name of the image. For example:

	$ docker run -d --name redis.0 -p 10000:6379 dockerfile/redis

Will result in a service:

	{
		"id": "<nodename>/redis.0:6379",
		"name": "redis",
		"port": 10000,
		"tags": []
	}

The service ID is a unique identifier for this service instance. It's produced by the Consul agent's node name (often the hostname), then the name of the container, then the exposed port. You rarely need to use the ID since Consul lookups are done by name.

You can override service name by setting the environment variable `SERVICE_NAME`. You also don't have to specify a host port, as it will use the automatically assigned one if not provided.

	$ docker run -d --name redis.0 -e "SERVICE_NAME=db" -p 6379 dockerfile/redis	

Results in the service:

	{
		"id": "<nodename>/redis.0:6379",
		"name": "db",
		"port": 23210,
		"tags": []
	}

You can also specify tags with a comma-delimited list. If you publish a port on UDP, it will automatically get a `udp` tag.

	$ docker run -d --name consul -p 53/udp -e "SERVICE_TAGS=dns,backup" progrium/consul

Results in the service:

	{
		"id": "<nodename>/consul:53",
		"name": "consul",
		"port": 18279,
		"tags": ["dns", "backup", "udp"]
	}

### Multiple published ports

If a container publishes more than one port, a service will be created for each published port. By default, the services will be named using the base name of the image and the *internal* exposed port. For example:
	
	$ docker run -p 8000:80 -p 4443:443 --name nginx.0 progrium/nginx

Results in two services:

	[
		{
			"id": "<nodename>/nginx.0:80",
			"name": "nginx-80",
			"port": 8000,
			"tags": []
		},
		{
			"id": "<nodename>/nginx.0:443",
			"name": "nginx-443",
			"port": 4443,
			"tags": []
		}
	]

You can override each port's service name by setting the environment variable `SERVICE_{port}_NAME` where port is the *internal* exposed port. For example:

	$ docker run -p 8000:80 -p 4443:443 --name nginx.0 -e "SERVICE_80_NAME=http" -e "SERVICE_443_NAME=https" progrium/nginx

Resulting in:

	[
		{
			"id": "<nodename>/nginx.0:80",
			"name": "http",
			"port": 8000,
			"tags": []
		},
		{
			"id": "<nodename>/nginx.0:443",
			"name": "https",
			"port": 4443,
			"tags": []
		}
	]

Setting tags or any future service attributes would use the same prefix convention for multi service containers (ie `SERVICE_80_TAGS`).

## Todo

 * Support custom Consul checks with SERVICE_CHECK_SCRIPT and SERVICE_CHECK_INTERVAL variables

## Sponsors and Thanks

This project was made possible by [DigitalOcean](http://digitalocean.com). Big thanks to Michael Crosby for [skydock](https://github.com/crosbymichael/skydock).

## License

BSD