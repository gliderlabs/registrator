FROM golang:1.19.1-alpine3.16 AS builder

WORKDIR /go/src/github.com/gliderlabs/registrator/

COPY . .
RUN apk --no-cache add -t build-deps build-base git curl ca-certificates
RUN CGO_ENABLED=0 GOOS=linux \
	go mod init && \
	go mod tidy && \
	go mod vendor && \
	go build -a -installsuffix cgo -ldflags "-X main.Version=$(cat VERSION)" -o bin/registrator .

FROM alpine:3.16

RUN apk add --no-cache ca-certificates
COPY --from=builder /go/src/github.com/gliderlabs/registrator/bin/registrator /bin/registrator

ENTRYPOINT ["/bin/registrator"]
