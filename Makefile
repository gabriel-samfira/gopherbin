SHELL := bash

.PHONY: help fmt fmt-check tests build build-ui no-ui with-ui all-no-ui all build-image container-start app
.DEFAULT_GOAL := help

IMAGE_NAME    = gopherbin
CONTAINER_NAME = gopher
CONFIG_FILE   = $(PWD)/testdata/config.toml
GOPATH       ?= $(shell go env GOPATH)

help:
	@echo "Variables:"
	@echo "  IMAGE_NAME       -> docker image name (default: gopherbin)"
	@echo "  CONTAINER_NAME   -> container name (default: gopher)"
	@echo "  CONFIG_FILE      -> config file mounted into the container"
	@echo
	@echo "Usage:"
	@echo "  make fmt          -> run gofmt -s -l (excluding vendor)"
	@echo "  make fmt-check    -> fail if any non-vendor Go file is unformatted"
	@echo "  make tests        -> fmt-check + run all tests (requires SQLite FTS5)"
	@echo "  make build        -> build gopherbin with embedded web UI"
	@echo "  make no-ui        -> build gopherbin without web UI"
	@echo "  make with-ui      -> build gopherbin with embedded web UI (requires Node.js)"
	@echo "  make all-no-ui    -> fmt + no-ui"
	@echo "  make all          -> fmt + with-ui"
	@echo "  make build-image  -> build docker image"
	@echo "  make container-start -> start gopherbin container"
	@echo "  make app          -> build-image + container-start"

GO_FILES = $(shell find . -name "*.go" -not -path "./vendor/*")

fmt:
	gofmt -s -l $(GO_FILES)

fmt-check:
	@unformatted=$$(gofmt -s -l $(GO_FILES)); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

tests: fmt-check
	go test -mod vendor -tags fts5 ./...

build-ui:
	cd webui/svelte-app && npm install && npm run build

no-ui:
	go install -mod vendor -ldflags="-s -w" -tags fts5 ./cmd/...

with-ui: build-ui
	go install -mod vendor -ldflags="-s -w -X 'gopherbin/webui.BuildTime=$(shell date +%s)'" -tags fts5,webui ./cmd/...

build: with-ui

all-no-ui: fmt no-ui

all: fmt with-ui

build-image:
	docker build --no-cache --tag $(IMAGE_NAME) .
	docker image prune -f --filter label=stage=builder

container-start:
	docker run --rm -p 9997:9997 --name $(CONTAINER_NAME) -v $(CONFIG_FILE):/etc/gopherbin-config.toml -d $(IMAGE_NAME)

app: build-image container-start
