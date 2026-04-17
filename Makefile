.PHONY: all build clean test test-unit test-integration test-roundtrip install lint lint-install lint-go-install lint-sh-install lint-go lint-sh chlog-install chlog-new chlog-validate chlog-preview chlog-update

all: lint test

BUILD_DIR=./build
BINARY_NAME=dash0
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Tools
TOOLS_BIN_DIR?=$(shell pwd)/.tools
GOLANGCI_LINT_VERSION=v1.64.8
GOLANGCI_LINT=$(TOOLS_BIN_DIR)/golangci-lint
CHLOGGEN_VERSION=v0.23.1
CHLOGGEN=$(TOOLS_BIN_DIR)/chloggen

build:
	(mkdir -p $(BUILD_DIR) || true) && go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dash0

test: test-unit test-integration test-roundtrip

test-unit:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

test-roundtrip: build
	bash test/roundtrip/run_all.sh

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

lint: lint-go lint-sh

lint-install: lint-go-install lint-sh-install

lint-go-install: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	@mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint-sh-install:
	@command -v shellcheck >/dev/null 2>&1 || { echo "Installing shellcheck..."; brew install shellcheck 2>/dev/null || sudo apt-get install -y shellcheck; }

lint-go: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

lint-sh:
	shellcheck -x $(shell find . -name '*.sh' -not -path './.claude/*' -not -path './.git/*')

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
