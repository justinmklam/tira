# Makefile for tira

GOLANGCI_LINT_VERSION := $(strip $(shell cat .golangci-version))

.PHONY: build install clean test fmt lint lint-install vet check update

# Build the binary
build:
	go build -o tira ./cmd/tira

run:
	./tira board

run-dev:
	./tira --profile dev board --debug

# Install the binary to $GOPATH/bin (or ~/go/bin)
install:
	go install ./cmd/tira

# Update all Go dependencies to their latest versions and tidy go.mod/go.sum
update:
	go get -u ./...
	go mod tidy

# Remove build artifacts
clean:
	rm -f tira

# Run all tests
test:
	go test ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Run tests with coverage report
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@rm -f coverage.out

# Format code (check only — fails if not formatted)
fmt-check:
	@test -z "$$(gofmt -l cmd/ internal/)" || (echo "Files not formatted:"; gofmt -l cmd/ internal/; exit 1)

# Format code in-place
fmt:
	gofmt -w cmd/ internal/

# Run go vet
vet:
	go vet ./...

# Run govulncheck
vuln-check:
	govulncheck ./...

# Install the repository-pinned golangci-lint version
lint-install:
	cd "$$(go env GOPATH)" && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)

# Run the repository-pinned golangci-lint version
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint is not installed; run make lint-install"; exit 1; }
	@actual="$$(golangci-lint version --short)"; \
	if [ "$$actual" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) required; found $$actual; run make lint-install"; \
		exit 1; \
	fi
	golangci-lint run ./...

# Run all local checks (fmt, vet, lint, test, vulnerability scan)
check: fmt-check vet lint test vuln-check
