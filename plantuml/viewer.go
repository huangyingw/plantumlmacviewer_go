package plantuml

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png" // 导入 PNG 解码器
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// 缩放和平移相关常量
const (
	MinZoomScale       = 0.1   // 最小缩放比例 (10%)
	MaxZoomScale       = 5.0   // 最大缩放比例 (500%)
	ZoomStep           = 1.1   // 缩放步进 (10%)
	PanStep            = 50.0  // 平移步进 (像素)
	FitToWindowMargin  = 0.95  // 适应窗口时的边距比例
	MinContainerSize   = 200.0 // 最小容器尺寸，用于检测布局是否完成
	FitToWindowRetries = 5     // FitToWindow 最大重试次数
	FitToWindowDelay   = 200   // FitToWindow 重试延迟 (毫秒)
)

// scalableImageContainer 是一个可缩放的图像容器
// 实现 fyne.CanvasObject 接口，支持设置 MinSize
type scalableImageContainer struct {
	widget.BaseWidget
	image   *canvas.Image
	minSize fyne.Size
}

func newScalableImageContainer(img *canvas.Image) *scalableImageContainer {
	c := &scalableImageContainer{
		image:   img,
		minSize: fyne.NewSize(100, 100),
	}
	c.ExtendBaseWidget(c)
	return c
}

func (c *scalableImageContainer) CreateRenderer() fyne.WidgetRenderer {
	return &scalableImageRenderer{
		container: c,
		image:     c.image,
	}
}

func (c *scalableImageContainer) SetMinSize(size fyne.Size) {
	c.minSize = size
	// 不在这里调用 Refresh，由调用者负责刷新
}

func (c *scalableImageContainer) MinSize() fyne.Size {
	return c.minSize
}

type scalableImageRenderer struct {
	container *scalableImageContainer
	image     *canvas.Image
}

func (r *scalableImageRenderer) Layout(size fyne.Size) {
	r.image.Resize(size)
	r.image.Move(fyne.NewPos(0, 0))
}

func (r *scalableImageRenderer) MinSize() fyne.Size {
	return r.container.minSize
}

func (r *scalableImageRenderer) Refresh() {
	// Fyne 会自动刷新 Objects() 返回的对象
	// 不需要手动调用 r.image.Refresh()
}

func (r *scalableImageRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.image}
}

func (r *scalableImageRenderer) Destroy() {
}

// Viewer 表示PlantUML查看器
type Viewer struct {
	filePath       string
	content        string
	imageView      *canvas.Image
	imageContainer *scalableImageContainer // 包裹图像的容器，用于控制缩放
	container      *fyne.Container
	scroll         *container.Scroll // 滚动容器引用
	rendered       bool
	lastModified   time.Time // 文件最后修改时间
	stopMonitoring chan bool // 停止监控的信号通道
	onFileChanged  func()    // 文件变化时的回调函数
	scale          float32   // 缩放比例（1.0 = 100%）
	originalSize   fyne.Size // 原始图片大小
	mu             sync.RWMutex // 保护 scale 和 originalSize 的并发访问
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
		scale:          1.0, // 初始缩放比例 100%
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
	v.imageView.FillMode = canvas.ImageFillOriginal  // 使用原始大小，不自动缩放
	v.imageView.ScaleMode = canvas.ImageScaleFastest // 使用最快的缩放模式，提高性能

	// 创建一个可缩放的容器包裹图像，用于控制缩放
	v.imageContainer = newScalableImageContainer(v.imageView)

	// 创建滚动容器并保存引用
	v.scroll = container.NewScroll(v.imageContainer)
	v.container = container.NewMax(v.scroll)
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
		v.imageView.Refresh()
		v.rendered = true

		// 延迟适应窗口大小（等待布局完成）
		// 使用多次重试以确保布局完成，成功后立即停止
		go func() {
			for i := 0; i < FitToWindowRetries; i++ {
				time.Sleep(FitToWindowDelay * time.Millisecond)

				done := make(chan bool)
				fyne.Do(func() {
					success := false
					if v.scroll != nil &&
					   v.scroll.Size().Width >= MinContainerSize &&
					   v.scroll.Size().Height >= MinContainerSize {
						v.FitToWindow()
						success = true
					}
					done <- success
				})

				if <-done {
					break  // 成功则退出循环
				}
			}
		}()
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

	// 解码图像获取原始尺寸
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Printf("警告：无法解码图像以获取尺寸: %v", err)
	} else {
		bounds := img.Bounds()
		v.originalSize = fyne.NewSize(float32(bounds.Dx()), float32(bounds.Dy()))
		log.Printf("保存原始图像尺寸: %dx%d", bounds.Dx(), bounds.Dy())
	}

	// 自动导出 PNG 到源文件所在目录
	if err := v.exportPNG(imgData); err != nil {
		log.Printf("警告：自动导出 PNG 失败: %v", err)
		// 导出失败不影响正常显示，继续执行
	}

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

	// 解码图像获取原始尺寸
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Printf("警告：无法解码图像以获取尺寸: %v", err)
	} else {
		bounds := img.Bounds()
		v.originalSize = fyne.NewSize(float32(bounds.Dx()), float32(bounds.Dy()))
		log.Printf("保存原始图像尺寸: %dx%d", bounds.Dx(), bounds.Dy())
	}

	// 自动导出 PNG 到源文件所在目录
	if err := v.exportPNG(imgData); err != nil {
		log.Printf("警告：自动导出 PNG 失败: %v", err)
		// 导出失败不影响正常显示，继续执行
	}

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
	v.imageView.Refresh()
	v.rendered = true

	// 延迟适应窗口大小（等待布局完成）
	// 使用多次重试以确保布局完成，成功后立即停止
	go func() {
		for i := 0; i < FitToWindowRetries; i++ {
			time.Sleep(FitToWindowDelay * time.Millisecond)

			done := make(chan bool)
			fyne.Do(func() {
				success := false
				if v.scroll != nil &&
				   v.scroll.Size().Width >= MinContainerSize &&
				   v.scroll.Size().Height >= MinContainerSize {
					v.FitToWindow()
					success = true
				}
				done <- success
			})

			if <-done {
				break  // 成功则退出循环
			}
		}
	}()

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

// ZoomIn 放大图像
func (v *Viewer) ZoomIn() {
	v.mu.RLock()
	currentScale := v.scale
	v.mu.RUnlock()

	v.setZoom(currentScale * ZoomStep)
}

// ZoomOut 缩小图像
func (v *Viewer) ZoomOut() {
	v.mu.RLock()
	currentScale := v.scale
	v.mu.RUnlock()

	v.setZoom(currentScale / ZoomStep)
}

// ResetZoom 重置缩放为 100%
func (v *Viewer) ResetZoom() {
	v.setZoom(1.0)
}

// setZoom 设置缩放比例
// 注意：此方法必须在 UI 线程中调用
func (v *Viewer) setZoom(newScale float32) {
	// 限制缩放范围
	if newScale < MinZoomScale {
		newScale = MinZoomScale
	} else if newScale > MaxZoomScale {
		newScale = MaxZoomScale
	}

	v.mu.Lock()
	v.scale = newScale
	originalSize := v.originalSize
	v.mu.Unlock()

	// 应用缩放
	if v.imageView != nil && v.imageView.Resource != nil && originalSize.Width > 0 {
		newWidth := originalSize.Width * newScale
		newHeight := originalSize.Height * newScale
		newSize := fyne.NewSize(newWidth, newHeight)

		// 设置容器的 MinSize，这样滚动容器才知道内容有多大
		v.imageContainer.SetMinSize(newSize)

		// 刷新容器和滚动容器
		v.imageContainer.Refresh()
		if v.scroll != nil {
			v.scroll.Refresh()
		}
	}
}

// Pan 平移图像
// 注意：此方法必须在 UI 线程中调用
func (v *Viewer) Pan(dx, dy float32) {
	if v.scroll == nil {
		return
	}

	// 获取当前滚动位置
	currentPos := v.scroll.Offset

	// 计算新位置
	newPos := fyne.NewPos(currentPos.X+dx, currentPos.Y+dy)

	// 使用 ScrollToOffset 方法设置新的滚动位置
	v.scroll.ScrollToOffset(newPos)
}

// exportPNG 将渲染后的 PNG 图片导出到源文件所在目录
// 导出的文件名与源文件相同，只是扩展名改为 .png
func (v *Viewer) exportPNG(imgData []byte) error {
	// 计算导出路径：与源文件相同目录，相同文件名，扩展名改为 .png
	dir := filepath.Dir(v.filePath)
	baseName := filepath.Base(v.filePath)
	pngName := strings.TrimSuffix(baseName, filepath.Ext(baseName)) + ".png"
	exportPath := filepath.Join(dir, pngName)

	// 写入 PNG 文件
	err := os.WriteFile(exportPath, imgData, 0644)
	if err != nil {
		log.Printf("导出 PNG 失败: %v", err)
		return fmt.Errorf("无法导出 PNG 到 %s: %v", exportPath, err)
	}

	log.Printf("成功导出 PNG 到: %s", exportPath)
	return nil
}

// FitToWindow 使图像适应窗口大小
// 注意：此方法必须在 UI 线程中调用
func (v *Viewer) FitToWindow() {
	if v.scroll == nil || v.originalSize.Width == 0 || v.originalSize.Height == 0 {
		log.Printf("警告：无法适应窗口，scroll 或 originalSize 未初始化")
		return
	}

	// 获取滚动容器的大小
	scrollSize := v.scroll.Size()
	if scrollSize.Width == 0 || scrollSize.Height == 0 {
		log.Printf("警告：滚动容器尺寸为零，稍后重试")
		return
	}

	// 检查容器尺寸是否合理（至少 200x200），避免在布局未完成时计算
	if scrollSize.Width < 200 || scrollSize.Height < 200 {
		log.Printf("警告：滚动容器尺寸太小 (%.0fx%.0f)，可能布局未完成，跳过适应窗口",
			scrollSize.Width, scrollSize.Height)
		return
	}

	// 计算两个方向的缩放比例
	scaleX := scrollSize.Width / v.originalSize.Width
	scaleY := scrollSize.Height / v.originalSize.Height

	// 选择较小的缩放比例，确保图像完全可见
	fitScale := fyne.Min(scaleX, scaleY)

	// 稍微缩小一点，留出边距（95%）
	fitScale = fitScale * 0.95

	log.Printf("适应窗口：容器尺寸 %.0fx%.0f，原始尺寸 %.0fx%.0f，缩放比例 %.2f",
		scrollSize.Width, scrollSize.Height,
		v.originalSize.Width, v.originalSize.Height,
		fitScale)

	v.setZoom(fitScale)
}
