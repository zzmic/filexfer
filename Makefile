# Go parameters.
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names.
BINARY_DIR=bin
CLIENT_BINARY=$(BINARY_DIR)/client
SERVER_BINARY=$(BINARY_DIR)/server

# Source directories.
CLIENT_SOURCE=./cmd/client
SERVER_SOURCE=./cmd/server

.PHONY: all build client server clean deps tidy install uninstall run-client run-server test test-sh test-large-directory-sh test-directory-limit-sh help

# Default target.
all: build

# Build both client and server.
build: client server

# Build client.
client: $(CLIENT_BINARY)

$(CLIENT_BINARY): $(CLIENT_SOURCE)/*.go protocol/*.go
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(CLIENT_BINARY) $(CLIENT_SOURCE)

# Build server.
server: $(SERVER_BINARY)

$(SERVER_BINARY): $(SERVER_SOURCE)/*.go protocol/*.go
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(SERVER_BINARY) $(SERVER_SOURCE)

# Clean build artifacts.
clean:
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)

# Download dependencies.
deps:
	$(GOMOD) tidy
	$(GOMOD) download

# Tidy modules.
tidy:
	$(GOMOD) tidy

# Install binaries to `GOPATH/bin`.
install: build
	$(GOCMD) install $(CLIENT_SOURCE)
	$(GOCMD) install $(SERVER_SOURCE)

# Uninstall binaries from `GOPATH/bin`.
uninstall:
	rm $(GOPATH)/bin/client
	rm $(GOPATH)/bin/server

# Run client or server with optional arguments.
ARGS ?=

run-client: client
	./$(CLIENT_BINARY) $(ARGS)

run-server: server
	./$(SERVER_BINARY) $(ARGS)

# Run all tests.
test:
	chmod +x test.sh test_large_directory.sh test_directory_limit.sh
	./test.sh
	./test_large_directory.sh
	./test_directory_limit.sh

# Run `test.sh`.
test-sh:
	chmod +x test.sh
	./test.sh

# Run `test_large_directory.sh`.
test-large-directory-sh:
	chmod +x test_large_directory.sh
	./test_large_directory.sh

# Run `test_directory_limit.sh`.
test-directory-limit-sh:
	chmod +x test_directory_limit.sh
	./test_directory_limit.sh

# Help target.
help:
	@echo "Available targets:"
	@echo "  all        - Build both client and server (default)"
	@echo "  build      - Build both client and server"
	@echo "  client     - Build client binary"
	@echo "  server     - Build server binary"
	@echo "  clean      - Remove build artifacts"
	@echo "  deps       - Download dependencies"
	@echo "  tidy       - Tidy module dependencies"
	@echo "  install    - Install binaries to GOPATH/bin"
	@echo "  uninstall  - Uninstall binaries from GOPATH/bin"
	@echo "  run-client - Build and run client"
	@echo "  run-server - Build and run server"
	@echo "  test       - Run all tests"
	@echo "  test-sh    - Run test.sh"
	@echo "  test-large-directory-sh - Run test_large_directory.sh"
	@echo "  test-directory-limit-sh - Run test_directory_limit.sh"
	@echo "  help       - Show this help message"
