FROM alpine:3.7
CMD ["/bin/registrator"]

COPY . /go/src/github.com/gliderlabs/registrator
RUN apk --no-cache add -t build-deps build-base go git curl \
	&& apk --no-cache add ca-certificates \
	&& export GOPATH=/go && mkdir -p /go/bin && export PATH=$PATH:/go/bin \
	&& curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh \
	&& cd /go/src/github.com/gliderlabs/registrator \
	&& export GOPATH=/go \
	&& git config --global http.https://gopkg.in.followRedirects true \
	&& dep ensure \
	&& go build -ldflags "-X main.Version=dev" -o /bin/registrator
