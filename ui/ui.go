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
	Tabs        *container.DocTabs // 导出字段以便可以从外部访问
	OpenedFiles map[string]int     // 导出字段以便可以从外部访问
}

// NewMainUI 创建新的UI实例
func NewMainUI(window fyne.Window, files []string) (*MainUI, error) {
	ui := &MainUI{
		window:      window,
		files:       files,
		OpenedFiles: make(map[string]int),
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

	// 直接返回tabs容器作为主布局
	return ui.Tabs
}

// OpenFile 打开文件并创建新标签页，如果文件已打开则切换到对应标签页
func (ui *MainUI) OpenFile(filePath string) {
	// 获取绝对路径
	absPath, err := filepath.Abs(filePath)
	if err == nil {
		filePath = absPath
	}

	// 检查文件是否已打开
	if tabIndex, exists := ui.OpenedFiles[filePath]; exists {
		// 文件已经打开，切换到对应标签
		ui.Tabs.SelectIndex(tabIndex)

		// 立即刷新当前标签内容，确保显示最新内容
		log.Printf("正在刷新已打开的文件: %s", filePath)

		// 重新创建PlantUML查看器
		newViewer, err := plantuml.NewViewer(filePath)
		if err == nil {
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

	// 创建标签项
	fileName := filepath.Base(filePath)

	// 创建标签内容
	content := container.NewScroll(viewer.GetCanvas())

	// 添加新标签
	tab := container.NewTabItem(fileName, content)
	ui.Tabs.Append(tab)

	// 记录文件路径和对应的tab索引
	ui.OpenedFiles[filePath] = len(ui.Tabs.Items) - 1

	// 选择新标签
	ui.Tabs.SelectIndex(len(ui.Tabs.Items) - 1)

	// 更新窗口标题
	ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", fileName))

	// 监听标签关闭事件，从OpenedFiles中移除
	ui.Tabs.OnClosed = func(item *container.TabItem) {
		// 查找并移除关闭的文件
		for path, index := range ui.OpenedFiles {
			if filepath.Base(path) == item.Text {
				delete(ui.OpenedFiles, path)
				// 更新其他文件的索引
				for otherPath, otherIndex := range ui.OpenedFiles {
					if otherIndex > index {
						ui.OpenedFiles[otherPath] = otherIndex - 1
					}
				}
				break
			}
		}
	}

	// 监听标签选择事件，更新窗口标题
	ui.Tabs.OnSelected = func(item *container.TabItem) {
		ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", item.Text))
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
