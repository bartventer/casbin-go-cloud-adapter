.SHELLFLAGS = -ecuo pipefail
SHELL = /bin/bash

# Variables
COVEPROFILE ?= cover.out

# Commands
GO := go
GOLINT := golangci-lint
GOTEST := go test

# Flags
GOLINTFLAGS := run --verbose --fast --fix
GOTESTFLAGS := -v -coverprofile=$(COVERPROFILE)

.PHONY: lint
lint:
	$(GOLINT) $(GOLINTFLAGS) ./...

.PHONY: build
build:
	$(GO) build -v -o ./tmp/main .

.PHONY: test
test:
	$(GOTEST) $(GOTESTFLAGS) ./...