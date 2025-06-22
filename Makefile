.PHONY: help run-client run-server test coverage lint mocks clean

# Default target
help:
	@echo "Available targets:"
	@echo "  run-client    - Run the client application"
	@echo "  run-server    - Run the server application"
	@echo "  test          - Run all tests with race detection"
	@echo "  coverage      - Generate and view test coverage report"
	@echo "  lint          - Run golangci-lint"
	@echo "  mocks         - Generate mock files using mockery"
	@echo "  clean         - Clean build artifacts"

# Run the client
run-client:
	go run ./cmd/client -output=transcript.txt

# Run the server
run-server:
	go run ./cmd/server

# Run tests with verbose output and race detection
test:
	go test -v -race ./...

# Generate test coverage report and open in browser
coverage:
	go test -cover -covermode=count -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Run golangci-lint
lint:
	gofmt -l -s -w . && go vet -all ./...

# Generate mocks using mockery
mocks:
	go tool -modfile=go.tools.mod mockery

# Clean build artifacts
clean:
	go clean -testcache ./...
	rm -f coverage.out
	rm -f transcript.txt
