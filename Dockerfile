# FROM alpine:3.5
# ENTRYPOINT ["/bin/registrator"]
#
# RUN apk --no-cache add ca-certificates
#
# COPY ./build/registrator /bin/registrator


FROM golang:1.8.3 as builder
WORKDIR /go/src/github.com/gliderlabs/registrator/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
			-ldflags "-X main.Version=$(cat VERSION)" -o bin/registrator .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=0 /go/src/github.com/gliderlabs/registrator/bin/registrator /bin/registrator
ENTRYPOINT ["/bin/registrator"]
