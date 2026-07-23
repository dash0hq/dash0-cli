.PHONY: all build clean test test-unit test-integration test-roundtrip install lint lint-install lint-go-install lint-sh-install lint-go lint-sh chlog-install chlog-new chlog-validate chlog-preview chlog-update update-vendor-hash skill-bundle skill-validate

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

# Recompute the Nix buildGoModule vendorHash after a go.mod/go.sum change.
# Requires Nix with flakes enabled.
update-vendor-hash:
	./nix/update-vendor-hash.sh

lint: lint-go lint-sh skill-validate

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

# Regenerate internal/skill/content/references/*.md from docs/commands.md,
# and publish identical copies at the repo root under .claude/skills/dash0-cli
# and .agents/skills/dash0-cli so `npx skills add dash0hq/dash0-cli` and
# `gh skill install dash0hq/dash0-cli` can discover the skill directly from
# this repository (see docs/agent-skill-maintenance.md).
skill-bundle:
	go run ./internal/skill/gen
	@rm -rf .claude/skills/dash0-cli .agents/skills/dash0-cli
	@mkdir -p .claude/skills/dash0-cli .agents/skills/dash0-cli
	cp internal/skill/content/SKILL.md .claude/skills/dash0-cli/SKILL.md
	cp -r internal/skill/content/references .claude/skills/dash0-cli/references
	cp internal/skill/content/SKILL.md .agents/skills/dash0-cli/SKILL.md
	cp -r internal/skill/content/references .agents/skills/dash0-cli/references

# Fail if docs/commands.md changed without regenerating the skill bundle, or
# if the root-level publish copies drifted from internal/skill/content.
skill-validate:
	@rm -rf /tmp/dash0-cli-skill-check
	@go run ./internal/skill/gen -out /tmp/dash0-cli-skill-check
	@diff -r internal/skill/content/references /tmp/dash0-cli-skill-check/references || (echo "skill reference content is stale — run 'make skill-bundle'" && exit 1)
	@diff -q internal/skill/content/SKILL.md .claude/skills/dash0-cli/SKILL.md >/dev/null || (echo ".claude/skills/dash0-cli/SKILL.md is stale — run 'make skill-bundle'" && exit 1)
	@diff -r internal/skill/content/references .claude/skills/dash0-cli/references || (echo ".claude/skills/dash0-cli/references is stale — run 'make skill-bundle'" && exit 1)
	@diff -q internal/skill/content/SKILL.md .agents/skills/dash0-cli/SKILL.md >/dev/null || (echo ".agents/skills/dash0-cli/SKILL.md is stale — run 'make skill-bundle'" && exit 1)
	@diff -r internal/skill/content/references .agents/skills/dash0-cli/references || (echo ".agents/skills/dash0-cli/references is stale — run 'make skill-bundle'" && exit 1)
	@rm -rf /tmp/dash0-cli-skill-check
