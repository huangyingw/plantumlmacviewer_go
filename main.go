package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	"plantumlmacviewer/ui"
)

var (
	version = "0.1.0"
)

// 常量定义
const (
	// 单实例锁文件路径
	lockFile = "/tmp/plantumlviewer.lock"
	// IPC服务器地址
	ipcAddr = "/tmp/plantumlviewer.sock"

	// 窗口初始尺寸（使用超大值，系统会自动调整到屏幕可用空间）
	initialWindowWidth  = 6000
	initialWindowHeight = 4000

	// UI 初始化等待相关
	mainUIInitMaxRetries    = 50                      // 最多重试 50 次（总计 5 秒）
	mainUIInitRetryInterval = 100 * time.Millisecond  // 每次重试间隔 100ms

	// 标签选择延迟（等待所有标签添加完成）
	tabSelectionDelay = 100 * time.Millisecond
)

// 全局变量来存储应用程序实例
var fyneApp fyne.App
var mainWindow fyne.Window
var mainUI *ui.MainUI

// 全局变量保存锁文件句柄
var lockFileHandle *os.File

func main() {
	// 设置日志输出到文件
	setupLogger()

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
		fmt.Println("\n快捷键:")
		fmt.Println("  Tab 或 PageDown: 下一个标签页")
		fmt.Println("  PageUp: 上一个标签页")
		fmt.Println("  Alt+←/→: 上一个/下一个标签页 (某些系统上)")
		os.Exit(0)
	}

	// 获取传入的文件路径参数
	files := flag.Args()

	// 验证文件路径有效性
	validFiles := validateFiles(files)
	if len(files) > 0 && len(validFiles) == 0 {
		log.Println("警告：没有找到有效的PlantUML文件")
	}

	// 检查应用程序是否已在运行
	if isAppRunning() {
		// 如果应用程序已在运行，发送文件列表给现有实例
		log.Println("检测到PlantUML Viewer已经在运行，将发送文件列表到现有实例")
		sendFilesToRunningInstance(validFiles)
		// 稍等片刻，确保文件被打开
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}

	// 如果应用程序未在运行，创建锁文件
	createLockFile()
	defer removeLockFile()

	// 启动IPC服务器来接收文件请求
	go startIPCServer()

	// 创建Fyne应用
	fyneApp = app.New()
	fyneApp.Settings().SetTheme(theme.LightTheme())

	// 创建主窗口
	mainWindow = fyneApp.NewWindow("PlantUML Viewer")

	// 设置窗口标题
	if len(validFiles) == 0 {
		log.Println("没有指定要打开的文件，请通过命令行参数提供PUML文件路径")
		mainWindow.SetTitle("PlantUML Viewer - 未加载文件")
	} else {
		mainWindow.SetTitle("PlantUML Viewer - 正在加载...")
	}

	// 设置窗口关闭事件
	mainWindow.SetCloseIntercept(func() {
		// 关闭窗口时，停止所有文件监控
		if mainUI != nil {
			mainUI.StopAllMonitoring()
		}

		// 移除锁文件并退出
		removeLockFile()
		mainWindow.Close()
	})

	// 初始化UI并设置到窗口
	mainUI, _ = ui.NewMainUI(mainWindow, validFiles)
	content := mainUI.GetContent()
	mainWindow.SetContent(content)

	// 添加键盘快捷键
	setupShortcuts()

	// 设置窗口为主窗口
	mainWindow.SetMaster()

	// 设置一个非常大的窗口尺寸，系统会自动限制到屏幕最大可用空间
	// 使用超大值（6K+ 分辨率），macOS会自动调整到屏幕可用大小（扣除菜单栏等）
	log.Printf("设置窗口大小为 %dx%d（自动适配屏幕最大可用空间）", initialWindowWidth, initialWindowHeight)
	mainWindow.Resize(fyne.NewSize(initialWindowWidth, initialWindowHeight))

	// 显示窗口
	log.Println("显示窗口")
	mainWindow.Show()

	// 居中显示
	mainWindow.CenterOnScreen()

	// 运行应用程序
	log.Println("开始运行应用")
	fyneApp.Run()
}

// setupLogger 配置日志输出到文件
func setupLogger() {
	// 获取当前执行程序所在目录
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("获取程序路径失败: %v", err)
		return
	}
	execDir := filepath.Dir(execPath)

	// 日志文件路径（放在程序所在目录下）
	logFilePath := filepath.Join(execDir, "plantumlviewer.log")

	// 创建或截断日志文件
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("无法创建日志文件: %v", err)
		return
	}

	// 设置日志同时输出到文件和标准输出
	multiWriter := io.MultiWriter(logFile, os.Stdout)
	log.SetOutput(multiWriter)

	// 设置日志前缀和标志
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 记录应用启动信息
	log.Printf("PlantUML Viewer v%s 启动", version)
	log.Printf("日志文件位置: %s", logFilePath)
}

// isAppRunning 检查应用程序是否已在运行（通过检查锁文件）
func isAppRunning() bool {
	log.Println("检查应用程序是否已在运行...")

	// 尝试打开锁文件
	file, err := os.OpenFile(lockFile, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			// 锁文件不存在，说明程序未运行
			log.Println("锁文件不存在，程序未运行")
			return false
		}
		// 其他错误，打印错误信息并假设程序未运行
		log.Printf("打开锁文件出错: %v，假设程序未运行", err)
		return false
	}

	// 尝试获取文件锁
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// 无法获取锁，说明文件已被锁定，程序已在运行
		log.Println("无法获取文件锁，程序已在运行")
		file.Close()
		return true
	}

	// 能够获取锁，但这意味着程序没有正确退出
	// 解锁并删除这个过时的锁文件
	log.Println("获取到锁，但之前程序可能未正常退出，删除旧锁文件")
	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	file.Close()
	os.Remove(lockFile)
	return false
}

// createLockFile 创建锁文件并锁定
func createLockFile() {
	log.Println("创建锁文件...")
	var err error
	lockFileHandle, err = os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Printf("警告：无法创建锁文件：%v", err)
		return
	}

	// 获取排他锁
	err = syscall.Flock(int(lockFileHandle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		log.Printf("警告：无法锁定文件：%v", err)
		lockFileHandle.Close()
		lockFileHandle = nil
		return
	}

	// 写入当前进程ID
	_, err = fmt.Fprintf(lockFileHandle, "%d", os.Getpid())
	if err != nil {
		log.Printf("警告：无法写入进程ID：%v", err)
	}

	log.Println("锁文件创建成功，进程ID已写入")
}

// removeLockFile 移除锁文件
func removeLockFile() {
	log.Println("尝试移除锁文件...")
	// 先检查文件句柄是否存在
	if lockFileHandle != nil {
		// 解锁文件
		syscall.Flock(int(lockFileHandle.Fd()), syscall.LOCK_UN)
		// 关闭文件
		lockFileHandle.Close()
		// 删除文件
		os.Remove(lockFile)
		lockFileHandle = nil
		log.Println("锁文件已成功移除")
	} else {
		log.Println("锁文件句柄为空，无需移除")
	}
}

// startIPCServer 启动IPC服务器，用于接收新实例发送的文件列表
func startIPCServer() {
	log.Println("启动IPC服务器...")
	// 确保套接字文件不存在
	os.Remove(ipcAddr)

	// 创建UNIX套接字
	listener, err := net.Listen("unix", ipcAddr)
	if err != nil {
		log.Printf("无法启动IPC服务器：%v", err)
		return
	}
	defer listener.Close()

	log.Printf("IPC服务器已启动，监听地址: %s", ipcAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("接受连接出错：%v", err)
			continue
		}

		log.Println("收到新的IPC连接")
		// 处理连接
		go handleIPCConnection(conn)
	}
}

// openFilesAndSelectLast 打开文件列表并选择最后一个标签
// 这个函数封装了重复的文件打开和标签选择逻辑
func openFilesAndSelectLast(files []string) {
	fyne.Do(func() {
		// 打开所有文件，不自动选择标签（避免触发 RequestFocus）
		for _, file := range files {
			log.Printf("尝试打开文件: %s", file)
			mainUI.OpenFileWithOptions(file, false)
		}

		log.Println("所有文件已处理完成")

		// 在所有文件打开后，选择最后一个文件的标签（在新的 fyne.Do 上下文中）
		if len(files) > 0 {
			go func() {
				// 延迟一小段时间，确保所有标签都已添加
				time.Sleep(tabSelectionDelay)
				fyne.Do(func() {
					// 在 fyne.Do 内部安全地访问 mainUI.Tabs
					if mainUI.Tabs != nil && len(mainUI.Tabs.Items) > 0 {
						lastIndex := len(mainUI.Tabs.Items) - 1
						mainUI.Tabs.SelectIndex(lastIndex)
						log.Printf("已选择最后一个标签，索引: %d", lastIndex)
					}
				})
			}()
		}
	})
}

// handleIPCConnection 处理IPC连接
// 从客户端接收文件列表，验证文件，然后在UI中打开这些文件
// 注意：此函数在 goroutine 中运行，需要使用 fyne.Do() 来操作UI
func handleIPCConnection(conn net.Conn) {
	defer conn.Close()
	log.Println("处理IPC连接...")

	// 使用带超时的读取
	err := conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		log.Printf("设置读取超时失败: %v", err)
	}

	// 读取文件列表
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("读取数据出错：%v", err)
		// 尝试发送错误信息
		conn.Write([]byte("ERROR: 读取数据失败"))
		return
	}

	log.Printf("收到数据: %d 字节", n)

	// 解析文件列表
	fileList := strings.Split(string(buf[:n]), "\n")
	log.Printf("解析文件列表: %v", fileList)

	validFiles := validateFiles(fileList)
	log.Printf("有效文件列表: %v", validFiles)

	// 发送确认信息（先发送，避免客户端超时）
	err = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err != nil {
		log.Printf("设置写入超时失败: %v", err)
	}

	_, err = conn.Write([]byte("OK"))
	if err != nil {
		log.Printf("发送确认信息失败: %v（继续处理文件）", err)
		// 不要 return，继续处理文件
	}

	// 在UI线程中打开文件（异步，避免阻塞IPC处理）
	// 注意：由于 mainUI 可能在 IPC 服务器启动后才初始化，需要处理两种情况
	if len(validFiles) > 0 {
		if mainUI == nil {
			log.Println("警告: mainUI 尚未初始化，延迟打开文件")
			// 情况1: mainUI 还未初始化，启动等待 goroutine
			go func() {
				files := validFiles // 捕获文件列表，避免闭包引用问题
				// 轮询等待 mainUI 初始化（最多等待 5 秒）
				for i := 0; i < mainUIInitMaxRetries; i++ {
					if mainUI != nil {
						break
					}
					time.Sleep(mainUIInitRetryInterval)
				}

				if mainUI == nil {
					log.Println("错误: mainUI 初始化超时，无法打开文件")
					return
				}

				log.Println("mainUI 已初始化，准备打开文件")
				openFilesAndSelectLast(files)
			}()
			return
		}

		// 情况2: mainUI 已经初始化完成，直接打开文件
		log.Println("mainUI 已就绪，直接打开文件")
		openFilesAndSelectLast(validFiles)
	}
}

// sendFilesToRunningInstance 将文件列表发送到正在运行的实例
func sendFilesToRunningInstance(files []string) {
	if len(files) == 0 {
		return
	}

	log.Printf("发送文件列表到运行中的实例: %v", files)

	// 连接到IPC服务器，添加超时
	conn, err := net.DialTimeout("unix", ipcAddr, 3*time.Second)
	if err != nil {
		log.Printf("无法连接到运行中的实例：%v", err)
		return
	}
	defer conn.Close()

	log.Println("已连接到IPC服务器")

	// 设置写入超时
	err = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err != nil {
		log.Printf("设置写入超时失败: %v", err)
	}

	// 发送文件列表
	fileList := strings.Join(files, "\n")
	_, err = conn.Write([]byte(fileList))
	if err != nil {
		log.Printf("发送文件列表失败：%v", err)
		return
	}

	log.Println("文件列表已发送")

	// 设置读取超时
	err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		log.Printf("设置读取超时失败: %v", err)
	}

	// 等待确认
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("读取确认信息失败: %v", err)
		return
	}

	log.Printf("收到确认信息: %s", string(buf[:n]))
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

		// 转换为绝对路径
		absPath, err := filepath.Abs(file)
		if err == nil {
			file = absPath
		}

		validFiles = append(validFiles, file)
	}
	return validFiles
}

// setupShortcuts 设置键盘快捷键
func setupShortcuts() {
	// 完全重新实现键盘事件处理，确保Tab键后方向键仍然有效
	canvas := mainWindow.Canvas()

	// 添加Cmd+W快捷键（关闭当前标签页）
	cmdW := &desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: desktop.SuperModifier}
	canvas.AddShortcut(cmdW, func(shortcut fyne.Shortcut) {
		log.Println("处理Cmd+W快捷键: 关闭当前标签页")
		if mainUI != nil {
			mainUI.CloseCurrentTab()
		}
	})

	// 设置一个键盘事件处理函数
	canvas.SetOnTypedKey(func(ke *fyne.KeyEvent) {
		log.Printf("接收到键盘事件: %v", ke.Name)

		// 确保mainUI已初始化
		if mainUI == nil {
			return
		}

		// 处理按键
		switch ke.Name {
		case fyne.KeyTab:
			log.Println("处理Tab键: 下一标签页")
			mainUI.NextTab()
			// 立即请求焦点回到主窗口
			mainWindow.RequestFocus()
		case fyne.KeyLeft:
			log.Println("处理左方向键: 上一标签页")
			mainUI.PrevTab()
		case fyne.KeyRight:
			log.Println("处理右方向键: 下一标签页")
			mainUI.NextTab()
		case fyne.KeyEscape:
			// ESC键退出全屏
			if mainWindow.FullScreen() {
				mainWindow.SetFullScreen(false)
			}
		case fyne.KeyF11:
			// F11切换全屏模式
			log.Println("处理F11键: 切换全屏模式")
			currentFullScreen := mainWindow.FullScreen()
			mainWindow.SetFullScreen(!currentFullScreen)

			// 如果退出全屏模式，尝试恢复到最大化尺寸
			if currentFullScreen {
				// 给UI一点时间更新
				go func() {
					time.Sleep(100 * time.Millisecond)
					fyne.Do(func() {
						// 获取当前Canvas尺寸
						canvasSize := mainWindow.Canvas().Size()
						// 调整为接近最大尺寸
						mainWindow.Resize(fyne.NewSize(canvasSize.Width*0.99, canvasSize.Height*0.99))
					})
				}()
			}
		case fyne.KeyF10:
			// F10进入窗口最大化模式（非全屏）
			log.Println("处理F10键: 最大化窗口")

			// 确保不是全屏模式
			if mainWindow.FullScreen() {
				mainWindow.SetFullScreen(false)
			}

			// 获取当前Canvas尺寸
			canvasSize := mainWindow.Canvas().Size()
			if canvasSize.Width > 100 {
				// 获取到有效尺寸，设置为接近最大尺寸（不是完全最大，避免遮挡系统UI）
				effectiveWidth := canvasSize.Width * 0.95
				effectiveHeight := canvasSize.Height * 0.95
				log.Printf("设置窗口尺寸为: %.2f x %.2f", effectiveWidth, effectiveHeight)
				mainWindow.Resize(fyne.NewSize(effectiveWidth, effectiveHeight))
			} else {
				// 使用固定大尺寸
				log.Println("使用默认大尺寸 1200x800")
				mainWindow.Resize(fyne.NewSize(1200, 800))
			}
		}
	})

	// 设置窗口获取焦点事件
	mainWindow.SetOnClosed(func() {
		removeLockFile()
	})

	// 确保窗口始终获取焦点
	mainWindow.RequestFocus()

	// 修复Tab键后焦点问题 - 监听窗口获取焦点的事件
	mainWindow.Canvas().SetOnTypedRune(func(r rune) {
		// 在任何字符输入后重新请求焦点，这有助于保持键盘事件的响应
		mainWindow.RequestFocus()
	})
}

// 应用程序图标资源（需要添加实际的图标数据）
func resourceIconPng() fyne.Resource {
	// 在实际应用中，这里应该返回一个真正的图标资源
	// 简化处理，返回空资源
	return fyne.NewStaticResource("icon.png", []byte{})
}
