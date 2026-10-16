.PHONY: test test-unit test-e2e e2e test-go test-file e2e-file test-run e2e-run build clean tidy help \
	check-toolchain fmt fmt-check vet lint tidy-check test-race race-test vuln secrets \
	replace-check security check check-all release-check release tools \
	surface-snapshot surface-check lint-actions mcpb

BINARY_NAME := mailpiece
BUILD_DIR := ./bin
BINARY := $(abspath $(BUILD_DIR))/$(BINARY_NAME)
MAILPIECE_TEST_BINARY ?= $(BINARY)

# Load local test credentials if present, but refuse tracked local secret files.
ifneq ($(shell git ls-files --error-unmatch .env.test >/dev/null 2>&1 && echo tracked),)
$(error .env.test is tracked by Git. Remove it from version control and keep local secret files untracked)
endif
-include .env.test
export MAILPIECE_TEST_TOKEN MAILPIECE_TEST_ACCOUNT MAILPIECE_TEST_WRITE_ACCOUNT MAILPIECE_TEST_API_URL MAILPIECE_TEST_BINARY
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

DIST := $(CURDIR)/dist
MCPB_BUILD := CGO_ENABLED=0 go build -ldflags "-s -w $(LDFLAGS)"
# The extension's version must be semver, and Claude Desktop installs a bundle only when it is
# newer than the one it has. A release tag gives its number; any other build gives a 0.0.0
# prerelease stamped with the build's time and commit, so each dev build is newer than the last.
MCPB_VERSION := $(shell echo '$(VERSION)' | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$$' && echo '$(VERSION)' | sed 's/^v//' || echo "0.0.0-dev.$(shell echo $(DATE) | tr -d 'TZ:-').g$(COMMIT)")
MAKEFAT := github.com/randall77/makefat@v0.0.0-20260406194835-1b91746796b7

# Test configuration (set these or export as environment variables)
# export MAILPIECE_TEST_TOKEN=your-token
# export MAILPIECE_TEST_ACCOUNT=your-account

# Default target: local CI gate
.DEFAULT_GOAL := check

help:
	@echo "Mailpiece CLI"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build the CLI"
	@echo "  make test           Run unit tests (no API required)"
	@echo "  make test-unit      Alias for test"
	@echo "  make e2e            Run owner-only CLI contract e2e tests"
	@echo "  make test-e2e       Alias for e2e"
	@echo "  make e2e-file       Run a specific CLI contract e2e test file"
	@echo "  make test-file      Alias for e2e-file"
	@echo "  make e2e-run        Run a specific CLI contract e2e test by name"
	@echo "  make test-run       Alias for e2e-run"
	@echo "  make mcpb           Build the desktop extension (dist/mailpiece.mcpb)"
	@echo "  make clean          Remove build artifacts"
	@echo "  make tidy           Tidy dependencies"
	@echo ""
	@echo "  make fmt            Format Go source files"
	@echo "  make fmt-check      Check formatting (CI gate)"
	@echo "  make vet            Run go vet"
	@echo "  make lint           Run golangci-lint"
	@echo "  make tidy-check     Verify go.mod/go.sum tidiness"
	@echo "  make test-race      Run unit tests with race detector (race-test is an alias)"
	@echo "  make vuln           Run govulncheck"
	@echo "  make secrets        Run gitleaks secret scan"
	@echo "  make replace-check  Guard against replace directives in go.mod"
	@echo ""
	@echo "  make surface-snapshot  Regenerate SURFACE.txt"
	@echo "  make surface-check     Check SURFACE.txt is up to date"
	@echo ""
	@echo "  make lint-actions   Lint GitHub Actions workflows"
	@echo "  make security       lint + vuln + secrets"
	@echo "  make check          fmt-check + vet + lint + lint-actions + tidy-check + test-race (local CI gate)"
	@echo "  make check-all      check + e2e"
	@echo "  make release-check  check + replace-check + vuln + secrets"
	@echo "  make release        Run release preflight and tag (VERSION=0.2.0)"
	@echo "  make tools          Install dev tools"
	@echo ""
	@echo "Environment variables (required for e2e tests):"
	@echo "  MAILPIECE_TEST_TOKEN          API token"
	@echo "  MAILPIECE_TEST_ACCOUNT        Account slug read by the tests"
	@echo "  MAILPIECE_TEST_WRITE_ACCOUNT  Scratch account for tests that write (optional; never the demo)"
	@echo "  MAILPIECE_TEST_API_URL        API base URL (default: http://app.mailpiece.localhost:3020)"
	@echo "  MAILPIECE_TEST_BINARY         Prebuilt binary path (optional)"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make test"
	@echo "  export MAILPIECE_TEST_TOKEN=your-token"
	@echo "  export MAILPIECE_TEST_ACCOUNT=your-account"
	@echo "  make e2e"

# Toolchain guard: fails fast when PATH go and GOROOT go disagree
check-toolchain:
	@GOV=$$(go version | awk '{print $$3}'); \
	ROOT=$$(go env GOROOT); \
	ROOTV=$$($$ROOT/bin/go version | awk '{print $$3}'); \
	if [ "$$GOV" != "$$ROOTV" ]; then \
		echo "ERROR: Go toolchain mismatch"; \
		echo "  PATH go:   $$GOV ($$(which go))"; \
		echo "  GOROOT go: $$ROOTV ($$ROOT/bin/go)"; \
		echo "Fix: eval \"\$$(mise hook-env)\" && make <target>"; \
		exit 1; \
	fi

# Build CLI
build: check-toolchain
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/mailpiece

# Build the desktop extension: the manifest with this build's version, a universal macOS
# binary and a Windows one. lipo exists on macOS only; makefat does the same job elsewhere.
mcpb: check-toolchain
	@rm -rf $(DIST)/extension $(DIST)/mailpiece.mcpb
	@mkdir -p $(DIST)/extension/server
	sed 's/"version": "[^"]*"/"version": "$(MCPB_VERSION)"/' extension/manifest.json > $(DIST)/extension/manifest.json
	GOOS=darwin GOARCH=arm64 $(MCPB_BUILD) -o $(DIST)/mailpiece-darwin-arm64 ./cmd/mailpiece
	GOOS=darwin GOARCH=amd64 $(MCPB_BUILD) -o $(DIST)/mailpiece-darwin-amd64 ./cmd/mailpiece
	@if command -v lipo >/dev/null 2>&1; then \
		lipo -create -output $(DIST)/extension/server/mailpiece $(DIST)/mailpiece-darwin-arm64 $(DIST)/mailpiece-darwin-amd64; \
	else \
		go run $(MAKEFAT) $(DIST)/extension/server/mailpiece $(DIST)/mailpiece-darwin-arm64 $(DIST)/mailpiece-darwin-amd64; \
	fi
	GOOS=windows GOARCH=amd64 $(MCPB_BUILD) -o $(DIST)/extension/server/mailpiece.exe ./cmd/mailpiece
	cd $(DIST)/extension && zip -q -r -X ../mailpiece.mcpb manifest.json server
	@echo "Built $(DIST)/mailpiece.mcpb, version $(MCPB_VERSION)"

# Run unit tests (no API required, no network)
test: check-toolchain
	MAILPIECE_NO_KEYRING=1 go test ./internal/... ./pkg/...

test-unit: test

# Run e2e tests (requires API credentials)
e2e: build
	@if [ -z "$$MAILPIECE_TEST_TOKEN" ]; then echo "Error: MAILPIECE_TEST_TOKEN not set"; exit 1; fi
	@if [ -z "$$MAILPIECE_TEST_ACCOUNT" ]; then echo "Error: MAILPIECE_TEST_ACCOUNT not set"; exit 1; fi
	go test -v -count=1 -timeout 10m ./e2e/cli_tests/...

test-e2e: e2e

test-go: e2e

# Run a single test file (e.g., make e2e-file FILE=output_contract)
e2e-file: build
	@if [ -z "$(FILE)" ]; then echo "Usage: make e2e-file FILE=output_contract"; exit 1; fi
	@if [ -z "$$MAILPIECE_TEST_TOKEN" ]; then echo "Error: MAILPIECE_TEST_TOKEN not set"; exit 1; fi
	@if [ -z "$$MAILPIECE_TEST_ACCOUNT" ]; then echo "Error: MAILPIECE_TEST_ACCOUNT not set"; exit 1; fi
	go test -v -count=1 ./e2e/cli_tests/main_test.go ./e2e/cli_tests/helpers_test.go ./e2e/cli_tests/$(FILE)_test.go

test-file: e2e-file

# Run a single test by name (e.g., make e2e-run NAME=TestOutputContractListCommands)
e2e-run: build
	@if [ -z "$(NAME)" ]; then echo "Usage: make e2e-run NAME=TestOutputContractListCommands"; exit 1; fi
	@if [ -z "$$MAILPIECE_TEST_TOKEN" ]; then echo "Error: MAILPIECE_TEST_TOKEN not set"; exit 1; fi
	@if [ -z "$$MAILPIECE_TEST_ACCOUNT" ]; then echo "Error: MAILPIECE_TEST_ACCOUNT not set"; exit 1; fi
	go test -v -count=1 -run $(NAME) ./e2e/cli_tests/...

test-run: e2e-run

# Format Go source
fmt:
	gofmt -s -w .

# Check formatting (CI gate)
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)

# Run go vet
vet: check-toolchain
	go vet ./...

# Run golangci-lint
lint:
	golangci-lint run ./...

# Verify go.mod/go.sum tidiness (non-mutating)
tidy-check: check-toolchain
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak
	@go mod tidy
	@if ! diff -q go.mod go.mod.bak >/dev/null 2>&1 || ! diff -q go.sum go.sum.bak >/dev/null 2>&1; then \
		mv go.mod.bak go.mod; mv go.sum.bak go.sum; \
		echo "go.mod or go.sum is not tidy: run 'go mod tidy'"; \
		exit 1; \
	fi
	@mv go.mod.bak go.mod && mv go.sum.bak go.sum

# Run unit tests with race detector
test-race: check-toolchain
	MAILPIECE_NO_KEYRING=1 go test -race -count=1 ./internal/... ./pkg/...

race-test: test-race

# Run govulncheck
vuln:
	govulncheck ./...

# Run gitleaks secret scan
secrets:
	@if ! command -v gitleaks >/dev/null 2>&1 || [ ! -f .gitleaks.toml ]; then \
		echo "Skipping gitleaks (binary not found or .gitleaks.toml absent)"; \
	else \
		gitleaks detect --source . --verbose; \
	fi

# Guard against replace directives in go.mod
replace-check:
	@if grep -q '^replace' go.mod; then \
		echo "ERROR: go.mod contains replace directives"; \
		grep '^replace' go.mod; \
		exit 1; \
	fi

# Security suite
security: lint vuln secrets

# Local CI gate: everything the test workflow checks, minus govulncheck.
# test-race runs the unit tests, so plain test is not repeated here.
check: fmt-check vet lint lint-actions tidy-check test-race

# Everything, including the e2e suite (needs API credentials)
check-all: check e2e

# Release preflight: check plus what only a release needs
release-check: check replace-check vuln secrets

# Release (delegates to script)
# VERSION defaults to git describe, which release.sh refuses, so only a VERSION given by hand is passed.
release:
	@scripts/release.sh $(if $(filter command line environment,$(origin VERSION)),$(VERSION))

# Lint GitHub Actions workflows (requires actionlint + zizmor, pinned in .mise.toml)
lint-actions:
	@command -v actionlint >/dev/null || (echo "Install actionlint: mise install (or: make tools)" && exit 1)
	@command -v zizmor >/dev/null || (echo "Install zizmor: mise install (or: make tools)" && exit 1)
	actionlint
	zizmor .

# Install dev tools
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1
	go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "For gitleaks, install via: brew install gitleaks (or see https://github.com/gitleaks/gitleaks)"
	@for tool in actionlint shellcheck zizmor jq; do \
		if ! command -v "$$tool" > /dev/null 2>&1; then \
			if command -v brew > /dev/null 2>&1; then \
				brew install "$$tool"; \
			elif command -v pacman > /dev/null 2>&1; then \
				sudo pacman -S --noconfirm "$$tool"; \
			else \
				echo "Error: install $$tool manually" >&2; \
				exit 1; \
			fi; \
		fi; \
	done

# Regenerate SURFACE.txt
surface-snapshot:
	MAILPIECE_NO_KEYRING=1 GENERATE_SURFACE=1 go test ./internal/commands/ -run TestGenerateSurfaceSnapshot -v

# CI check: SURFACE.txt is up to date
surface-check:
	MAILPIECE_NO_KEYRING=1 go test ./internal/commands/ -run TestSurfaceSnapshot -v

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

# Tidy dependencies
tidy:
	go mod tidy
