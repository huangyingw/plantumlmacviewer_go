#!/bin/zsh

# 编译并运行程序，打开测试目录中的文件
go build -o bin/plantumlviewer main.go
./bin/plantumlviewer testdata/*

echo "已启动程序并打开测试文件夹中的所有puml文件"
echo "测试功能："
echo "1. 所有测试文件现在都放在testdata文件夹中"
echo "2. 修改puml文件将自动刷新并切换到对应标签页"
echo "3. 可以使用Cmd+W关闭当前标签页"
echo "4. 窗口大小会自动适应屏幕尺寸"
echo "5. 长文件名在标签页中会被截断，避免显示问题" 