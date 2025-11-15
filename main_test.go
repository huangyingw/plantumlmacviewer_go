package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestIsAppRunning_NoLockFile 测试锁文件不存在的情况
func TestIsAppRunning_NoLockFile(t *testing.T) {
	// 清理可能存在的锁文件
	os.Remove(lockFile)
	os.Remove(ipcAddr)

	// 调用函数
	result := isAppRunning()

	// 验证：没有锁文件时应该返回false
	if result {
		t.Error("期望返回 false（程序未运行），但返回了 true")
	}
}

// TestCreateAndRemoveLockFile 测试锁文件的创建和删除
func TestCreateAndRemoveLockFile(t *testing.T) {
	// 清理可能存在的锁文件
	os.Remove(lockFile)

	// 创建锁文件
	createLockFile()

	// 验证锁文件存在
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Error("锁文件应该存在，但未找到")
	}

	// 验证可以读取PID
	file, err := os.Open(lockFile)
	if err != nil {
		t.Errorf("无法打开锁文件: %v", err)
	} else {
		buf := make([]byte, 32)
		n, _ := file.Read(buf)
		file.Close()

		if n == 0 {
			t.Error("锁文件应该包含PID，但为空")
		}

		var pid int
		_, err := fmt.Sscanf(string(buf[:n]), "%d", &pid)
		if err != nil {
			t.Errorf("无法解析PID: %v", err)
		}

		if pid != os.Getpid() {
			t.Errorf("期望PID=%d，但得到%d", os.Getpid(), pid)
		}
	}

	// 删除锁文件
	removeLockFile()

	// 验证锁文件已删除
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("锁文件应该已删除，但仍然存在")
	}
}

// TestValidateFiles 测试文件验证功能
func TestValidateFiles(t *testing.T) {
	// 创建临时测试文件
	tmpFile, err := os.CreateTemp("", "test*.puml")
	if err != nil {
		t.Fatalf("无法创建临时文件: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// 测试有效文件
	validFiles := validateFiles([]string{tmpFile.Name()})
	if len(validFiles) != 1 {
		t.Errorf("期望1个有效文件，但得到%d个", len(validFiles))
	}

	// 测试不存在的文件
	invalidFiles := validateFiles([]string{"/nonexistent/file.puml"})
	if len(invalidFiles) != 0 {
		t.Errorf("期望0个有效文件，但得到%d个", len(invalidFiles))
	}

	// 测试目录（应该被过滤）
	tmpDir, err := os.MkdirTemp("", "testdir")
	if err != nil {
		t.Fatalf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dirFiles := validateFiles([]string{tmpDir})
	if len(dirFiles) != 0 {
		t.Errorf("期望目录被过滤掉，但得到%d个文件", len(dirFiles))
	}
}

// TestIsAppRunning_WithZombieLock 测试僵尸锁文件的清理
func TestIsAppRunning_WithZombieLock(t *testing.T) {
	// 清理环境
	os.Remove(lockFile)
	os.Remove(ipcAddr)

	// 创建一个僵尸锁文件（使用不存在的PID）
	zombiePID := 99999 // 假设这个PID不存在
	file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("无法创建测试锁文件: %v", err)
	}
	file.WriteString(string(rune(zombiePID)))
	file.Close()

	// 等待一小段时间确保文件写入完成
	time.Sleep(100 * time.Millisecond)

	// 调用isAppRunning - 应该检测到僵尸锁并清理
	result := isAppRunning()

	// 验证：僵尸锁应该被清理，返回false
	if result {
		t.Error("期望检测到僵尸锁并返回false，但返回了true")
	}

	// 验证锁文件已被清理
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("僵尸锁文件应该被清理，但仍然存在")
	}

	// 清理
	os.Remove(lockFile)
	os.Remove(ipcAddr)
}
