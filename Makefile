# PlantUML Viewer Makefile

# 基本参数
APP_NAME = plantumlviewer
GO = go
GOBUILD = $(GO) build
GORUN = $(GO) run
GOCLEAN = $(GO) clean
GOTEST = $(GO) test
GOGET = $(GO) get
GOFMT = $(GO) fmt

# 系统相关路径
BINDIR = ./bin
MAIN_FILE = main.go

# 安装路径
PREFIX = /usr/local
INSTALL_BIN_DIR = $(PREFIX)/bin

# 构建标志
BUILD_FLAGS = 
ifeq ($(OS),Windows_NT)
	# Windows平台下添加使窗口不显示命令行
	BUILD_FLAGS += -ldflags -H=windowsgui
endif

# 基本命令
.PHONY: all build run clean test fmt install uninstall help

all: build

# 构建应用
build:
	@echo "正在构建 $(APP_NAME)..."
	@mkdir -p $(BINDIR)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINDIR)/$(APP_NAME) .
	@echo "构建完成: $(BINDIR)/$(APP_NAME)"

# 直接运行应用
run:
	@echo "直接运行 $(APP_NAME)..."
	$(GORUN) $(MAIN_FILE)

# 清理构建产物
clean:
	@echo "清理构建目录..."
	@rm -rf $(BINDIR)
	$(GOCLEAN)
	@echo "清理完成"

# 运行测试
test:
	@echo "运行测试..."
	$(GOTEST) -v ./...

# 格式化代码
fmt:
	@echo "格式化代码..."
	$(GOFMT) ./...

# 安装应用
install: build
	@echo "安装 $(APP_NAME) 到 $(INSTALL_BIN_DIR)..."
	@mkdir -p $(INSTALL_BIN_DIR)
	@cp $(BINDIR)/$(APP_NAME) $(INSTALL_BIN_DIR)/$(APP_NAME)
	@chmod +x $(INSTALL_BIN_DIR)/$(APP_NAME)
	@echo "安装完成: $(INSTALL_BIN_DIR)/$(APP_NAME)"

# 卸载应用
uninstall:
	@echo "卸载 $(APP_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(APP_NAME)
	@echo "卸载完成"

# 显示帮助信息
help:
	@echo "可用的命令:"
	@echo "  make build     - 构建应用"
	@echo "  make run       - 直接运行应用"
	@echo "  make clean     - 清理构建目录"
	@echo "  make test      - 运行测试"
	@echo "  make fmt       - 格式化代码"
	@echo "  make install   - 安装应用到系统 (默认: /usr/local/bin)"
	@echo "  make uninstall - 卸载应用"
	@echo "  make help      - 显示此帮助信息"
	@echo ""
	@echo "环境变量:"
	@echo "  PREFIX=/path   - 自定义安装路径前缀 (默认: /usr/local)" 