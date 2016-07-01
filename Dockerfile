FROM golang:1.6.3-alpine
ENTRYPOINT ["/bin/registrator"]

COPY . /go/src/github.com/gliderlabs/registrator
RUN apk add --no-cache git mercurial
RUN cd /go/src/github.com/gliderlabs/registrator \
	&& go get \
	&& go build -ldflags "-X main.Version=$(cat VERSION)" -o /bin/registrator
