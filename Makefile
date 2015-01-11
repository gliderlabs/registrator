NAME=registrator

## the go-get'able path
PKG_PATH=github.com/progrium/$(NAME)

## version, taken from Git tag (like v1.0.0) or hash
VERSION:=$(shell git describe --always --dirty | sed -e 's/^v//g' )

## all non-test source files
SOURCES:=$(shell go list -f '{{range .GoFiles}}{{ $$.Dir }}/{{.}} {{end}}' ./... | sed -e "s@$(PWD)/@@g" )

## all packages in this prject
PACKAGES:=$(shell go list -f '{{.Name}}' ./... )

OS:=$(shell uname -s)
HARDWARE:=$(shell uname -m)

export GOPATH:=$(PWD)/.build

GODEP=$(GOPATH)/bin/godep

.PHONY: all init deps update-deps build clean docker linux darwin release

all: build

$(GOPATH) stage:
	@mkdir -p $@

## retrieve godep tool
$(GODEP): | $(GOPATH)
	@echo "Installing godep"
	@go get github.com/tools/godep

## retrieve/restore dependencies with godep
$(GOPATH)/.deps_installed: $(GODEP) Godeps/Godeps.json | $(GOPATH)
	@echo "Retrieving dependencies"
	@$(GODEP) restore
	
#	ensure this project can be imported
	@mkdir -p $(shell dirname $(GOPATH)/src/$(PKG_PATH))
	@test -e $(GOPATH)/src/$(PKG_PATH) || ln -s $(PWD) $(GOPATH)/src/$(PKG_PATH)
	
	@touch $@

## just installs dependencies
deps: $(GOPATH)/.deps_installed

## update dependencies to their latest versions and save *all* dependencies
update-deps: $(GOPATH)/.deps_installed
	@cd $(GOPATH)/src/$(PKG_PATH) && ./update-deps.sh

## build the binary for local use
stage/$(NAME): $(GODEP) deps $(SOURCES) | stage
	$(GODEP) go build -o $@ -ldflags '-X main.version $(VERSION)' -v .

## shortcut
build: stage/$(NAME)

clean:
	rm -rf .build stage release

## build per-platform binaries for release
linux darwin: $(GODEP) deps $(SOURCES) | stage
	GOOS=$@ $(GODEP) go build -o release/$(NAME) -ldflags '-X main.version $(VERSION)' -v .
	cd release && tar -zcf $(NAME)_$(VERSION)_$(@)_$(HARDWARE).tgz $(NAME)
	rm -f release/$(NAME)

docker:
	docker build -t registrator .

release: linux darwin
	echo "$(VERSION)" > release/version
	echo "progrium/$(NAME)" > release/repo
	gh-release # https://github.com/progrium/gh-release
