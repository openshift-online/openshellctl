SHELL := /usr/bin/env bash

# Build metadata injected via -ldflags.
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT        ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
OPENSHELL_PIN := $(shell awk '{print $$1}' hack/openshell-pin)

MODULE   := github.com/openshift-online/openshellctl
LDFLAGS  := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.openshellPin=$(OPENSHELL_PIN)
BIN_DIR  := bin
BINARY   := $(BIN_DIR)/openshellctl

IMAGE       ?= openshellctl
IMAGE_TAG   ?= $(VERSION)
CONTAINER_ENGINE ?= podman

GO ?= go

.PHONY: all
all: verify test lint build

.PHONY: build
build:
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/openshellctl

.PHONY: test
test:
	$(GO) test -race -count=1 ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: generate
generate:
	$(GO) generate ./...

.PHONY: verify
verify: verify-pin verify-tidy verify-generate

.PHONY: verify-pin
verify-pin:
	./hack/verify-pin.sh

.PHONY: verify-tidy
verify-tidy:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum

.PHONY: verify-generate
verify-generate: generate
	git diff --exit-code

.PHONY: image
image:
	$(CONTAINER_ENGINE) build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg OPENSHELL_PIN=$(OPENSHELL_PIN) \
		-f build/Dockerfile -t $(IMAGE):$(IMAGE_TAG) .

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
