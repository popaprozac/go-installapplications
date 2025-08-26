# go-installapplications Makefile
# Build and package the go-installapplications binary

# Variables
BINARY_NAME := go-installapplications
PAYLOAD_DIR := payload/Library/go-installapplications
BUILD_DIR := build
VERSION := 1.0.0

# Go build variables
GOOS := darwin
CGO_ENABLED := 0

# Build flags  
LDFLAGS := -s -w
# Optimized build flags for minimal size
LDFLAGS_OPTIMIZED := -s -w -X main.version=$(VERSION) -buildmode=exe

.PHONY: all build build-intel build-arm build-universal build-tiny clean package package-intel package-arm package-universal help

# Default target (universal binary)
all: build-universal

# Architecture-specific builds
build-intel:
	@echo "Building $(BINARY_NAME) for darwin/amd64 (Intel)..."
	GOOS=$(GOOS) GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Intel binary built successfully: $(BINARY_NAME)"

build-arm:
	@echo "Building $(BINARY_NAME) for darwin/arm64 (Apple Silicon)..."
	GOOS=$(GOOS) GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Apple Silicon binary built successfully: $(BINARY_NAME)"

build-universal:
	@echo "Building universal binary (Intel + Apple Silicon)..."
	GOOS=$(GOOS) GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-amd64 .
	GOOS=$(GOOS) GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-arm64 .
	lipo -create -output $(BINARY_NAME) $(BINARY_NAME)-amd64 $(BINARY_NAME)-arm64
	@rm $(BINARY_NAME)-amd64 $(BINARY_NAME)-arm64
	@echo "Universal binary created: $(BINARY_NAME)"

# Legacy alias for backwards compatibility
build: build-universal
universal: build-universal

# Optimized tiny build (single architecture, maximum compression)
build-tiny:
	@echo "Building optimized tiny binary for current architecture..."
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS_OPTIMIZED)" -trimpath -o $(BINARY_NAME)-tiny .
	@echo "Tiny binary size:"
	@ls -lh $(BINARY_NAME)-tiny | awk '{printf "Size: %s\n", $$5}'

# Prepare payload for packaging
prepare:
	@echo "Preparing payload for packaging..."
	@mkdir -p $(PAYLOAD_DIR)
	@cp $(BINARY_NAME) $(PAYLOAD_DIR)/
	@chmod +x $(PAYLOAD_DIR)/$(BINARY_NAME)
	@echo "Payload prepared in $(PAYLOAD_DIR)/"

# Base package function
define build_package
	@echo "Building $(1) package with munkipkg..."
	@if ! command -v munkipkg >/dev/null 2>&1; then \
		echo "Error: munkipkg not found. Please install munkipkg first."; \
		echo "Visit: https://github.com/munki/munki-pkg"; \
		exit 1; \
	fi
	@echo "Note: Make sure to update the signing identity in build-info.json"
	@echo "Find your identity with: security find-identity -v -p codesigning"
	munkipkg .
	@echo "$(1) package built successfully in $(BUILD_DIR)/"
endef

# Architecture-specific packaging
package-intel: build-intel prepare
	$(call build_package,Intel)

package-arm: build-arm prepare
	$(call build_package,Apple Silicon)

package-universal: build-universal prepare
	$(call build_package,Universal)

# Default package target (universal)
package: package-universal

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME) $(BINARY_NAME)-amd64 $(BINARY_NAME)-arm64
	@rm -rf $(PAYLOAD_DIR)/$(BINARY_NAME)
	@rm -rf $(BUILD_DIR)
	@echo "Clean completed"

# Help target
help:
	@echo "go-installapplications Build System"
	@echo "=================================="
	@echo ""
	@echo "Build Targets:"
	@echo "  build-intel      - Build for Intel Macs (amd64)"
	@echo "  build-arm        - Build for Apple Silicon Macs (arm64)"
	@echo "  build-universal  - Build universal binary (Intel + Apple Silicon)"
	@echo "  build            - Alias for build-universal"
	@echo ""
	@echo "Package Targets:"
	@echo "  package-intel    - Build and package Intel-only installer"
	@echo "  package-arm      - Build and package Apple Silicon-only installer"
	@echo "  package-universal- Build and package universal installer"
	@echo "  package          - Alias for package-universal"
	@echo ""
	@echo "Utility Targets:"
	@echo "  prepare          - Copy binary to payload directory"
	@echo "  clean            - Remove all build artifacts"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build-intel               # Intel-only binary"
	@echo "  make package-arm              # Apple Silicon package"
	@echo "  make package                  # Universal package (recommended)"
	@echo ""
	@echo "Before packaging:"
	@echo "  1. Update build-info.json with your Developer ID Installer certificate"
	@echo "  2. Install munkipkg: https://github.com/munki/munki-pkg"
	@echo ""
	@echo "Find your signing identity:"
	@echo "  security find-identity -v -p codesigning"
	@echo ""
	@echo "Deployment recommendations:"
	@echo "  - Universal packages work on all Macs (recommended for most deployments)"
	@echo "  - Intel-only packages for legacy Mac fleets"
	@echo "  - ARM-only packages for Apple Silicon-only environments"
