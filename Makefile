# Makefile for tira

.PHONY: build clean test fmt lint vet check

# Build the binary
build:
	GOTOOLCHAIN=local go build -o tira ./cmd/tira

run:
	./tira backlog

run-dev:
	./tira --profile dev backlog --debug

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
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)

# Format code in-place
fmt:
	gofmt -w .

# Run go vet
vet:
	go vet ./...

# Run golangci-lint (install: https://golangci-lint.run/welcome/install/)
lint:
	golangci-lint run ./...

# Run all checks (fmt, vet, lint, test) — same as CI
check: fmt-check vet lint test
