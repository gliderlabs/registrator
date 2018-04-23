FROM alpine:3.7 as builder
ENTRYPOINT ["/bin/registrator"]

COPY . /go/src/github.com/gliderlabs/registrator
RUN apk --no-cache add -t build-deps build-base go git curl \
	&& apk --no-cache add ca-certificates \
	&& export GOPATH=/go && mkdir -p /go/bin && export PATH=$PATH:/go/bin \
	&& curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh \
	&& cd /go/src/github.com/gliderlabs/registrator \
	&& export GOPATH=/go \
        && go get -u github.com/ugorji/go/codec/codecgen \
	&& mkdir /go/src/github.com/coreos \
	&& git clone https://github.com/coreos/go-etcd.git /go/src/github.com/coreos/go-etcd  \
        && cd /go/src/github.com/coreos/go-etcd/etcd \
        && /go/bin/codecgen -d 1978 -o response.generated.go response.go \
	&& cd /go/src/github.com/gliderlabs/registrator \
  && git config --global http.https://gopkg.in.followRedirects true \
	&& dep ensure \
	&& go build -ldflags "-X main.Version=$(cat VERSION)" -o /bin/registrator \
	&& rm -rf /go \
	&& apk del --purge build-deps

FROM scratch
COPY --from=builder /bin/registrator /bin/registrator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bin/registrator"]
