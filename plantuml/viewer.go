package plantuml

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	// 显示加载指示器
	loadingLabel := widget.NewLabel("正在渲染...")
	loadingLabel.Alignment = fyne.TextAlignCenter
	v.container.Objects[0] = container.NewCenter(loadingLabel)
	v.container.Refresh()

	v.imageView.Resource = nil
	v.imageView.Refresh()

	// 优先尝试使用!pragma处理图表 (不需要graphviz)
	contentWithPragma := v.addPragmaIfNeeded(v.content)

	// 使用本地的plantuml.jar
	if img, err := v.renderUsingJar(contentWithPragma); err == nil {
		v.imageView.Resource = img
		v.container.Objects[0] = container.NewScroll(v.imageView)
		v.container.Refresh()
		v.rendered = true
		return
	}

	// 如果渲染失败，显示错误
	v.showRenderError("无法渲染PlantUML图表，请确保安装了Java和PlantUML。")
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

	// 查找可能的plantuml.jar路径
	jarPaths := []string{
		"/usr/local/bin/plantuml.jar",
		"/usr/local/Cellar/plantuml/*/plantuml.jar", // homebrew安装路径
		"/opt/plantuml/plantuml.jar",
		"/usr/share/plantuml/plantuml.jar",
		"/Applications/plantuml.jar",
		filepath.Join(os.Getenv("HOME"), "plantuml.jar"),
		filepath.Join(os.Getenv("HOME"), "bin/plantuml.jar"),
	}

	var jarPath string
	for _, path := range jarPaths {
		// 支持glob模式匹配
		if strings.Contains(path, "*") {
			matches, err := filepath.Glob(path)
			if err == nil && len(matches) > 0 {
				jarPath = matches[0]
				break
			}
		} else if _, err := os.Stat(path); err == nil {
			jarPath = path
			break
		}
	}

	if jarPath == "" {
		return nil, fmt.Errorf("找不到plantuml.jar")
	}

	// 执行plantuml.jar来生成图像
	cmd := exec.Command("java", "-jar", jarPath, "-tpng", tempFile, "-o", tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("执行plantuml失败: %v, %s", err, stderr.String())
	}

	// 读取生成的图像
	imgData, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取生成的图像: %v", err)
	}

	defer os.Remove(outputPath) // 删除临时生成的图像

	// 创建Fyne资源
	res := fyne.NewStaticResource("plantuml_image.png", imgData)
	return res, nil
}

// showRenderError 显示渲染错误
func (v *Viewer) showRenderError(message string) {
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

	// 替换内容
	v.container.Objects[0] = container.NewCenter(errorContainer)
	v.container.Refresh()
}
