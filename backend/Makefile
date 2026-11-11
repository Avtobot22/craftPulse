COMMIT_SHA ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
LDFLAGS    := -s -w -X github.com/Avtobot22/craftPulse/backend/pkg/health._commitSHA=$(COMMIT_SHA)
IMAGE      ?= craftpulse-server:$(COMMIT_SHA)

.PHONY: help generate build test vet image

## help: list targets
help:
	@grep -E '^## [a-z-]+:' $(MAKEFILE_LIST) | sed -e 's/## //'

## generate: generate the contract code from openapi.yaml
generate:
	go generate ./...

## build: build api and worker into bin/
build: generate
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/ ./cmd/api ./cmd/worker

## vet: run go vet
vet: generate
	go vet ./...

## test: run tests with the race detector
test: generate
	go test -race ./...

## image: build the craftpulse-server image
image:
	docker build --build-arg COMMIT_SHA=$(COMMIT_SHA) -t $(IMAGE) .
