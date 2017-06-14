FROM alpine:3.2
ENTRYPOINT ["/bin/usage-tracker"]

EXPOSE 53/udp

COPY . /go/src/github.com/gliderlabs/pkg/usage
RUN \
  apk add --update -t build-deps go git mercurial \
  && export GOPATH=/go \
  && cd /go/src/github.com/gliderlabs/pkg/usage \
  && go get ./... \
  && go build -o /bin/usage-tracker ./server \
  && apk del --purge build-deps \
  && rm -rf /go \
  && rm -rf /var/cache/apk/*
