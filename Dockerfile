FROM progrium/busybox
MAINTAINER Jeff Lindsay <progrium@gmail.com

ADD ./stage/docksul /bin/docksul

ENV DOCKER_HOST /tmp/docker.sock

ENTRYPOINT ["/bin/docksul"]