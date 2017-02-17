NAME=registrator
VERSION=$(shell cat VERSION)
DEV_RUN_OPTS ?= -ttl 60 -ttl-refresh 30 -require-label -ip 127.0.0.1  eureka://localhost:8090/eureka/v2
RELEASE_TAG=761584570493.dkr.ecr.us-east-1.amazonaws.com/registrator

dev:
	docker kill reg_eureka; echo Stopped.
	docker run --rm --name reg_eureka -e "SERVICE_REGISTER=true" -td -p 8090:8080 netflixoss/eureka:1.1.147
	docker build -f Dockerfile.dev -t $(NAME):dev .
	docker run --rm \
		--net=host \
		-v /var/run/docker.sock:/tmp/docker.sock \
		$(NAME):dev /bin/registrator $(DEV_RUN_OPTS)
	docker kill reg_eureka

build:
	mkdir -p build
	docker build -t $(NAME):$(VERSION) .
	docker save $(NAME):$(VERSION) | gzip -9 > build/$(NAME)_$(VERSION).tgz

release:
	docker build -t $(RELEASE_TAG) .
	docker push $(RELEASE_TAG)

