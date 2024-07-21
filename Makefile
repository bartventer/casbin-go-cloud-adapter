.SHELLFLAGS = -ecuo pipefail
SHELL = /bin/bash

# Variables
COVEPROFILE ?= cover.out
RELEASE_DRYRUN ?=

# Commands
GO := go
GOLINT := golangci-lint
GOTEST := go test

# Flags
GOLINTFLAGS := run --verbose --fast --fix
GOTESTFLAGS := -v -coverprofile=$(COVERPROFILE)
RELEASEFLAGS :=
ifneq ($(RELEASE_DRYRUN),)
	RELEASEFLAGS += --dry-run
endif

.PHONY: lint
lint:
	$(GOLINT) $(GOLINTFLAGS) ./...

.PHONY: build
build:
	$(GO) build -v -o ./tmp/main .

.PHONY: test
test:
	$(GOTEST) $(GOTESTFLAGS) ./...

.PHONY: update
update:
	$(GO) mod tidy
	$(GO) get -u ./...

.PHONY: release
release:
	yarn install
	yarn run semantic-release $(RELEASEFLAGS)