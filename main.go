package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"plantumlmacviewer/ui"
)

var (
	version = "0.1.0"
)

func main() {
	// 解析命令行参数
	showVersion := flag.Bool("version", false, "显示版本信息")
	showHelp := flag.Bool("help", false, "显示帮助信息")
	flag.Parse()

	// 如果请求显示版本信息
	if *showVersion {
		fmt.Printf("PlantUML Viewer v%s\n", version)
		os.Exit(0)
	}

	// 如果请求显示帮助信息
	if *showHelp {
		fmt.Printf("PlantUML Viewer v%s\n\n", version)
		fmt.Println("用法: plantumlmacviewer [选项] [文件...]")
		fmt.Println("\n选项:")
		flag.PrintDefaults()
		fmt.Println("\n支持的文件类型: .puml, .plantuml, .pu")
		os.Exit(0)
	}

	// 获取传入的文件路径参数
	files := flag.Args()

	// 验证文件路径有效性
	validFiles := validateFiles(files)
	if len(files) > 0 && len(validFiles) == 0 {
		log.Println("警告：没有找到有效的PlantUML文件")
	}

	// 创建Fyne应用
	a := app.New()
	a.Settings().SetTheme(theme.LightTheme())

	// 创建主窗口
	w := a.NewWindow("PlantUML Viewer")
	w.Resize(fyne.NewSize(800, 600))

	// 如果没有文件参数，显示提示信息
	if len(validFiles) == 0 {
		log.Println("没有指定要打开的文件，请通过命令行参数提供PUML文件路径")
		w.SetTitle("PlantUML Viewer - 未加载文件")
	}

	// 初始化UI并设置到窗口
	content := setupUI(w, validFiles)
	w.SetContent(content)

	// 显示窗口并运行应用
	w.ShowAndRun()
}

// validateFiles 验证文件路径是否存在且是否为PlantUML文件
func validateFiles(files []string) []string {
	var validFiles []string
	for _, file := range files {
		// 检查文件是否存在
		info, err := os.Stat(file)
		if err != nil {
			log.Printf("警告：无法访问文件 %s: %v\n", file, err)
			continue
		}

		// 检查是否为目录
		if info.IsDir() {
			log.Printf("警告：%s 是一个目录，不是文件\n", file)
			continue
		}

		// 检查文件扩展名
		ext := filepath.Ext(file)
		if ext != ".puml" && ext != ".plantuml" && ext != ".pu" {
			log.Printf("警告：%s 可能不是PlantUML文件（扩展名不是.puml、.plantuml或.pu）\n", file)
			// 继续添加，因为有些文件可能没有标准扩展名但仍然包含有效的PlantUML内容
		}

		validFiles = append(validFiles, file)
	}
	return validFiles
}

// setupUI 初始化应用程序UI
func setupUI(w fyne.Window, files []string) fyne.CanvasObject {
	// 初始化UI组件
	log.Println("启动PlantUML查看器，文件列表:", files)

	// 创建UI实例
	mainUI, err := ui.NewMainUI(w, files)
	if err != nil {
		log.Fatalf("初始化UI失败: %v", err)
	}

	// 返回UI内容
	return mainUI.GetContent()
}

// 应用程序图标资源（需要添加实际的图标数据）
func resourceIconPng() fyne.Resource {
	// 在实际应用中，这里应该返回一个真正的图标资源
	// 简化处理，返回空资源
	return fyne.NewStaticResource("icon.png", []byte{})
}
