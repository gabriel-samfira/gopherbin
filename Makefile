SHELL := bash

.PHONY : help fmt build-image container-start app
.DEFAULT_GOAL := help

IMAGE_NAME = gopherbin
CONTAINER_NAME = gopher
CONFIG_FILE = $(PWD)/testdata/config.toml
GOPATH ?= $(shell go env GOPATH)

help :
	@echo "Variables:"
	@echo "	IMAGE_NAME			-> docker image name. Default = gopherbin"
	@echo "	CONTAINER_NAME			-> container name. Default = gopher"
	@echo "	CONFIG_FILE			-> your gopherbin config file(see testdata directory) which will be mounted inside the container"
	@echo
	@echo "Usage:"
	@echo "	make fmt			-> running gofmt with options -s(simplify code) and -l (list files)"
	@echo "	make submodules			-> initialize the web UI submodule"
	@echo "	make noUI			-> build gopherbin without the web UI"
	@echo "	make withUI			-> build gopherbin with the web UI (requires nodejs and yarn to be installed)"
	@echo "	make all-noui			-> shorthand for make fmt submodules noUI"
	@echo "	make all-ui			-> shorthand for make fmt submodules withUI"
	@echo "	make build-image		-> create a docker image with gopher binary"
	@echo "	make container-start		-> start ghoperbin container"
	@echo "	make app			-> build go binary and start a container with it"


fmt:
	gofmt -s -l .

submodules:
	git submodule update --init --recursive

noUI:
	go build -o $(GOPATH)/bin/gopherbin cmd/gopherbin/gopherbin.go

withUI:
	cd webui/web && npm install && yarn build
	go build -o $(GOPATH)/bin/gopherbin -ldflags "-X 'gopherbin/webui.BuildTime=$(shell date +%s)'" -tags webui cmd/gopherbin/gopherbin.go

all-noui: fmt submodules noUI

all-ui: fmt submodules withUI

build-image:
	docker build --no-cache --tag $(IMAGE_NAME) .
	docker image prune -f --filter label=stage=builder

container-start :
	docker run --rm -p 9997:9997 --name $(CONTAINER_NAME) -v $(CONFIG_FILE):/etc/gopherbin-config.toml -d $(IMAGE_NAME)

app: build-image container-start
