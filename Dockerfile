FROM alpine:3.5
ENTRYPOINT ["/bin/registrator"]

COPY . /go/src/github.com/gliderlabs/registrator
RUN apk --no-cache add -t build-deps build-base go git \
	&& apk --no-cache add ca-certificates \
	&& export GOPATH=/go \
        && go get -u github.com/ugorji/go/codec/codecgen \
	&& mkdir /go/src/github.com/coreos \
	&& git clone https://github.com/coreos/go-etcd.git /go/src/github.com/coreos/go-etcd  \
        && cd /go/src/github.com/coreos/go-etcd/etcd \
        && /go/bin/codecgen -d 1978 -o response.generated.go response.go \
	&& cd /go/src/github.com/gliderlabs/registrator \
  && git config --global http.https://gopkg.in.followRedirects true \
	&& go get \
	&& go build -ldflags "-X main.Version=$(cat VERSION)" -o /bin/registrator \
	&& rm -rf /go \
	&& apk del --purge build-deps
