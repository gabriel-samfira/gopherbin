SHELL := bash

.PHONY : help build-image container-start all
.DEFAULT_GOAL := help

IMAGE_NAME = gopherbin
CONTAINER_NAME = gopher
CONFIG_FILE = $(PWD)/examples/gopherbin-config.toml

help :
	@echo "Variables:"
	@echo "	IMAGE_NAME			-> docker image name. Default = gopherbin"
	@echo "	CONTAINER_NAME			-> container name. Default = gopher"
	@echo "	CONFIG_FILE			-> your gopherbin config file(see examples directory) which will be mounted inside the container"
	@echo
	@echo "Usage:"
	@echo "	make build-image		-> create a docker image with gopher binary"
	@echo "	make container-start		-> start ghoperbin container"
	@echo "	make all			-> build go binary and start a container with it"


build-image:
	docker build --no-cache --tag $(IMAGE_NAME) .
	docker image prune -f --filter label=stage=builder

container-start :
	docker run --rm -p 9997:9997 --name $(CONTAINER_NAME) -v $(CONFIG_FILE):/etc/gopherbin-config.toml -d $(IMAGE_NAME)

all: build-image container-start

asd:
	docker run --rm -p 9997:9997 -v $(PWD)/examples/gopherbin-config.toml:/etc/gopherbin-config.toml gopherbin
