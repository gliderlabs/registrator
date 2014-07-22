FROM progrium/busybox
MAINTAINER Jeff Lindsay <progrium@gmail.com

ADD ./stage/dockser /bin/dockser

ENV DOCKER_HOST unix:///tmp/docker.sock

ENTRYPOINT ["/bin/dockser"]