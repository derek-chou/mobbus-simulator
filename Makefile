# Makefile for Modbus TCP Simulator

# 變數
APP_NAME := modbussim
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go 相關
GO := go
GOFLAGS := -v
LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildTime=$(BUILD_TIME) \
	-X main.GitCommit=$(GIT_COMMIT)

# 目錄
BUILD_DIR := build
DIST_DIR := dist

# 目標平台
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: all build build-all clean test lint fmt vet run docker docker-up docker-down help

# 預設目標
all: test build

# 建置
build:
	@echo "Building $(APP_NAME)..."
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) .

# 跨平台建置
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		$(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-$${platform%/*}-$${platform#*/} . ; \
		echo "Built: $(DIST_DIR)/$(APP_NAME)-$${platform%/*}-$${platform#*/}"; \
	done

# 清理
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR) $(DIST_DIR)
	$(GO) clean

# 測試
test:
	@echo "Running tests..."
	$(GO) test -v -race -cover ./...

# 測試覆蓋率
coverage:
	@echo "Generating coverage report..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# 效能測試
bench:
	@echo "Running benchmarks..."
	$(GO) test -bench=. -benchmem ./...

# Lint
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

# 格式化
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	@which goimports > /dev/null || (echo "Installing goimports..." && go install golang.org/x/tools/cmd/goimports@latest)
	goimports -w .

# Vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

# 執行
run: build
	@echo "Running $(APP_NAME)..."
	./$(BUILD_DIR)/$(APP_NAME) start

# 執行 (開發模式)
dev:
	@echo "Running in development mode..."
	$(GO) run . start

# Docker 建置
docker:
	@echo "Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(APP_NAME):$(VERSION) \
		-t $(APP_NAME):latest \
		.

# Docker Compose 啟動
docker-up:
	@echo "Starting with Docker Compose..."
	VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) GIT_COMMIT=$(GIT_COMMIT) \
	docker-compose up -d

# Docker Compose 停止
docker-down:
	@echo "Stopping Docker Compose..."
	docker-compose down

# Docker Compose 日誌
docker-logs:
	docker-compose logs -f

# 安裝依賴
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# 更新依賴
deps-update:
	@echo "Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy

# 生成配置範例
config:
	./$(BUILD_DIR)/$(APP_NAME) config generate -o config.example.json

# 驗證配置
config-validate:
	./$(BUILD_DIR)/$(APP_NAME) config validate -c config.json

# 安裝到系統
install: build
	@echo "Installing $(APP_NAME)..."
	sudo cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/

# 解除安裝
uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	sudo rm -f /usr/local/bin/$(APP_NAME)

# 幫助
help:
	@echo "Modbus TCP Simulator - Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all          - Run tests and build (default)"
	@echo "  build        - Build the binary"
	@echo "  build-all    - Build for all platforms"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  coverage     - Generate coverage report"
	@echo "  bench        - Run benchmarks"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  run          - Build and run"
	@echo "  dev          - Run in development mode"
	@echo "  docker       - Build Docker image"
	@echo "  docker-up    - Start with Docker Compose"
	@echo "  docker-down  - Stop Docker Compose"
	@echo "  docker-logs  - View Docker Compose logs"
	@echo "  deps         - Install dependencies"
	@echo "  deps-update  - Update dependencies"
	@echo "  config       - Generate config example"
	@echo "  install      - Install to system"
	@echo "  uninstall    - Uninstall from system"
	@echo "  help         - Show this help"
