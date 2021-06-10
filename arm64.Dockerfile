FROM golang:1.9.4 AS builder
WORKDIR /go/src/github.com/gliderlabs/registrator/
COPY . .
RUN \
        curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh \
        && dep ensure -vendor-only \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-a -installsuffix cgo \
		-ldflags "-X main.Version=$(cat VERSION)" \
		-o bin/registrator \
		.

FROM arm64v8/ubuntu
COPY --from=builder /go/src/github.com/gliderlabs/registrator/bin/registrator /bin/registrator

ENTRYPOINT ["/bin/registrator"]
