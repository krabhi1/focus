BINARY_VERSION?=dev
BINARY_NAME=focus
DAEMON_NAME=focusd
EVENTS_NAME=focus-events
DIST_DIR=dist
NATIVE_SRC=native/session_event_listener.c
NATIVE_FLAGS=$(shell pkg-config --cflags --libs libsystemd x11 xscrnsaver)

.PHONY: all build clean test fmt install uninstall package-release check-release

# Default target
all: clean build

# Build the binary
build:
	@echo "Building $(BINARY_NAME), $(DAEMON_NAME), and $(EVENTS_NAME)..."
	@mkdir -p $(DIST_DIR)
	@go build -ldflags="-X focus/cmd/client.version=$(BINARY_VERSION)" -o $(DIST_DIR)/$(BINARY_NAME) ./cmd/client
	@go build -o $(DIST_DIR)/$(DAEMON_NAME) ./cmd/daemon
	@gcc -Wall -Wextra -O2 $(NATIVE_SRC) -o $(DIST_DIR)/$(EVENTS_NAME) $(NATIVE_FLAGS)

run-daemon:
	@go run ./cmd/daemon

clean:
	@echo "Cleaning $(DIST_DIR) directory..."
	@rm -rf $(DIST_DIR)
	@rm -f /tmp/$(BINARY_NAME).sock

test:
	@echo "Running tests..."
	@go test ./...

fmt:
	@echo "Formatting files..."
	@go fmt ./...
	@clang-format -i $(NATIVE_SRC)

install:
	@./scripts/install.sh

uninstall:
	@./scripts/uninstall.sh

package-release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make package-release VERSION=v0.1.0"; \
		exit 1; \
	fi
	@./scripts/package-release.sh --version "$(VERSION)"

check-release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make check-release VERSION=v0.1.0"; \
		exit 1; \
	fi
	@./scripts/check-release.sh --tag "$(VERSION)"
