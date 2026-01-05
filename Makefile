GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod
GOFMT = $(GOCMD) fmt
GOVET = $(GOCMD) vet

LDFLAGS = -ldflags "-s -w" # For smaller binaries by stripping debug info.
RACE_FLAG = -race
# RACE_FLAG =

BINARY_DIR = bin
CLIENT_BINARY = $(BINARY_DIR)/client
SERVER_BINARY = $(BINARY_DIR)/server

CLIENT_SOURCE = ./cmd/client
SERVER_SOURCE = ./cmd/server

TEST_SCRIPTS = test.sh test_large_directory.sh test_directory_limit.sh

RESET = \033[0m
GREEN = \033[32m
YELLOW = \033[33m
CYAN = \033[36m

.PHONY: all build client server clean fmt vet clean deps tidy lint install uninstall \
	run-client run-server test cover test-sh test-large-directory-sh test-directory-limit-sh help

all: fmt vet build

build: client server

client: $(CLIENT_BINARY)

$(CLIENT_BINARY): $(CLIENT_SOURCE)/*.go protocol/*.go
	@echo "$(CYAN)Building client binary...$(RESET)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) $(RACE_FLAG) -o $(CLIENT_BINARY) $(CLIENT_SOURCE)
	@echo "$(GREEN)Client binary built at $(CLIENT_BINARY)$(RESET)"

server: $(SERVER_BINARY)

$(SERVER_BINARY): $(SERVER_SOURCE)/*.go protocol/*.go
	@echo "$(CYAN)Building server binary...$(RESET)"
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) $(RACE_FLAG) -o $(SERVER_BINARY) $(SERVER_SOURCE)
	@echo "$(GREEN)Server binary built at $(SERVER_BINARY)$(RESET)"

fmt:
	@echo "$(YELLOW)Formatting code...$(RESET)"
	$(GOFMT) ./...

vet:
	@echo "$(YELLOW)Vetting code...$(RESET)"
	$(GOVET) ./...

clean:
	@echo "$(YELLOW)Cleaning build artifacts...$(RESET)"
	$(GOCLEAN)
	rm -r $(BINARY_DIR)/*
	@echo "$(GREEN)Clean complete.$(RESET)"

deps:
	@echo "$(CYAN)Downloading dependencies...$(RESET)"
	$(GOMOD) tidy
	$(GOMOD) download

tidy:
	$(GOMOD) tidy

lint:
	golangci-lint run ./...

install: build
	@echo "$(CYAN)Installing binaries to GOPATH...$(RESET)"
	$(GOCMD) install $(CLIENT_SOURCE)
	$(GOCMD) install $(SERVER_SOURCE)
	@echo "$(GREEN)Installation complete.$(RESET)"

uninstall:
	@echo "$(YELLOW)Uninstalling binaries...$(RESET)"
	rm -f $(GOPATH)/bin/client
	rm -f $(GOPATH)/bin/server
	@echo "$(GREEN)Uninstalled.$(RESET)"

ARGS ?=

run-client: client
	./$(CLIENT_BINARY) $(ARGS)

run-server: server
	./$(SERVER_BINARY) $(ARGS)

test:
	$(GOCMD) test -v ./...

cover:
	$(GOCMD) test -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

test-sh:
	chmod +x test.sh
	./test.sh

test-large-directory-sh:
	chmod +x test_large_directory.sh
	./test_large_directory.sh

test-directory-limit-sh:
	chmod +x test_directory_limit.sh
	./test_directory_limit.sh

# Help target.
help:
	@echo 'Usage: make <target> [VAR=value]'
	@echo
	@echo 'Common variables:'
	@printf '  %-30s %s\n' 'ARGS' 'Arguments for run-client/run-server (e.g. ARGS="-port 9090")'
	@echo
	@echo 'Targets:'
	@printf '  %-30s %s\n' 'all' 'Format, vet, and build both client and server.'
	@printf '  %-30s %s\n' 'build' 'Build both client and server binaries.'
	@printf '  %-30s %s\n' 'client' 'Build the client binary.'
	@printf '  %-30s %s\n' 'server' 'Build the server binary.'
	@printf '  %-30s %s\n' 'fmt' 'Format the codebase using gofmt.'
	@printf '  %-30s %s\n' 'vet' 'Vet the codebase using govet.'
	@printf '  %-30s %s\n' 'clean' 'Clean build artifacts.'
	@printf '  %-30s %s\n' 'deps' 'Download project dependencies.'
	@printf '  %-30s %s\n' 'tidy' 'Ensure that the go.mod file matches the source code in the module.'
	@printf '  %-30s %s\n' 'lint' 'Run golangci-lint on the codebase.'
	@printf '  %-30s %s\n' 'install' 'Install client and server binaries to GOPATH/bin.'
	@printf '  %-30s %s\n' 'uninstall' 'Uninstall client and server binaries from GOPATH/bin.'
	@printf '  %-30s %s\n' 'run-client' 'Run the client binary with optional ARGS.'
	@printf '  %-30s %s\n' 'run-server' 'Run the server binary with optional ARGS.'
	@printf '  %-30s %s\n' 'test' 'Run unit tests.'
	@printf '  %-30s %s\n' 'cover' 'Run unit tests and generate coverage report.'
	@printf '  %-30s %s\n' 'test-sh' 'Run test.sh script.'
	@printf '  %-30s %s\n' 'test-large-directory-sh' 'Run test_large_directory.sh script.'
	@printf '  %-30s %s\n' 'test-directory-limit-sh' 'Run test_directory_limit.sh script.'
	@printf '  %-30s %s\n' 'help' 'Show this help message.'
