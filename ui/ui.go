package ui

import (
	"fmt"
	"log"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"plantumlmacviewer/plantuml"
)

// MainUI 是应用程序的主UI结构
type MainUI struct {
	window      fyne.Window
	files       []string
	Tabs        *container.DocTabs          // 导出字段以便可以从外部访问
	OpenedFiles map[string]int              // 导出字段以便可以从外部访问
	viewers     map[string]*plantuml.Viewer // 存储查看器引用，用于管理文件监控
}

// NewMainUI 创建新的UI实例
func NewMainUI(window fyne.Window, files []string) (*MainUI, error) {
	ui := &MainUI{
		window:      window,
		files:       files,
		OpenedFiles: make(map[string]int),
		viewers:     make(map[string]*plantuml.Viewer),
	}
	return ui, nil
}

// InitializeUI 初始化UI组件
func (ui *MainUI) InitializeUI() fyne.CanvasObject {
	// 创建Tab容器用于显示多个PUML文件
	ui.Tabs = container.NewDocTabs()
	ui.Tabs.SetTabLocation(container.TabLocationTop)

	// 如果有文件参数传入，立即打开它们
	for _, file := range ui.files {
		ui.OpenFile(file)
	}

	// 监听标签关闭事件，从OpenedFiles中移除并停止文件监控
	ui.Tabs.OnClosed = func(item *container.TabItem) {
		log.Printf("关闭标签页: %s", item.Text)

		// 查找并移除关闭的文件
		var closedPath string
		var closedIndex int

		for path, index := range ui.OpenedFiles {
			if index >= len(ui.Tabs.Items) {
				// 索引已经超出范围，直接删除
				log.Printf("删除无效的文件记录: %s, 索引: %d", path, index)
				delete(ui.OpenedFiles, path)
				continue
			}

			if filepath.Base(path) == item.Text || ui.Tabs.Items[index] == item {
				closedPath = path
				closedIndex = index

				// 停止文件监控
				if viewer, exists := ui.viewers[path]; exists {
					log.Printf("停止对文件 %s 的监控", path)
					viewer.StopMonitoring()
					delete(ui.viewers, path)
				}

				break
			}
		}

		// 如果找到了被关闭的标签对应的文件
		if closedPath != "" {
			log.Printf("从映射中删除文件: %s, 索引: %d", closedPath, closedIndex)
			delete(ui.OpenedFiles, closedPath)

			// 更新其他文件的索引
			for otherPath, otherIndex := range ui.OpenedFiles {
				if otherIndex > closedIndex {
					ui.OpenedFiles[otherPath] = otherIndex - 1
					log.Printf("更新文件索引: %s, 从 %d 到 %d", otherPath, otherIndex, otherIndex-1)
				}
			}
		} else {
			log.Printf("警告: 无法找到被关闭的标签对应的文件记录")
			// 记录当前所有标签和文件映射的状态，用于调试
			log.Printf("当前标签数量: %d", len(ui.Tabs.Items))
			for i, tab := range ui.Tabs.Items {
				log.Printf("标签[%d]: %s", i, tab.Text)
			}
			log.Printf("当前文件映射:")
			for path, idx := range ui.OpenedFiles {
				log.Printf("  %s -> %d", path, idx)
			}
		}
	}

	// 监听标签选择事件，更新窗口标题
	ui.Tabs.OnSelected = func(item *container.TabItem) {
		// 获取当前选中的标签索引
		selectedIndex := ui.Tabs.SelectedIndex()

		// 从 OpenedFiles 映射中找到对应的完整文件路径
		var fullFileName string
		for path, index := range ui.OpenedFiles {
			if index == selectedIndex {
				// 找到了对应的文件路径，提取完整文件名
				fullFileName = filepath.Base(path)
				break
			}
		}

		// 如果找到了完整文件名，使用它；否则使用标签文本（fallback）
		if fullFileName != "" {
			ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", fullFileName))
		} else {
			ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", item.Text))
		}
	}

	// 直接返回tabs容器作为主布局
	return ui.Tabs
}

// truncateFileName 截断过长的文件名，确保标签页不会过长
func truncateFileName(fileName string, maxLength int) string {
	if len(fileName) <= maxLength {
		return fileName
	}

	// 分离文件名和扩展名
	ext := filepath.Ext(fileName)
	baseName := fileName[:len(fileName)-len(ext)]

	// 计算需要保留的字符数（考虑到要添加"..."）
	keep := maxLength - 3 - len(ext)
	if keep < 10 {
		keep = 10 // 确保至少保留10个字符
	}

	// 返回截断后的文件名
	return baseName[:keep] + "..." + ext
}

// createFileChangeCallback 创建文件变化回调函数
// 当检测到文件变化时，自动切换到对应的标签页
func (ui *MainUI) createFileChangeCallback(filePath string) func() {
	return func() {
		currentIndex, exists := ui.OpenedFiles[filePath]
		if exists && currentIndex >= 0 && currentIndex < len(ui.Tabs.Items) {
			log.Printf("检测到文件变化，切换到标签页: %s", filepath.Base(filePath))
			ui.Tabs.SelectIndex(currentIndex)
		} else {
			log.Printf("检测到文件变化，但标签索引无效: %d，当前标签数量: %d", currentIndex, len(ui.Tabs.Items))
		}
	}
}

// OpenFile 打开文件并创建新标签页，如果文件已打开则切换到对应标签页
func (ui *MainUI) OpenFile(filePath string) {
	ui.OpenFileWithOptions(filePath, true)
}

// OpenFileWithOptions 打开文件并创建新标签页，可选是否自动选择标签
func (ui *MainUI) OpenFileWithOptions(filePath string, selectTab bool) {
	// 获取绝对路径
	absPath, err := filepath.Abs(filePath)
	if err == nil {
		filePath = absPath
	}

	// 检查文件是否已打开
	if tabIndex, exists := ui.OpenedFiles[filePath]; exists {
		// 文件已经打开，检查索引是否有效
		if tabIndex < 0 || tabIndex >= len(ui.Tabs.Items) {
			log.Printf("警告: 文件 %s 的标签索引 %d 无效，当前标签数量: %d", filePath, tabIndex, len(ui.Tabs.Items))
			// 从OpenedFiles中删除无效的记录
			delete(ui.OpenedFiles, filePath)
			// 重新打开文件
			ui.OpenFileWithOptions(filePath, selectTab)
			return
		}

		// 文件已经打开，如果需要则切换到对应标签
		if selectTab {
			ui.Tabs.SelectIndex(tabIndex)
		}

		// 立即刷新当前标签内容，确保显示最新内容
		log.Printf("正在刷新已打开的文件: %s", filePath)

		// 停止旧的查看器监控
		if oldViewer, exists := ui.viewers[filePath]; exists {
			oldViewer.StopMonitoring()
			delete(ui.viewers, filePath)
		}

		// 重新创建PlantUML查看器
		newViewer, err := plantuml.NewViewer(filePath)
		if err == nil {
			// 设置文件变化回调，自动切换到这个标签页
			newViewer.SetOnFileChanged(ui.createFileChangeCallback(filePath))

			// 存储新的查看器引用
			ui.viewers[filePath] = newViewer

			// 成功创建新查看器，替换现有内容
			newContent := container.NewScroll(newViewer.GetCanvas())
			ui.Tabs.Items[tabIndex].Content = newContent
			ui.Tabs.Refresh() // 刷新整个标签容器
			log.Printf("已成功刷新标签内容: %s", filePath)
		}

		return
	}

	// 创建PlantUML查看器
	viewer, err := plantuml.NewViewer(filePath)
	if err != nil {
		log.Printf("无法创建PlantUML查看器: %v", err)
		return
	}

	// 设置文件变化回调，自动切换到这个标签页
	viewer.SetOnFileChanged(ui.createFileChangeCallback(filePath))

	// 存储查看器引用
	ui.viewers[filePath] = viewer

	// 创建标签项
	fileName := filepath.Base(filePath)
	// 截断过长的文件名
	displayName := truncateFileName(fileName, 30) // 最多显示30个字符

	// 创建标签内容
	content := container.NewScroll(viewer.GetCanvas())

	// 添加新标签
	tab := container.NewTabItem(displayName, content)
	ui.Tabs.Append(tab)

	// 记录文件路径和对应的tab索引
	ui.OpenedFiles[filePath] = len(ui.Tabs.Items) - 1

	// 如果需要，选择新标签并更新窗口标题
	if selectTab {
		ui.Tabs.SelectIndex(len(ui.Tabs.Items) - 1)
		ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", fileName))
	}
}

// RefreshCurrentTab 刷新当前选中的标签页
// 即使不再支持F5刷新，我们保留此方法，以便需要时可以通过程序逻辑刷新
func (ui *MainUI) RefreshCurrentTab() {
	if ui.Tabs == nil || len(ui.Tabs.Items) == 0 {
		return
	}

	currentIndex := ui.Tabs.SelectedIndex()
	// 查找当前选中的文件路径
	var currentFilePath string
	for path, index := range ui.OpenedFiles {
		if index == currentIndex {
			currentFilePath = path
			break
		}
	}

	// 直接调用OpenFile方法来刷新内容
	if currentFilePath != "" {
		log.Printf("自动刷新当前标签页文件: %s", currentFilePath)
		ui.OpenFile(currentFilePath)
	}
}

// NextTab 切换到下一个标签页
func (ui *MainUI) NextTab() {
	if ui.Tabs == nil || len(ui.Tabs.Items) <= 1 {
		return
	}

	currentIndex := ui.Tabs.SelectedIndex()
	nextIndex := (currentIndex + 1) % len(ui.Tabs.Items)
	ui.Tabs.SelectIndex(nextIndex)
}

// PrevTab 切换到上一个标签页
func (ui *MainUI) PrevTab() {
	if ui.Tabs == nil || len(ui.Tabs.Items) <= 1 {
		return
	}

	currentIndex := ui.Tabs.SelectedIndex()
	prevIndex := (currentIndex - 1 + len(ui.Tabs.Items)) % len(ui.Tabs.Items)
	ui.Tabs.SelectIndex(prevIndex)
}

// GetContent 返回UI内容
func (ui *MainUI) GetContent() fyne.CanvasObject {
	return ui.InitializeUI()
}

// StopAllMonitoring 停止所有文件监控
func (ui *MainUI) StopAllMonitoring() {
	log.Println("停止所有文件监控...")
	for path, viewer := range ui.viewers {
		log.Printf("停止对文件 %s 的监控", path)
		viewer.StopMonitoring()
	}
	// 清空查看器映射
	ui.viewers = make(map[string]*plantuml.Viewer)
}

// CloseCurrentTab 关闭当前选中的标签页
func (ui *MainUI) CloseCurrentTab() {
	if ui.Tabs == nil || len(ui.Tabs.Items) == 0 {
		return
	}

	currentIndex := ui.Tabs.SelectedIndex()
	if currentIndex < 0 || currentIndex >= len(ui.Tabs.Items) {
		return
	}

	// 让标签容器处理关闭逻辑（会触发OnClosed回调）
	ui.Tabs.RemoveIndex(currentIndex)
}
