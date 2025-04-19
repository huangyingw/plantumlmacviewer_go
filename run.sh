#!/bin/zsh

# 颜色输出设置
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # 无颜色

# 输出带颜色的标题
echo "${BLUE}========================================${NC}"
echo "${GREEN}PlantUML Viewer 构建与运行脚本${NC}"
echo "${BLUE}========================================${NC}"

# 定义应用名称和构建目录
APP_NAME="plantumlviewer"
BIN_DIR="./bin"

# 创建构建目录（如果不存在）
if [ ! -d "$BIN_DIR" ]; then
    echo "${YELLOW}创建构建目录 $BIN_DIR...${NC}"
    mkdir -p "$BIN_DIR"
fi

# 编译应用
echo "${GREEN}正在编译 $APP_NAME...${NC}"
go build -o "$BIN_DIR/$APP_NAME" .

# 检查编译是否成功
if [ $? -ne 0 ]; then
    echo "${RED}编译失败!${NC}"
    exit 1
else
    echo "${GREEN}编译成功!${NC}"
fi

# 运行应用
echo "${GREEN}正在运行 $APP_NAME...${NC}"
"$BIN_DIR/$APP_NAME" "$@"

echo "${BLUE}========================================${NC}"
echo "${GREEN}完成${NC}" 