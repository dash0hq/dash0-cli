.PHONY: build clean test test-unit test-integration install chlog-install chlog-new chlog-validate chlog-preview chlog-update

BUILD_DIR=./build
BINARY_NAME=dash0
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Tools
TOOLS_BIN_DIR?=$(shell pwd)/.tools
CHLOGGEN_VERSION=v0.23.1
CHLOGGEN=$(TOOLS_BIN_DIR)/chloggen

build:
	(mkdir -p $(BUILD_DIR) || true) && go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dash0

test: test-unit test-integration

test-unit:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

# Changelog management
$(CHLOGGEN):
	@mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install go.opentelemetry.io/build-tools/chloggen@$(CHLOGGEN_VERSION)

chlog-install: $(CHLOGGEN)

chlog-new: $(CHLOGGEN)
	$(CHLOGGEN) new --config .chloggen/config.yaml --filename $(shell git branch --show-current)

chlog-validate: $(CHLOGGEN)
	$(CHLOGGEN) validate --config .chloggen/config.yaml

chlog-preview: $(CHLOGGEN)
	$(CHLOGGEN) update --config .chloggen/config.yaml --dry

chlog-update: $(CHLOGGEN)
	$(CHLOGGEN) update --config .chloggen/config.yaml --version $(VERSION)
