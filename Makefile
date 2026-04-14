.PHONY: build clean test-all help report benchmark

# Detect OS using Go's GOOS
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Set binary name and commands based on OS
BUILD_DIR = build

ifeq ($(GOOS),windows)
	BINARY_NAME = $(BUILD_DIR)/hui-problem.exe
	RM_CMD = del
	MKDIR_CMD = mkdir
else
	BINARY_NAME = $(BUILD_DIR)/hui-problem
	RM_CMD = rm -f
	MKDIR_CMD = mkdir -p
endif

# Default test dataset
TESTDATA ?= testdata/DB_Utility.txt

# Benchmark settings
BENCH_DATASETS ?= testdata/mushroom_utility_SPMF.txt testdata/retail_utility_spmf.txt
BENCH_K ?= 1,10,100,1000
BENCH_TIMEOUT ?= 600

# Build the CLI binary
build:
	@$(MKDIR_CMD) $(BUILD_DIR)
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

# Run benchmark: rebuild binary, run all experiments, print comparison tables
benchmark: build
	@$(MKDIR_CMD) outputs
	python3 script/benchmark.py \
		--binary $(BINARY_NAME) \
		--datasets $(BENCH_DATASETS) \
		--k-values $(BENCH_K) \
		--output-dir outputs \
		--timeout $(BENCH_TIMEOUT)

# Clean up generated files
clean:
ifeq ($(GOOS),windows)
	rmdir /s /q $(BUILD_DIR) 2>nul || true
	rmdir /s /q outputs 2>nul || true
else
	rm -rf $(BUILD_DIR)/ outputs/
endif
	$(RM_CMD) *-output.txt

# Generate PDF report from LaTeX
report:
	@echo "Building PDF report from LaTeX..."
	@cd report && pdflatex -interaction=nonstopmode -halt-on-error main.tex > /dev/null 2>&1
	@cd report && pdflatex -interaction=nonstopmode -halt-on-error main.tex > /dev/null 2>&1
	@echo "✓ Report generated successfully: report/main.pdf"

# Show help
help:
	@echo "Available targets:"
	@echo "  build       - Build the CLI binary"
	@echo "  test-all    - Run all algorithms with separate output files"
	@echo "  run         - Run a single algorithm (ALGO=tku|tko|ptku|thui)"
	@echo "  benchmark   - Run benchmark experiments and print comparison tables"
	@echo "  report      - Generate PDF report from LaTeX sources"
	@echo "  clean       - Remove generated files"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Variables:"
	@echo "  TESTDATA        - Path to test dataset (default: testdata/DB_Utility.txt)"
	@echo "  BENCH_DATASETS  - Space-separated dataset paths for benchmark"
	@echo "  BENCH_K         - Comma-separated k values (default: 1,10,100,1000)"
	@echo "  BENCH_TIMEOUT   - Timeout per run in seconds (default: 600)"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make test-all"
	@echo "  make test-all TESTDATA=testdata/Chicago_Crimes_2001_to_2017_utility.txt"
	@echo "  make run ALGO=tku"
	@echo "  make run ALGO=thui TESTDATA=testdata/Chicago_Crimes_2001_to_2017_utility.txt"
	@echo "  make benchmark"
	@echo "  make benchmark BENCH_DATASETS=testdata/DB_Utility.txt BENCH_K=3,5,10"
