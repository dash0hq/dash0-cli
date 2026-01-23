.PHONY: build clean test install

BUILD_DIR=./build
BINARY_NAME=dash0ctl
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

build:
	mkdir -p $(BUILD_DIR) && go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dash0

test:
	go test -v ./...

install: build
	mv $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
