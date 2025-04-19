package ui

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"plantumlmacviewer/plantuml"
)

// MainUI 是应用程序的主UI结构
type MainUI struct {
	window      fyne.Window
	files       []string
	tabs        *container.DocTabs
	openedFiles map[string]int // 用于跟踪已打开文件及其在tabs.Items中的索引
}

// NewMainUI 创建新的UI实例
func NewMainUI(window fyne.Window, files []string) (*MainUI, error) {
	ui := &MainUI{
		window:      window,
		files:       files,
		openedFiles: make(map[string]int),
	}
	return ui, nil
}

// InitializeUI 初始化UI组件
func (ui *MainUI) InitializeUI() fyne.CanvasObject {
	// 创建Tab容器用于显示多个PUML文件
	ui.tabs = container.NewDocTabs()
	ui.tabs.SetTabLocation(container.TabLocationTop)

	// 如果有文件参数传入，立即打开它们
	for _, file := range ui.files {
		ui.OpenFile(file)
	}

	// 直接返回tabs容器作为主布局
	return ui.tabs
}

// OpenFile 打开文件并创建新标签页，如果文件已打开则切换到对应标签页
func (ui *MainUI) OpenFile(filePath string) {
	// 获取绝对路径
	absPath, err := filepath.Abs(filePath)
	if err == nil {
		filePath = absPath
	}

	// 检查文件是否已打开
	if tabIndex, exists := ui.openedFiles[filePath]; exists {
		// 文件已经打开，切换到对应标签
		ui.tabs.SelectIndex(tabIndex)
		return
	}

	// 创建PlantUML查看器
	viewer, err := plantuml.NewViewer(filePath)
	if err != nil {
		return
	}

	// 创建标签项
	fileName := filepath.Base(filePath)

	// 创建标签内容
	content := container.NewScroll(viewer.GetCanvas())

	// 添加新标签
	tab := container.NewTabItem(fileName, content)
	ui.tabs.Append(tab)

	// 记录文件路径和对应的tab索引
	ui.openedFiles[filePath] = len(ui.tabs.Items) - 1

	// 选择新标签
	ui.tabs.SelectIndex(len(ui.tabs.Items) - 1)

	// 更新窗口标题
	ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", fileName))

	// 监听标签关闭事件，从openedFiles中移除
	ui.tabs.OnClosed = func(item *container.TabItem) {
		// 查找并移除关闭的文件
		for path, index := range ui.openedFiles {
			if filepath.Base(path) == item.Text {
				delete(ui.openedFiles, path)
				// 更新其他文件的索引
				for otherPath, otherIndex := range ui.openedFiles {
					if otherIndex > index {
						ui.openedFiles[otherPath] = otherIndex - 1
					}
				}
				break
			}
		}
	}

	// 监听标签选择事件，更新窗口标题
	ui.tabs.OnSelected = func(item *container.TabItem) {
		ui.window.SetTitle(fmt.Sprintf("PlantUML Viewer - %s", item.Text))
	}
}

// NextTab 切换到下一个标签页
func (ui *MainUI) NextTab() {
	if ui.tabs == nil || len(ui.tabs.Items) <= 1 {
		return
	}

	currentIndex := ui.tabs.SelectedIndex()
	nextIndex := (currentIndex + 1) % len(ui.tabs.Items)
	ui.tabs.SelectIndex(nextIndex)
}

// PrevTab 切换到上一个标签页
func (ui *MainUI) PrevTab() {
	if ui.tabs == nil || len(ui.tabs.Items) <= 1 {
		return
	}

	currentIndex := ui.tabs.SelectedIndex()
	prevIndex := (currentIndex - 1 + len(ui.tabs.Items)) % len(ui.tabs.Items)
	ui.tabs.SelectIndex(prevIndex)
}

// GetContent 返回UI内容
func (ui *MainUI) GetContent() fyne.CanvasObject {
	return ui.InitializeUI()
}
