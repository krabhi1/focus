BINARY_VERSION?=dev
BINARY_NAME=focus
DAEMON_NAME=focusd
EVENTS_NAME=focus-events
DIST_DIR=dist

.PHONY: all build clean test fmt install uninstall package-release check-release

# Default target
all: clean build

# Build the binary
build:
	@echo "Building $(BINARY_NAME), $(DAEMON_NAME), and $(EVENTS_NAME)..."
	@mkdir -p $(DIST_DIR)
	@go build -ldflags="-X main.version=$(BINARY_VERSION)" -o $(DIST_DIR)/$(BINARY_NAME) ./cmd/focus
	@go build -o $(DIST_DIR)/$(DAEMON_NAME) ./cmd/focusd
	@native_flags="$$(pkg-config --cflags --libs libsystemd)"; \
		gcc -Wall -Wextra -O2 ./native/session_event_listener.c -o $(DIST_DIR)/$(EVENTS_NAME) $$native_flags

run-daemon:
	@go run ./cmd/focusd

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

install:
	@./scripts/install-local.sh

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
