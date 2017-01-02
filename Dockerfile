FROM alpine:3.5
ENTRYPOINT ["/bin/registrator"]

ENV GOPATH /go
COPY . /go/src/github.com/gliderlabs/registrator
RUN apk --no-cache add -t build-deps build-base go git glide \
	&& cd /go/src/github.com/gliderlabs/registrator \
	&& glide install \
	&& go build -ldflags "-X main.Version=$(cat VERSION)" -o /bin/registrator \
	&& rm -rf /go \
	&& glide cc \
	&& apk del --purge build-deps
