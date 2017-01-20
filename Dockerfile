FROM gliderlabs/alpine:3.3
ENTRYPOINT ["/bin/registrator"]

ARG RUN_TESTS=false

RUN apk-install -t build-deps build-base go git mercurial

COPY . /go/src/github.com/gliderlabs/registrator
WORKDIR /go/src/github.com/gliderlabs/registrator

ENV GOPATH=/go

RUN go get -t
RUN go build -ldflags "-X main.Version=$(cat VERSION)" -o /bin/registrator
RUN [ "$RUN_TESTS" = "true" ]; echo "Executing tests\n" && go test ./...
RUN rm -rf /go
RUN apk del --purge build-deps
