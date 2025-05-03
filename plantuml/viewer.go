package plantuml

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Viewer 表示PlantUML查看器
type Viewer struct {
	filePath       string
	content        string
	imageView      *canvas.Image
	container      *fyne.Container
	rendered       bool
	lastModified   time.Time // 文件最后修改时间
	stopMonitoring chan bool // 停止监控的信号通道
	onFileChanged  func()    // 文件变化时的回调函数
}

// NewViewer 创建新的PlantUML查看器
func NewViewer(filePath string) (*Viewer, error) {
	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("文件不存在: %s", filePath)
	}

	// 读取文件内容
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件: %v", err)
	}

	// 创建查看器
	viewer := &Viewer{
		filePath:       filePath,
		content:        string(content),
		lastModified:   fileInfo.ModTime(),
		stopMonitoring: make(chan bool),
	}

	// 初始化UI组件
	viewer.initComponents()

	// 立即渲染PlantUML，而不是异步进行
	// 这确保在视图显示时，图像已经准备好
	err = viewer.renderSynchronously()
	if err != nil {
		log.Printf("同步渲染失败: %v，尝试异步渲染...", err)
		// 如果同步渲染失败，则使用异步方式作为备选方案
		go viewer.renderPlantUML()
	}

	// 启动文件监控
	go viewer.monitorFile()

	return viewer, nil
}

// initComponents 初始化UI组件
func (v *Viewer) initComponents() {
	// 创建用于显示渲染图像的组件
	v.imageView = &canvas.Image{}
	v.imageView.FillMode = canvas.ImageFillContain   // 内容适应屏幕
	v.imageView.ScaleMode = canvas.ImageScaleFastest // 使用最快的缩放模式，提高性能

	// 创建容器
	v.container = container.NewMax(container.NewScroll(v.imageView))
}

// GetCanvas 返回查看器的Canvas对象
func (v *Viewer) GetCanvas() fyne.CanvasObject {
	return v.container
}

// renderPlantUML 渲染PlantUML图表
func (v *Viewer) renderPlantUML() {
	log.Printf("开始渲染文件: %s", v.filePath)

	// 重新读取文件内容，确保获取最新的内容
	content, err := ioutil.ReadFile(v.filePath)
	if err == nil {
		// 只有成功读取时才更新内容
		v.content = string(content)
		log.Printf("已重新读取文件内容，大小: %d 字节", len(content))
	} else {
		log.Printf("警告：无法重新读取文件内容: %v，使用缓存的内容", err)
	}

	// 使用 JAR 包渲染 PlantUML 图表
	img, err := v.renderUsingJar()
	if err != nil {
		log.Printf("使用 JAR 渲染失败: %v", err)
		v.showRenderError(fmt.Sprintf("无法渲染PlantUML图表: %v", err))
		return
	}

	// 渲染成功，更新UI
	fyne.Do(func() {
		v.imageView.Resource = img
		v.container.Objects[0] = container.NewScroll(v.imageView)
		v.container.Refresh()
		v.rendered = true
	})

	log.Printf("成功渲染文件: %s", v.filePath)
}

// renderUsingJar 使用本地jar文件渲染PlantUML图表
func (v *Viewer) renderUsingJar() (fyne.Resource, error) {
	// 查找可能的plantuml.jar路径
	jarPaths := []string{
		"/usr/local/bin/plantuml.jar",
		"/usr/local/Cellar/plantuml/*/libexec/plantuml.jar", // 根据实际情况找到的Homebrew安装路径
		"/usr/local/Cellar/plantuml/*/plantuml.jar",         // homebrew安装路径
		"/opt/plantuml/plantuml.jar",
		"/usr/share/plantuml/plantuml.jar",
		"/Applications/plantuml.jar",
		filepath.Join(os.Getenv("HOME"), "plantuml.jar"),
		filepath.Join(os.Getenv("HOME"), "bin/plantuml.jar"),
		filepath.Join(os.Getenv("HOME"), ".plantuml/plantuml.jar"),
		filepath.Join(os.Getenv("HOME"), "/Downloads/plantuml.jar"),
	}

	// 寻找最新版本的 PlantUML JAR 包
	var jarPath string
	for _, path := range jarPaths {
		// 支持glob模式匹配
		if strings.Contains(path, "*") {
			matches, err := filepath.Glob(path)
			if err == nil && len(matches) > 0 {
				// 按修改时间排序，取最新的
				var latestJar string
				var latestTime time.Time
				for _, match := range matches {
					info, err := os.Stat(match)
					if err == nil {
						if latestJar == "" || info.ModTime().After(latestTime) {
							latestJar = match
							latestTime = info.ModTime()
						}
					}
				}
				if latestJar != "" {
					jarPath = latestJar
					log.Printf("找到最新的 PlantUML JAR 包: %s", jarPath)
					break
				}
			}
		} else if _, err := os.Stat(path); err == nil {
			jarPath = path
			log.Printf("找到 PlantUML JAR 包: %s", jarPath)
			break
		}
	}

	if jarPath == "" {
		// 查找 plantuml 命令行工具
		_, err := exec.LookPath("plantuml")
		if err == nil {
			log.Printf("找不到 JAR 包，但找到 plantuml 命令行工具，使用命令行工具渲染")
			return v.renderUsingCommandLine()
		}

		return nil, fmt.Errorf("找不到 plantuml.jar 或命令行工具，请确保已安装 PlantUML")
	}

	// 创建临时目录用于存放生成的图像
	tempDir, err := ioutil.TempDir("", "plantuml")
	if err != nil {
		return nil, fmt.Errorf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tempDir) // 函数返回时删除临时目录

	// 获取文件名
	fileName := filepath.Base(v.filePath)
	pngName := strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ".png"
	outputPath := filepath.Join(tempDir, pngName)

	// 执行 plantuml.jar 命令
	log.Printf("执行命令: java -jar %s -tpng -o %s %s", jarPath, tempDir, v.filePath)
	cmd := exec.Command("java", "-jar", jarPath, "-tpng", "-o", tempDir, v.filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		log.Printf("执行失败，stderr: %s, stdout: %s", stderr.String(), stdout.String())
		return nil, fmt.Errorf("执行 plantuml 失败: %v, %s", err, stderr.String())
	}

	log.Printf("命令执行成功，查找生成的图像文件")

	// 检查输出文件是否存在
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		log.Printf("警告: 预期的输出文件不存在: %s，尝试查找生成的图像文件", outputPath)
		// 查找临时目录中的 PNG 文件
		files, err := filepath.Glob(filepath.Join(tempDir, "*.png"))
		if err != nil || len(files) == 0 {
			return nil, fmt.Errorf("无法找到生成的图像文件")
		}
		outputPath = files[0] // 使用第一个找到的 PNG 文件
		log.Printf("找到图像文件: %s", outputPath)
	}

	// 读取生成的图像
	imgData, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取生成的图像: %v", err)
	}

	log.Printf("成功读取图像文件，大小: %d 字节", len(imgData))

	// 创建 Fyne 资源
	res := fyne.NewStaticResource("plantuml_image.png", imgData)
	return res, nil
}

// renderUsingCommandLine 使用命令行工具渲染PlantUML图表
func (v *Viewer) renderUsingCommandLine() (fyne.Resource, error) {
	// 创建临时目录用于存放生成的图像
	tempDir, err := ioutil.TempDir("", "plantuml")
	if err != nil {
		return nil, fmt.Errorf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tempDir) // 函数返回时删除临时目录

	// 获取文件名
	fileName := filepath.Base(v.filePath)
	pngName := strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ".png"
	outputPath := filepath.Join(tempDir, pngName)

	// 执行 plantuml 命令
	log.Printf("执行命令: plantuml -tpng -o %s %s", tempDir, v.filePath)
	cmd := exec.Command("plantuml", "-tpng", "-o", tempDir, v.filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		log.Printf("执行失败，stderr: %s, stdout: %s", stderr.String(), stdout.String())
		return nil, fmt.Errorf("执行 plantuml 命令失败: %v, %s", err, stderr.String())
	}

	log.Printf("命令执行成功，查找生成的图像文件")

	// 检查输出文件是否存在
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		log.Printf("警告: 预期的输出文件不存在: %s，尝试查找生成的图像文件", outputPath)
		// 查找临时目录中的 PNG 文件
		files, err := filepath.Glob(filepath.Join(tempDir, "*.png"))
		if err != nil || len(files) == 0 {
			return nil, fmt.Errorf("无法找到生成的图像文件")
		}
		outputPath = files[0] // 使用第一个找到的 PNG 文件
		log.Printf("找到图像文件: %s", outputPath)
	}

	// 读取生成的图像
	imgData, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取生成的图像: %v", err)
	}

	log.Printf("成功读取图像文件，大小: %d 字节", len(imgData))

	// 创建 Fyne 资源
	res := fyne.NewStaticResource("plantuml_image.png", imgData)
	return res, nil
}

// showRenderError 显示渲染错误
func (v *Viewer) showRenderError(message string) {
	log.Printf("渲染错误: %s", message)

	errorText := widget.NewLabel(message)
	errorText.Alignment = fyne.TextAlignCenter

	// 添加重试按钮
	retryButton := widget.NewButton("重试", func() {
		go v.renderPlantUML()
	})

	errorContainer := container.NewVBox(
		errorText,
		container.NewCenter(retryButton),
	)

	// 在UI线程中更新界面
	fyne.Do(func() {
		v.container.Objects[0] = container.NewCenter(errorContainer)
		v.container.Refresh()
	})
}

// renderSynchronously 同步渲染PlantUML图表
func (v *Viewer) renderSynchronously() error {
	log.Printf("开始同步渲染文件: %s", v.filePath)

	// 重新读取文件内容，确保获取最新的内容
	content, err := ioutil.ReadFile(v.filePath)
	if err == nil {
		// 只有成功读取时才更新内容
		v.content = string(content)
		log.Printf("已重新读取文件内容，大小: %d 字节", len(content))
	} else {
		log.Printf("警告：无法重新读取文件内容: %v，使用缓存的内容", err)
	}

	// 使用 JAR 包渲染 PlantUML 图表
	img, err := v.renderUsingJar()
	if err != nil {
		log.Printf("使用 JAR 渲染失败: %v", err)
		v.showRenderError(fmt.Sprintf("无法渲染PlantUML图表: %v", err))
		return err
	}

	// 渲染成功，更新UI
	v.imageView.Resource = img
	v.container.Objects[0] = container.NewScroll(v.imageView)
	v.container.Refresh()
	v.rendered = true

	log.Printf("成功同步渲染文件: %s", v.filePath)
	return nil
}

// monitorFile 监控文件变化并在变化时自动刷新
func (v *Viewer) monitorFile() {
	// 为了减少CPU使用，不使用过于频繁的ticker
	ticker := time.NewTicker(500 * time.Millisecond) // 每500毫秒检查一次文件变化
	defer ticker.Stop()

	log.Printf("开始监控文件: %s", v.filePath)

	// 为防止频繁刷新，添加刷新冷却时间
	lastRefreshTime := time.Now()
	refreshCooldown := 1 * time.Second // 两次刷新之间最短的时间间隔

	// 记录文件大小，用于快速检测文件是否变化
	fileInfo, err := os.Stat(v.filePath)
	var lastSize int64 = 0
	if err == nil {
		lastSize = fileInfo.Size()
	}

	for {
		select {
		case <-ticker.C:
			// 检查文件是否被修改
			fileInfo, err := os.Stat(v.filePath)
			if err != nil {
				log.Printf("监控文件时出错: %v", err)
				continue
			}

			// 快速检查：如果文件大小没变，通常内容也没变
			currentSize := fileInfo.Size()
			currentModTime := fileInfo.ModTime()

			// 检查文件大小和修改时间是否有变化
			if (currentSize != lastSize || currentModTime.After(v.lastModified)) &&
				time.Since(lastRefreshTime) > refreshCooldown {

				log.Printf("检测到文件 %s 可能有变化，检查内容", v.filePath)

				// 文件可能已修改，读取内容确认
				content, err := ioutil.ReadFile(v.filePath)
				if err != nil {
					log.Printf("读取已更改文件失败: %v", err)
					continue
				}

				// 只有当内容真的变了才重新渲染
				newContent := string(content)
				if newContent != v.content {
					log.Printf("文件内容确实有变化，准备刷新显示")
					v.content = newContent
					v.lastModified = currentModTime
					lastSize = currentSize
					lastRefreshTime = time.Now()

					// 使用UI线程更新，确保UI操作线程安全
					fyne.Do(func() {
						go v.renderPlantUML() // 在UI线程中启动渲染

						// 如果设置了回调函数，调用它
						if v.onFileChanged != nil {
							log.Printf("调用文件变化回调函数")
							v.onFileChanged()
						}
					})
				} else {
					log.Printf("文件修改时间或大小变化，但内容未变，不需刷新")
				}
			}
		case <-v.stopMonitoring:
			// 收到停止监控的信号
			log.Printf("停止监控文件: %s", v.filePath)
			return
		}
	}
}

// StopMonitoring 停止文件监控
func (v *Viewer) StopMonitoring() {
	v.stopMonitoring <- true
}

// SetOnFileChanged 设置文件变化时的回调函数
func (v *Viewer) SetOnFileChanged(callback func()) {
	v.onFileChanged = callback
}
