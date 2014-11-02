NAME=registrator
HARDWARE=$(shell uname -m)
VERSION=$(shell cat VERSION)

build:
	docker build -t registrator .

release:
	rm -rf release
	mkdir release
	GOOS=linux godep go build -ldflags "-X main.Version $(VERSION)" -o release/$(NAME)
	cd release && tar -zcf $(NAME)_$(VERSION)_linux_$(HARDWARE).tgz $(NAME)
	GOOS=darwin godep go build -ldflags "-X main.Version $(VERSION)" -o release/$(NAME)
	cd release && tar -zcf $(NAME)_$(VERSION)_darwin_$(HARDWARE).tgz $(NAME)
	rm release/$(NAME)
	echo "$(VERSION)" > release/version
	echo "progrium/$(NAME)" > release/repo
	gh-release # https://github.com/progrium/gh-release

.PHONY: release