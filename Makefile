.PHONY: build clean test install

BINARY_NAME=dash0
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

build:
	go build -o $(BINARY_NAME) ./cmd/dash0

test:
	go test -v ./...

install: build
	mv $(BINARY_NAME) $(GOPATH)/bin/
