.PHONY: help build vet test test-verbose test-coverage coverage-html coverage-func lint govulncheck \
        go helm helm-lint helm-template helm-schema helm-docs helm-docs-check clean

# ── Build variables ────────────────────────────────────────────────────────
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

# ── Help ───────────────────────────────────────────────────────────────────
help:
	@echo "Setup:"
	@echo "  mise install           - Install all dev tools (go, helm, golangci-lint, helm-docs)"
	@echo ""
	@echo "Shortcuts:"
	@echo "  make go                - Run all Go checks (build, vet, test, lint)"
	@echo "  make helm              - Run all Helm checks (lint, template, schema, docs)"
	@echo ""
	@echo "Go targets:"
	@echo "  make build             - Build the binary with version info"
	@echo "  make vet               - Run go vet"
	@echo "  make test              - Run all tests"
	@echo "  make test-verbose      - Run tests with -race"
	@echo "  make test-coverage     - Run tests and show coverage summary"
	@echo "  make coverage-html     - Open HTML coverage report in browser"
	@echo "  make coverage-func     - Show per-function coverage"
	@echo "  make lint              - Run golangci-lint"
	@echo "  make govulncheck       - Check for known Go vulnerabilities"
	@echo ""
	@echo "Helm targets:"
	@echo "  make helm-lint         - helm lint chart/ --strict"
	@echo "  make helm-template     - Validate template rendering"
	@echo "  make helm-schema       - Verify schema rejects invalid values"
	@echo "  make helm-docs         - Regenerate chart/README.md via helm-docs"
	@echo "  make helm-docs-check   - Fail if chart/README.md is out of date (CI use)"
	@echo ""
	@echo "Other:"
	@echo "  make build VERSION=v1.2.3 COMMIT=abc123"
	@echo "  make clean"

# ── Go checks ──────────────────────────────────────────────────────────────

# Run all Go checks — mirrors CI exactly
go: build vet test lint
	@echo ""
	@echo "✓ All Go checks passed!"

build:
	@echo "Building kube-binpacking-exporter $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o kube-binpacking-exporter .

vet:
	@echo "Running go vet..."
	go vet ./...

test:
	@echo "Running tests..."
	go test -v ./...

test-verbose:
	@echo "Running tests with race detector..."
	go test -v -race ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@echo "Coverage summary:"
	go tool cover -func=coverage.out | tail -1

coverage-html: test-coverage
	@echo "Generating HTML coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@which open > /dev/null && open coverage.html || \
	which xdg-open > /dev/null && xdg-open coverage.html || \
	echo "Please open coverage.html in your browser"

coverage-func: test-coverage
	@echo "Per-function coverage:"
	go tool cover -func=coverage.out

lint:
	@echo "Running golangci-lint..."
	golangci-lint run

govulncheck:
	@echo "Running govulncheck..."
	govulncheck ./...

# ── Helm checks ────────────────────────────────────────────────────────────

# Run all Helm checks — mirrors chart-ci.yaml exactly
helm: helm-lint helm-template helm-schema helm-docs-check
	@echo ""
	@echo "✓ All Helm checks passed!"

helm-lint:
	@echo "Linting Helm chart..."
	helm lint chart/ --strict
	@echo "✓ helm lint passed"

helm-template:
	@echo "Validating template rendering..."
	helm template kube-binpacking-exporter chart/ > /dev/null
	@echo "✓ Chart templates render successfully"

helm-schema:
	@echo "Verifying schema rejects invalid values..."
	@! helm template kube-binpacking-exporter chart/ --set logLevel=invalid-level > /dev/null 2>&1 \
		&& echo "✓ Schema correctly rejects invalid values" \
		|| (echo "✗ Schema should have rejected logLevel=invalid-level" && exit 1)

helm-docs:
	@echo "Regenerating chart/README.md via helm-docs..."
	helm-docs --chart-search-root chart/
	@echo "✓ chart/README.md updated"

helm-docs-check:
	@echo "Checking chart/README.md is up to date..."
	@$(MAKE) --no-print-directory helm-docs
	@if ! git diff --quiet chart/README.md; then \
		echo ""; \
		echo "✗ chart/README.md is out of date!"; \
		echo "  Run 'make helm-docs' and commit the result."; \
		echo ""; \
		git diff chart/README.md; \
		exit 1; \
	fi
	@echo "✓ chart/README.md is up to date"

# ── Clean ──────────────────────────────────────────────────────────────────
clean:
	@echo "Cleaning up..."
	rm -f kube-binpacking-exporter
	rm -f coverage.out coverage.html
