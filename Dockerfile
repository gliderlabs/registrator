FROM golang:1.16.3-alpine3.13 AS builder
WORKDIR /go/src/github.com/gliderlabs/registrator/

COPY . .

RUN \
	apk add --no-cache curl git \
	&& CGO_ENABLED=0 GOOS=linux go build \
		-a -installsuffix cgo \
		-ldflags "-X main.Version=$(cat VERSION)" \
		-o bin/registrator \
		.

FROM alpine:3.13

COPY docker-entrypoint.sh /bin/docker-entrypoint.sh

RUN \ 
	chmod +x /bin/docker-entrypoint.sh
ENV STOP_TIMEOUT 1200000
STOPSIGNAL SIGQUIT

RUN apk add --no-cache ca-certificates
COPY --from=builder /go/src/github.com/gliderlabs/registrator/bin/registrator /bin/registrator

ENTRYPOINT ["/bin/docker-entrypoint.sh"]
