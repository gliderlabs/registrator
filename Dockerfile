FROM progrium/busybox
MAINTAINER Jeff Lindsay <progrium@gmail.com

ADD ./stage/registrator /bin/registrator

ENV DOCKER_HOST unix:///tmp/docker.sock

ENTRYPOINT ["/bin/registrator"]