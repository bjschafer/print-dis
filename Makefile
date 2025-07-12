.DEFAULT_GOAL := build

VERSION 	?= $(shell git describe --tags --always --dirty)
BUILD_FLAGS ?= -v
LDFLAGS     ?= -extldflags=-Wl,-ld_classic
ARCH        ?= $(shell go env GOARCH)
GOARCH      ?= $(ARCH)
OS          ?= $(shell go env GOOS)
GOOS        ?= $(OS)
PACKAGE_DIR ?= packages

# Track all Go source files
GO_SOURCES := $(shell find . -name "*.go")

.PHONY: check
check: test lint

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test -race -cover -ldflags=$(LDFLAGS) ./...

.PHONY: generate
generate:
	GOARCH=$(ARCH) GOOS=$(OS) go generate -v ./...

.PHONY: build
build: bin/print-dis bin/migrate

.PHONY: clean
clean:
	rm -rf ./bin
	rm -rf $(PACKAGE_DIR)

bin/print-dis: $(GO_SOURCES) generate
	GOARCH=$(GOARCH) GOOS=$(GOOS) go build -o bin/print-dis $(BUILD_FLAGS) ./main.go

bin/migrate: $(GO_SOURCES) generate
	GOARCH=$(GOARCH) GOOS=$(GOOS) go build -o bin/migrate $(BUILD_FLAGS) ./cmd/migrate/main.go

.PHONY: package
package:
	$(MAKE) build GOOS=linux
	mkdir -p $(PACKAGE_DIR)
	VERSION=$(VERSION) nfpm package \
		--config nfpm.yaml \
		--target $(PACKAGE_DIR)/print-dis_$(VERSION)_$(ARCH).deb

.PHONY: package-all
package-all: package-amd64 package-arm64

.PHONY: package-amd64
package-amd64:
	$(MAKE) package ARCH=amd64 GOARCH=amd64

.PHONY: package-arm64
package-arm64:
	$(MAKE) package ARCH=arm64 GOARCH=arm64
