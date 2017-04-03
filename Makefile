ORGANIZATION ?= 'pipedrive/'
NAME=registrator
VERSION=$(shell cat VERSION)
DOCKER_ORG = $(ORGANIZATION)$(NAME)
DOCKER_TAG = $(DOCKER_ORG):$(VERSION)
DEV_RUN_OPTS ?= consul:

dev-local:
	docker-compose down
	go build -ldflags "-extldflags '-static'" -o registrator
	docker-compose up --build -d

dev:
	docker build -f Dockerfile.dev -t $(NAME):dev .
	docker run --rm \
		-v /var/run/docker.sock:/tmp/docker.sock \
		$(NAME):dev /bin/registrator $(DEV_RUN_OPTS)

build-scratch:
	mkdir -p build
	docker build -t $(DOCKER_TAG)_interim .
	docker run --rm -v $(PWD):/opt --entrypoint=cp $(DOCKER_TAG)_interim /bin/registrator /opt
	docker build -f Dockerfile.release -t $(DOCKER_TAG) .
	docker rmi $(DOCKER_TAG)_interim

tag-beta:
	docker tag $(DOCKER_TAG) $(DOCKER_ORG):beta

push-beta:
	docker push $(DOCKER_ORG):beta

push-release:
	docker push $(DOCKER_TAG)

make-beta: build-scratch tag-beta push-beta
make-release: build-scratch push-release

build:
	mkdir -p build
	docker build -t $(NAME):$(VERSION) .
	docker save $(NAME):$(VERSION) | gzip -9 > build/$(NAME)_$(VERSION).tgz

release:
	rm -rf release && mkdir release
	go get github.com/progrium/gh-release/...
	cp build/* release
	gh-release create gliderlabs/$(NAME) $(VERSION) \
		$(shell git rev-parse --abbrev-ref HEAD) $(VERSION)

docs:
	boot2docker ssh "sync; sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'" || true
	docker run --rm -it -p 8000:8000 -v $(PWD):/work gliderlabs/pagebuilder mkdocs serve

circleci:
	rm ~/.gitconfig
ifneq ($(CIRCLE_BRANCH), release)
	echo build-$$CIRCLE_BUILD_NUM > VERSION
endif

.PHONY: build release docs
