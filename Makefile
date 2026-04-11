.PHONY: build clean test-all help

# Detect OS using Go's GOOS
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Set binary name and commands based on OS
ifeq ($(GOOS),windows)
	BINARY_NAME = hui-problem.exe
	RM_CMD = del
	MKDIR_CMD = mkdir
else
	BINARY_NAME = hui-problem
	RM_CMD = rm -f
	MKDIR_CMD = mkdir -p
endif

# Default test dataset
TESTDATA ?= testdata/DB_Utility.txt

# Build the CLI binary
build:
	go build -o $(BINARY_NAME) ./cmd/hui-problem

# Run all algorithms on the test dataset with separate output files
test-all: build
	@echo "Running all algorithms on $(TESTDATA)..."
	@$(MKDIR_CMD) outputs
	$(BINARY_NAME) tku -i $(TESTDATA) -o outputs/tku-output.txt -k 10
	$(BINARY_NAME) tko -i $(TESTDATA) -o outputs/tko-output.txt -k 8
	$(BINARY_NAME) ptku -i $(TESTDATA) -o outputs/ptku-output.txt -k 10
	$(BINARY_NAME) thui -i $(TESTDATA) -o outputs/thui-output.txt -k 10
	@echo "✓ All algorithms completed. Results saved to outputs/ directory"

# Run a single algorithm (usage: make run ALGO=tku)
run: build
	@if [ -z "$(ALGO)" ]; then \
		echo "Usage: make run ALGO=<tku|tko|ptku|thui>"; \
		exit 1; \
	fi
	@$(MKDIR_CMD) outputs
	$(BINARY_NAME) $(ALGO) -i $(TESTDATA) -o outputs/$(ALGO)-output.txt -k 10

# Clean up generated files
clean:
	$(RM_CMD) $(BINARY_NAME)
ifeq ($(GOOS),windows)
	rmdir /s /q outputs 2>nul || true
else
	rm -rf outputs/
endif
	$(RM_CMD) *-output.txt

# Show help
help:
	@echo "Available targets:"
	@echo "  build       - Build the CLI binary"
	@echo "  test-all    - Run all algorithms with separate output files"
	@echo "  run         - Run a single algorithm (ALGO=tku|tko|ptku|thui)"
	@echo "  clean       - Remove generated files"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Variables:"
	@echo "  TESTDATA - Path to test dataset (default: testdata/DB_Utility.txt)"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make test-all"
	@echo "  make test-all TESTDATA=testdata/Chicago_Crimes_2001_to_2017_utility.txt"
	@echo "  make run ALGO=tku"
	@echo "  make run ALGO=thui TESTDATA=testdata/Chicago_Crimes_2001_to_2017_utility.txt"
