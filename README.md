# PlantUML Mac Viewer (Go版本)

这是一个使用Go语言和Fyne框架开发的PlantUML查看器应用程序。它允许你通过命令行打开一个或多个PlantUML(.puml)文件，并以图形方式显示它们。

## 功能特性

- 通过命令行打开一个或多个PlantUML文件
- 在标签页中显示多个文件
- 以只读方式查看PlantUML代码和预览图表
- 支持使用本地PlantUML JAR文件进行渲染

## 安装要求

- Go 1.18或更高版本
- Java Runtime Environment (JRE) - 用于渲染PlantUML
- PlantUML JAR文件

### 安装PlantUML

在MacOS上，你可以使用Homebrew安装PlantUML：

```bash
brew install plantuml
```

或者从PlantUML官网下载JAR文件：
[https://plantuml.com/download](https://plantuml.com/download)

## 编译和运行

### 编译应用

```bash
go build -o plantuml-viewer
```

### 运行应用

```bash
# 不带参数运行
./plantuml-viewer

# 指定一个或多个文件
./plantuml-viewer path/to/file1.puml path/to/file2.puml

# 显示版本信息
./plantuml-viewer -version

# 显示帮助信息
./plantuml-viewer -help
```

## 使用方法

1. 在命令行中启动应用程序，并指定PlantUML文件路径
2. 在左侧文件列表中选择要查看的文件
3. 使用底部标签页在源码和图表视图之间切换
4. 也可以在应用程序中点击"打开文件"按钮选择其它PlantUML文件

## 特别说明

本应用仅支持本地渲染模式，使用安装在本地的PlantUML JAR文件进行渲染。这需要安装Java和PlantUML。

## 许可证

MIT

## 贡献

欢迎提交问题报告和改进建议！ 