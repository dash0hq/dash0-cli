.PHONY: all build clean test test-unit test-integration test-roundtrip test-e2e install lint lint-install lint-go-install lint-sh-install lint-go lint-sh chlog-install chlog-new chlog-validate chlog-preview chlog-update update-vendor-hash skill-bundle skill-validate

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

test: test-unit test-integration test-e2e test-roundtrip

test-unit:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

test-roundtrip: build
	bash test/roundtrip/run_all.sh

# End-to-end tests: the real dash0 binary + the real git binary inside a
# container, proving --since's git-shell-out path works across a real
# process boundary (in-process unit/integration tests can't). Gated behind
# Docker being available and kept separate from test-unit/test-integration
# given the added runtime cost and the Docker dependency.
#
# Colima users: testcontainers-go's Docker auto-detection doesn't recognize
# colima's non-standard socket forwarding. Export these first:
#   export DOCKER_HOST="unix://$$HOME/.colima/default/docker.sock"
#   export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE="/var/run/docker.sock"
test-e2e:
	@command -v docker >/dev/null 2>&1 || { echo "Error: docker is required for test-e2e" >&2; exit 1; }
	@docker version >/dev/null 2>&1 || { echo "Error: docker daemon is not reachable (is it running?)" >&2; exit 1; }
	go test -v -tags=e2e ./test/e2e/...

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

# Regenerate internal/skill/content/references/*.md from docs/commands.md.
# The two root-level publish paths — .claude/skills/dash0-cli and
# .agents/skills/dash0-cli, where `npx skills add dash0hq/dash0-cli` and
# `gh skill install dash0hq/dash0-cli` look for the skill — are checked-in
# symlinks pointing at internal/skill/content, so regenerating the canonical
# content is enough to update all three paths (see
# docs/agent-skill-maintenance.md).
SKILL_SYMLINK_TARGET := ../../internal/skill/content
skill-bundle:
	@rm -rf internal/skill/content/references
	go run ./internal/skill/gen
	@for link in .claude/skills/dash0-cli .agents/skills/dash0-cli; do \
		mkdir -p "$$(dirname "$$link")"; \
		if [ ! -L "$$link" ] || [ "$$(readlink "$$link")" != "$(SKILL_SYMLINK_TARGET)" ]; then \
			rm -rf "$$link"; \
			ln -s "$(SKILL_SYMLINK_TARGET)" "$$link"; \
		fi; \
	done

# Fail if docs/commands.md changed without regenerating the skill bundle, or
# if either publish symlink was replaced with a copy or points at the wrong
# target. Uses a per-invocation mktemp directory so shared /tmp on CI hosts
# stays safe from predictable-path races.
skill-validate:
	@set -e; \
		SKILL_TMP="$$(mktemp -d)"; \
		trap 'rm -rf "$$SKILL_TMP"' EXIT INT TERM; \
		go run ./internal/skill/gen -out "$$SKILL_TMP"; \
		diff -r internal/skill/content/references "$$SKILL_TMP/references" || { echo "skill reference content is stale — run 'make skill-bundle'"; exit 1; }; \
		for link in .claude/skills/dash0-cli .agents/skills/dash0-cli; do \
			if [ ! -L "$$link" ]; then \
				echo "$$link must be a symlink to $(SKILL_SYMLINK_TARGET) — run 'make skill-bundle'"; \
				exit 1; \
			fi; \
			target="$$(readlink "$$link")"; \
			if [ "$$target" != "$(SKILL_SYMLINK_TARGET)" ]; then \
				echo "$$link points at $$target, expected $(SKILL_SYMLINK_TARGET) — run 'make skill-bundle'"; \
				exit 1; \
			fi; \
		done
