#!/bin/bash
# 1. Clean previous builds
echo "Cleaning old binaries..."
rm -rf dist

# 2. Build the Daemon
echo "Building Daemon..."
go build -o dist/focusd ./cmd/daemon
if [ $? -ne 0 ]; then echo "Daemon build failed!"; exit 1; fi

# 3. Build the Client
echo "Building Client..."
go build -o dist/focus ./cmd/client
if [ $? -ne 0 ]; then echo "Client build failed!"; exit 1; fi

echo "Build successful!"
