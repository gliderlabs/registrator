NAME=registrator
HARDWARE=$(shell uname -m)
VERSION=0.2.0

build:
	mkdir -p stage
	go build -o stage/dockser
	docker build -t registrator .

release:
	rm -rf release
	mkdir release
	GOOS=linux go build -o release/$(NAME)
	cd release && tar -zcf $(NAME)_$(VERSION)_linux_$(HARDWARE).tgz $(NAME)
	GOOS=darwin go build -o release/$(NAME)
	cd release && tar -zcf $(NAME)_$(VERSION)_darwin_$(HARDWARE).tgz $(NAME)
	rm release/$(NAME)
	echo "$(VERSION)" > release/version
	echo "progrium/$(NAME)" > release/repo
	gh-release

.PHONY: release