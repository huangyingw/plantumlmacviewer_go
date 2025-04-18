package ui

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"plantumlmacviewer/plantuml"
)

// MainUI 是应用程序的主UI结构
type MainUI struct {
	window    fyne.Window
	files     []string
	tabs      *container.DocTabs
	statusBar *widget.Label
}

// NewMainUI 创建新的UI实例
func NewMainUI(window fyne.Window, files []string) (*MainUI, error) {
	ui := &MainUI{
		window:    window,
		files:     files,
		statusBar: widget.NewLabel("就绪"),
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
		ui.openFile(file)
	}

	// 创建状态栏
	statusContainer := container.NewHBox(
		widget.NewLabel("状态:"),
		ui.statusBar,
	)

	// 创建主布局
	mainLayout := container.NewBorder(
		nil,
		statusContainer,
		nil, nil,
		ui.tabs,
	)

	return mainLayout
}

// 打开文件并创建新标签页
func (ui *MainUI) openFile(filePath string) {
	ui.statusBar.SetText(fmt.Sprintf("正在打开: %s", filepath.Base(filePath)))

	// 创建PlantUML查看器
	viewer, err := plantuml.NewViewer(filePath)
	if err != nil {
		ui.statusBar.SetText(fmt.Sprintf("无法打开文件: %v", err))
		return
	}

	// 创建标签项
	fileName := filepath.Base(filePath)

	// 检查是否已有相同标签
	for i, item := range ui.tabs.Items {
		if item.Text == fileName {
			// 已有标签，切换到它
			ui.tabs.SelectIndex(i)
			ui.statusBar.SetText(fmt.Sprintf("已打开: %s", fileName))
			return
		}
	}

	// 创建标签内容
	content := container.NewScroll(viewer.GetCanvas())

	// 添加新标签
	tab := container.NewTabItem(fileName, content)
	ui.tabs.Append(tab)

	// 选择新标签
	ui.tabs.SelectIndex(len(ui.tabs.Items) - 1)

	ui.statusBar.SetText(fmt.Sprintf("已打开: %s", fileName))
}

// GetContent 返回UI内容
func (ui *MainUI) GetContent() fyne.CanvasObject {
	return ui.InitializeUI()
}
