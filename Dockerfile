FROM gliderlabs/alpine:3.1
ENTRYPOINT ["/bin/registrator"]

ENV DOCKER_TLS_PATH="/certs" DOCKER_HOST="unix:///tmp/docker.sock"

COPY . /go/src/github.com/gliderlabs/registrator
RUN apk-install -t build-deps go git mercurial \
	&& cd /go/src/github.com/gliderlabs/registrator \
	&& export GOPATH=/go \
	&& go get \
	&& go build -ldflags "-X main.Version $(cat VERSION)" -o /bin/registrator \
	&& rm -rf /go \
	&& apk del --purge build-deps
