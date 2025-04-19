#!/bin/zsh

# 测试脚本 - 测试窗口最大化和多文件发送

# 颜色输出设置
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # 无颜色

# 输出带颜色的标题
echo "${BLUE}========================================${NC}"
echo "${GREEN}PlantUML Viewer 窗口测试脚本${NC}"
echo "${BLUE}========================================${NC}"

# 定义变量
APP_BIN_DIR="./bin"
APP_NAME="plantumlviewer"
TEST_FILES="example.puml test.puml"

# 如果不存在bin目录，先编译
if [ ! -f "$APP_BIN_DIR/$APP_NAME" ]; then
    echo "${YELLOW}应用程序不存在，先编译...${NC}"
    make build
fi

# 启动应用程序
echo "${GREEN}第一步: 启动应用程序...${NC}"
$APP_BIN_DIR/$APP_NAME example.puml &
APP_PID=$!

# 等待3秒，让应用程序完全启动
echo "${YELLOW}等待应用程序启动...${NC}"
sleep 3

# 检查进程是否在运行
if ps -p $APP_PID > /dev/null; then
    echo "${GREEN}应用程序已启动，PID: $APP_PID${NC}"
    
    # 第二步：发送更多文件
    echo "${GREEN}第二步: 发送更多文件到已运行的实例...${NC}"
    $APP_BIN_DIR/$APP_NAME test.puml sequence_example.puml
    
    # 等待5秒查看效果
    echo "${YELLOW}等待5秒观察文件加载效果...${NC}"
    sleep 5
    
    # 检查应用程序是否仍在运行
    if ps -p $APP_PID > /dev/null; then
        echo "${GREEN}应用程序仍在运行，请手动关闭查看效果${NC}"
        echo "${YELLOW}测试完成后，可以使用 kill $APP_PID 关闭应用程序${NC}"
    else
        echo "${RED}应用程序已退出！${NC}"
    fi
else
    echo "${RED}应用程序启动失败！${NC}"
fi

echo "${BLUE}========================================${NC}"
echo "${GREEN}测试完成${NC}" 