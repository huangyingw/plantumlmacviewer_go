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
	filePath  string
	content   string
	imageView *canvas.Image
	container *fyne.Container
	rendered  bool
}

// NewViewer 创建新的PlantUML查看器
func NewViewer(filePath string) (*Viewer, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("文件不存在: %s", filePath)
	}

	// 读取文件内容
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件: %v", err)
	}

	// 创建查看器
	viewer := &Viewer{
		filePath: filePath,
		content:  string(content),
	}

	// 初始化UI组件
	viewer.initComponents()

	// 尝试渲染PlantUML
	go viewer.renderPlantUML()

	return viewer, nil
}

// initComponents 初始化UI组件
func (v *Viewer) initComponents() {
	// 创建用于显示渲染图像的组件
	v.imageView = &canvas.Image{}
	v.imageView.FillMode = canvas.ImageFillContain // 内容适应屏幕

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
	// 如果内容中没有!pragma
	if !strings.Contains(content, "!pragma") {
		// 查找@startuml行
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "@startuml") {
				// 在@startuml后添加!pragma行
				lines = append(lines[:i+1], append([]string{"!pragma layout smetana"}, lines[i+1:]...)...)
				return strings.Join(lines, "\n")
			}
		}

		// 如果没有找到@startuml行，则在开头添加
		if strings.TrimSpace(content) != "" {
			return "@startuml\n!pragma layout smetana\n" + content + "\n@enduml"
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

	// 执行plantuml命令
	log.Printf("执行命令: plantuml -tpng %s", tempFile)
	cmd := exec.Command("plantuml", "-tpng", tempFile)
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
			cmd := exec.Command("plantuml", "-tpng", tempFile, "-o", tempDir)
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
		// 执行plantuml.jar来生成图像
		log.Printf("执行命令: java -jar %s -tpng %s -o %s", jarPath, tempFile, tempDir)
		cmd := exec.Command("java", "-jar", jarPath, "-tpng", tempFile, "-o", tempDir)
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
