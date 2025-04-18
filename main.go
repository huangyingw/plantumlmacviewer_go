package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"plantumlmacviewer/ui"
)

var (
	version = "0.1.0"
)

// 全局变量来存储应用程序实例
var fyneApp fyne.App
var mainWindow fyne.Window
var mainUI *ui.MainUI

// 单实例锁文件路径
const lockFile = "/tmp/plantumlviewer.lock"

// IPC服务器地址
const ipcAddr = "/tmp/plantumlviewer.sock"

// 全局变量保存锁文件句柄
var lockFileHandle *os.File

func main() {
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
	mainWindow.Resize(fyne.NewSize(800, 600))

	// 设置窗口关闭事件
	mainWindow.SetCloseIntercept(func() {
		// 关闭窗口时，移除锁文件并退出
		removeLockFile()
		mainWindow.Close()
	})

	// 如果没有文件参数，显示提示信息
	if len(validFiles) == 0 {
		log.Println("没有指定要打开的文件，请通过命令行参数提供PUML文件路径")
		mainWindow.SetTitle("PlantUML Viewer - 未加载文件")
	}

	// 初始化UI并设置到窗口
	mainUI, _ = ui.NewMainUI(mainWindow, validFiles)
	content := mainUI.GetContent()
	mainWindow.SetContent(content)

	// 添加键盘快捷键
	setupShortcuts()

	// 显示窗口并运行应用
	mainWindow.ShowAndRun()
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

// handleIPCConnection 处理IPC连接
func handleIPCConnection(conn net.Conn) {
	defer conn.Close()
	log.Println("处理IPC连接...")

	// 读取文件列表
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("读取数据出错：%v", err)
		return
	}

	log.Printf("收到数据: %d 字节", n)

	// 解析文件列表
	fileList := strings.Split(string(buf[:n]), "\n")
	log.Printf("解析文件列表: %v", fileList)

	validFiles := validateFiles(fileList)
	log.Printf("有效文件列表: %v", validFiles)

	// 在UI线程中打开文件
	if len(validFiles) > 0 {
		fyne.Do(func() {
			for _, file := range validFiles {
				log.Printf("尝试打开文件: %s", file)
				// 使用现有UI打开文件
				if mainUI != nil {
					mainUI.OpenFile(file)
				}
			}
			// 将窗口置于前台
			mainWindow.Show()
			log.Println("窗口已置于前台")
		})
	}

	// 发送确认信息
	conn.Write([]byte("OK"))
}

// sendFilesToRunningInstance 将文件列表发送到正在运行的实例
func sendFilesToRunningInstance(files []string) {
	if len(files) == 0 {
		return
	}

	log.Printf("发送文件列表到运行中的实例: %v", files)

	// 连接到IPC服务器
	conn, err := net.Dial("unix", ipcAddr)
	if err != nil {
		log.Printf("无法连接到运行中的实例：%v", err)
		return
	}
	defer conn.Close()

	log.Println("已连接到IPC服务器")

	// 发送文件列表
	fileList := strings.Join(files, "\n")
	_, err = conn.Write([]byte(fileList))
	if err != nil {
		log.Printf("发送文件列表失败：%v", err)
		return
	}

	log.Println("文件列表已发送")

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
	// 使用简单的按键切换，先支持基本功能
	mainWindow.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
		if ke.Name == fyne.KeyTab {
			log.Println("按下Tab键，切换到下一个标签页")
			if mainUI != nil {
				mainUI.NextTab()
			}
		}

		// 为PageDown键绑定下一个标签页功能
		if ke.Name == fyne.KeyPageDown {
			log.Println("按下PageDown键，切换到下一个标签页")
			if mainUI != nil {
				mainUI.NextTab()
			}
		}

		// 为PageUp键绑定上一个标签页功能
		if ke.Name == fyne.KeyPageUp {
			log.Println("按下PageUp键，切换到上一个标签页")
			if mainUI != nil {
				mainUI.PrevTab()
			}
		}
	})
}

// 应用程序图标资源（需要添加实际的图标数据）
func resourceIconPng() fyne.Resource {
	// 在实际应用中，这里应该返回一个真正的图标资源
	// 简化处理，返回空资源
	return fyne.NewStaticResource("icon.png", []byte{})
}
