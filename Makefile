BINARY_NAME=focus
DAEMON_NAME=focusd
DIST_DIR=dist

.PHONY: all build clean test

# Default target
all: clean build

# Build the binary
build:
	@echo "Building $(BINARY_NAME) and $(DAEMON_NAME)..."
	@go build -o $(DIST_DIR)/$(BINARY_NAME) ./cmd/client
	@go build -o $(DIST_DIR)/$(DAEMON_NAME) ./cmd/daemon

clean:
	@echo "Cleaning $(DIST_DIR) directory..."
	@rm -rf $(DIST_DIR)
	@rm -f /tmp/$(BINARY_NAME).sock

test:
	@echo "Running tests..."
	@go test ./...
