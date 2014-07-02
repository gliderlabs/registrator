FROM progrium/busybox
MAINTAINER Jeff Lindsay <progrium@gmail.com

ADD ./stage/docksul /bin/docksul

ENTRYPOINT ["/bin/docksul"]