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

	// 优先尝试使用!pragma处理图表 (不需要graphviz)
	contentWithPragma := v.addPragmaIfNeeded(v.content)

	// 先尝试使用命令行工具
	img, err := v.renderUsingCommandLine(contentWithPragma)
	if err != nil {
		log.Printf("使用命令行工具渲染失败: %v，尝试使用JAR...", err)
		// 如果命令行工具失败，尝试使用JAR
		img, err = v.renderUsingJar(contentWithPragma)
		if err != nil {
			log.Printf("使用JAR渲染也失败: %v", err)
			v.showRenderError(fmt.Sprintf("无法渲染PlantUML图表: %v", err))
			return
		}
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

// addPragmaIfNeeded 添加!pragma指令以避免需要graphviz
func (v *Viewer) addPragmaIfNeeded(content string) string {
	// 获取文件名，用于title
	fileName := filepath.Base(v.filePath)

	// 如果内容中没有!pragma
	if !strings.Contains(content, "!pragma") {
		// 查找@startuml行
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "@startuml") {
				// 在@startuml后添加!pragma行和title
				titleLine := fmt.Sprintf("title %s", fileName)
				lines = append(lines[:i+1], append([]string{"!pragma layout smetana", titleLine}, lines[i+1:]...)...)
				return strings.Join(lines, "\n")
			}
		}

		// 如果没有找到@startuml行，则在开头添加
		if strings.TrimSpace(content) != "" {
			return fmt.Sprintf("@startuml\n!pragma layout smetana\ntitle %s\n%s\n@enduml", fileName, content)
		}
	} else if !strings.Contains(content, "title ") {
		// 如果已经有pragma但没有title，添加title
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "@startuml") ||
				strings.HasPrefix(strings.TrimSpace(line), "!pragma") {
				// 找到适合插入title的位置
				insertPos := i + 1
				// 如果下一行是!pragma，移到pragma后面
				if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "!pragma") {
					insertPos = i + 2
				}

				// 插入title行
				titleLine := fmt.Sprintf("title %s", fileName)
				lines = append(lines[:insertPos], append([]string{titleLine}, lines[insertPos:]...)...)
				return strings.Join(lines, "\n")
			}
		}
	}

	return content
}

// renderUsingCommandLine 使用plantuml命令行工具渲染
func (v *Viewer) renderUsingCommandLine(content string) (fyne.Resource, error) {
	// 检查命令行工具是否存在
	_, err := exec.LookPath("plantuml")
	if err != nil {
		return nil, fmt.Errorf("找不到plantuml命令行工具: %v", err)
	}

	// 创建临时文件保存修改后的内容
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, "temp_"+filepath.Base(v.filePath))

	// 写入修改后的内容到临时文件
	if err := ioutil.WriteFile(tempFile, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("无法创建临时文件: %v", err)
	}
	defer os.Remove(tempFile) // 删除临时文件

	// 注意：plantuml命令默认会在相同目录下生成PNG文件，输出路径是tempFile+".png"
	outputPath := tempFile + ".png"

	// 基于图表内容自动确定合适的DPI值
	dpi := v.calculateOptimalDPI(content)
	log.Printf("自动计算得到的最佳DPI值: %d", dpi)

	// 执行plantuml命令，使用动态计算的DPI值
	log.Printf("执行命令: plantuml -tpng -Sdpi=%d %s", dpi, tempFile)
	cmd := exec.Command("plantuml", "-tpng", fmt.Sprintf("-Sdpi=%d", dpi), tempFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		log.Printf("执行失败，stderr: %s, stdout: %s", stderr.String(), stdout.String())
		return nil, fmt.Errorf("执行plantuml命令失败: %v, %s", err, stderr.String())
	}

	log.Printf("命令执行成功")

	// 检查输出文件是否存在
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		log.Printf("警告: 输出文件不存在: %s，尝试查找其他可能的输出文件", outputPath)
		// 先尝试查找同名但不同扩展名的文件
		for _, ext := range []string{".png", ".svg", ".eps"} {
			possiblePath := tempFile + ext
			if _, err := os.Stat(possiblePath); err == nil {
				log.Printf("找到可能的输出文件: %s", possiblePath)
				outputPath = possiblePath
				break
			}
		}

		// 如果还是找不到，尝试查找tempDir中的所有图像文件
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			files, _ := filepath.Glob(filepath.Join(tempDir, "*.png"))
			// 查找最新创建的文件
			var newestFile string
			var newestTime time.Time
			for _, file := range files {
				fileInfo, err := os.Stat(file)
				if err == nil {
					if newestFile == "" || fileInfo.ModTime().After(newestTime) {
						newestFile = file
						newestTime = fileInfo.ModTime()
					}
				}
			}

			if newestFile != "" {
				log.Printf("找到最新的输出文件: %s", newestFile)
				outputPath = newestFile
			} else {
				return nil, fmt.Errorf("无法找到生成的图像文件")
			}
		}
	}

	// 读取生成的图像
	imgData, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取生成的图像: %v", err)
	}

	log.Printf("成功读取图像文件，大小: %d 字节", len(imgData))
	defer os.Remove(outputPath) // 删除临时生成的图像

	// 创建Fyne资源
	res := fyne.NewStaticResource("plantuml_image.png", imgData)
	return res, nil
}

// renderUsingJar 使用本地jar文件渲染
func (v *Viewer) renderUsingJar(content string) (fyne.Resource, error) {
	// 创建临时文件保存修改后的内容
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, "temp_"+filepath.Base(v.filePath))

	// 写入修改后的内容到临时文件
	if err := ioutil.WriteFile(tempFile, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("无法创建临时文件: %v", err)
	}
	defer os.Remove(tempFile) // 删除临时文件

	outputPath := filepath.Join(tempDir, filepath.Base(tempFile)+".png")

	// 先检查Java是否可用
	javaCmd := exec.Command("java", "-version")
	if err := javaCmd.Run(); err != nil {
		return nil, fmt.Errorf("Java未安装或不可用: %v", err)
	}

	// 基于图表内容自动确定合适的DPI值
	dpi := v.calculateOptimalDPI(content)
	log.Printf("自动计算得到的最佳DPI值: %d", dpi)

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

	var jarPath string
	for _, path := range jarPaths {
		// 支持glob模式匹配
		if strings.Contains(path, "*") {
			matches, err := filepath.Glob(path)
			if err == nil && len(matches) > 0 {
				jarPath = matches[0]
				log.Printf("找到PlantUML JAR包: %s", jarPath)
				break
			}
		} else if _, err := os.Stat(path); err == nil {
			jarPath = path
			log.Printf("找到PlantUML JAR包: %s", jarPath)
			break
		}
	}

	if jarPath == "" {
		// 如果找不到JAR文件，尝试使用命令行工具
		if _, err := exec.LookPath("plantuml"); err == nil {
			log.Printf("找到PlantUML命令行工具，尝试使用它")
			cmd := exec.Command("plantuml", "-tpng", fmt.Sprintf("-Sdpi=%d", dpi), tempFile, "-o", tempDir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				return nil, fmt.Errorf("执行plantuml命令失败: %v, %s", err, stderr.String())
			}

			log.Printf("使用PlantUML命令行工具渲染成功")
		} else {
			log.Printf("找不到plantuml.jar或命令行工具，请确保已安装PlantUML")
			return nil, fmt.Errorf("找不到plantuml.jar或命令行工具，请确保已安装PlantUML")
		}
	} else {
		// 执行plantuml.jar来生成图像，使用动态计算的DPI值
		log.Printf("执行命令: java -jar %s -tpng -Sdpi=%d %s -o %s", jarPath, dpi, tempFile, tempDir)
		cmd := exec.Command("java", "-jar", jarPath, "-tpng", fmt.Sprintf("-Sdpi=%d", dpi), tempFile, "-o", tempDir)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		var stdout bytes.Buffer
		cmd.Stdout = &stdout

		if err := cmd.Run(); err != nil {
			log.Printf("执行失败，stderr: %s, stdout: %s", stderr.String(), stdout.String())
			return nil, fmt.Errorf("执行plantuml失败: %v, %s", err, stderr.String())
		}

		log.Printf("使用JAR执行成功，输出路径: %s", outputPath)
	}

	// 检查输出文件是否存在
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		log.Printf("警告: 输出文件不存在: %s", outputPath)
		// 尝试查找可能的输出文件
		files, _ := filepath.Glob(filepath.Join(tempDir, "*.png"))
		if len(files) > 0 {
			log.Printf("找到可能的输出文件: %s", files[0])
			outputPath = files[0]
		} else {
			return nil, fmt.Errorf("无法找到生成的图像文件")
		}
	}

	// 读取生成的图像
	imgData, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取生成的图像: %v", err)
	}

	log.Printf("成功读取图像文件，大小: %d 字节", len(imgData))
	defer os.Remove(outputPath) // 删除临时生成的图像

	// 创建Fyne资源
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

	// 优先尝试使用!pragma处理图表 (不需要graphviz)
	contentWithPragma := v.addPragmaIfNeeded(v.content)

	// 先尝试使用命令行工具
	img, err := v.renderUsingCommandLine(contentWithPragma)
	if err != nil {
		log.Printf("使用命令行工具渲染失败: %v，尝试使用JAR...", err)
		// 如果命令行工具失败，尝试使用JAR
		img, err = v.renderUsingJar(contentWithPragma)
		if err != nil {
			log.Printf("使用JAR渲染也失败: %v", err)
			v.showRenderError(fmt.Sprintf("无法渲染PlantUML图表: %v", err))
			return err
		}
	}

	// 渲染成功，更新UI
	v.imageView.Resource = img
	v.container.Objects[0] = container.NewScroll(v.imageView)
	v.container.Refresh()
	v.rendered = true

	log.Printf("成功同步渲染文件: %s", v.filePath)
	return nil
}

// calculateOptimalDPI 计算最佳DPI值
func (v *Viewer) calculateOptimalDPI(content string) int {
	// 分析图表的复杂度来确定适合的DPI值
	lines := strings.Split(content, "\n")

	// 计算有效内容行数（非空行、非注释行）
	contentLines := 0
	classes := 0
	relationships := 0

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "'") ||
			strings.HasPrefix(trimmedLine, "!") || strings.HasPrefix(trimmedLine, "@") ||
			strings.HasPrefix(trimmedLine, "title") {
			continue // 跳过空行、注释行、标记行
		}

		contentLines++

		// 检测类定义（如class、interface、enum等）
		if strings.HasPrefix(trimmedLine, "class ") ||
			strings.HasPrefix(trimmedLine, "interface ") ||
			strings.HasPrefix(trimmedLine, "enum ") ||
			strings.HasPrefix(trimmedLine, "entity ") {
			classes++
		}

		// 检测关系（如继承、组合、聚合等）
		if strings.Contains(trimmedLine, "-->") ||
			strings.Contains(trimmedLine, "<--") ||
			strings.Contains(trimmedLine, "->") ||
			strings.Contains(trimmedLine, "<-") ||
			strings.Contains(trimmedLine, "--") ||
			strings.Contains(trimmedLine, "..") {
			relationships++
		}
	}

	// 基于内容复杂度计算DPI
	// 简单图表使用较低的DPI（清晰但不占用太多空间）
	// 复杂图表使用较高的DPI（确保细节清晰可见）

	// 基本DPI值
	baseDPI := 72

	// 根据内容复杂度调整DPI
	if classes > 10 || relationships > 15 || contentLines > 50 {
		// 非常复杂的图表
		return baseDPI + 48 // 120
	} else if classes > 5 || relationships > 8 || contentLines > 30 {
		// 较复杂的图表
		return baseDPI + 24 // 96
	} else if classes > 2 || relationships > 4 || contentLines > 15 {
		// 中等复杂度的图表
		return baseDPI + 12 // 84
	}

	// 简单图表使用基本DPI
	return baseDPI // 72
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
