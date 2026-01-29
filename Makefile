.PHONY: build clean test test-unit test-integration install

BUILD_DIR=./build
BINARY_NAME=dash0
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

build:
	(mkdir -p $(BUILD_DIR) || true) && go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dash0

test: test-unit test-integration

test-unit:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
