#!/bin/zsh

# 创建一个简单的puml文件
echo "@startuml
class Foo
class Bar
Foo --> Bar
@enduml" > test.puml

# 显示puml文件内容
echo "PUML内容:"
cat test.puml
echo ""

# 尝试使用plantuml命令行工具渲染
echo "使用命令行工具渲染:"
plantuml -tpng test.puml

# 检查是否生成了图片
if [ -f "test.png" ]; then
    echo "成功: 已生成图片 test.png ($(du -h test.png | cut -f1))"
else
    echo "失败: 没有生成图片"
fi

# 显示plantuml版本
echo ""
echo "PlantUML版本信息:"
plantuml -version

# 显示java版本
echo ""
echo "Java版本信息:"
java -version 