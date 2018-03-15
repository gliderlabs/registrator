FROM alpine:3.7
ENTRYPOINT ["/bin/registrator"]

COPY . /go/src/github.com/pipedrive/registrator
RUN apk --no-cache add -t build-deps build-base go git \
	&& apk --no-cache add ca-certificates \
	&& cd /go/src/github.com/pipedrive/registrator \
	&& export GOPATH=/go \
	&& git config --global http.https://gopkg.in.followRedirects true \
	&& go get \
	&& go build -ldflags "-X main.Version=$(cat VERSION) -extldflags \"-static\"" -o /bin/registrator \
	&& rm -rf /go \
	&& apk del --purge build-deps
