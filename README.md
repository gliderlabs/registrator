# docksul

A Docker-Consul bridge that automatically registers containers with published ports as Consul services.

Although available standalone, docksul is meant to be built-in to the [progrium/consul](https://github.com/progrium/docker-consul) container and not used directly.

## What it does, how to use it

As Docker containers are started, docksul will inspect them for published ports and register them as services with Consul. As containers stop, the services are deregistered. 

If a container publishes one port, one service will be created. By default, the service will be named after the name of the container. You can override this by setting the environment variable `CONSUL_NAME`. 

If a container publishes more than one port, a service will be created for each published port. By default, the service will be named using the name of the container and the *internal* port published. For example `docker run -p 8000:80 --name foobar progrium/foobar` will result in a service called `foobar-80`, listening on port `8000`. 

You can override each port's service name by setting the environment variable `CONSUL_{port}_NAME` where port is the *internal* port. In the previous example, that means `CONSUL_80_NAME`. 

All published ports using UDP will produce services with the tag `udp`. You can add more tags to a service by setting `CONSUL_TAGS` or `CONSUL_{port}_TAGS` to a comma-delimited list of tags. 

## Todo

 * set check command via environment variable

## Inspiration

I originally wanted to solve this problem slightly differently, but discussions on the Consul mailing list made it clear this was in demand. Big credit to Michael Crosby for [skydock](https://github.com/crosbymichael/skydock), as this is what everybody wanted "but for Consul".

## Sponsors

This project was made possible by [DigitalOcean](http://digitalocean.com).

## License

BSD