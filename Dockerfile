FROM alpine:3.4
ENTRYPOINT ["/bin/registrator"]

COPY . /go/src/github.com/gliderlabs/registrator
RUN apk add --update build-base go git \
	&& cd /go/src/github.com/gliderlabs/registrator \
	&& export GOPATH=/go \
	&& go get \
	&& go build -ldflags "-X main.Version=$(cat VERSION)" -o /bin/registrator \
	&& rm -rf /go \
	&& apk del --purge build-base go git
