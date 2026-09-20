package runner

import (
	"testing"
	"time"
)

// 终端窗口模式：子进程必须能真正读取控制台输入（tdl login 的交互选择依赖这一点）。
// 判定方法：`cmd /c pause` 在有控制台标准输入时会一直等待按键；若标准输入是 NUL 则立即退出。
func TestStartInConsoleWaitsForInput(t *testing.T) {
	r := NewRunner()
	exits := make(chan ExitEvent, 1)
	if err := r.StartInConsole("cmd.exe", []string{"/c", "pause"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	}); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	if !r.Running() {
		t.Fatal("终端窗口模式下 Running 应为 true")
	}

	select {
	case e := <-exits:
		t.Fatalf("子进程未能等待控制台输入（标准输入不是控制台）：%+v", e)
	case <-time.After(2 * time.Second):
		// 正确：仍在等待按键
	}

	start := time.Now()
	r.Abort()
	select {
	case e := <-exits:
		if !e.Aborted {
			t.Fatalf("Aborted 应为 true：%+v", e)
		}
		if elapsed := time.Since(start); elapsed > 8*time.Second {
			t.Fatalf("中止耗时过长：%v", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("中止后等待退出事件超时")
	}
	if r.Running() {
		t.Fatal("退出后不应处于运行状态")
	}
}

// 对照：普通（管道）模式下标准输入是 NUL，pause 会立即返回。
func TestPipeModeHasNullStdin(t *testing.T) {
	r := NewRunner()
	exits := make(chan ExitEvent, 1)
	if err := r.Start("cmd.exe", []string{"/c", "pause"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	}); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	select {
	case <-exits:
		// 正确：无控制台输入，立即退出
	case <-time.After(5 * time.Second):
		t.Fatal("管道模式不应等待控制台输入")
		_ = r
	}
}

// 终端窗口模式下退出码必须回传。
func TestStartInConsoleExitCode(t *testing.T) {
	r := NewRunner()
	exits := make(chan ExitEvent, 1)
	if err := r.StartInConsole("cmd.exe", []string{"/c", "exit 7"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	}); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	select {
	case e := <-exits:
		if e.Code != 7 {
			t.Fatalf("退出码应为 7，实际 %d（err=%v）", e.Code, e.Err)
		}
		if e.Aborted {
			t.Fatal("未中止的任务 Aborted 应为 false")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("等待退出事件超时")
	}
}

// 终端窗口模式下启动不存在的程序必须返回错误且不占运行态。
func TestStartInConsoleInvalidPath(t *testing.T) {
	r := NewRunner()
	if err := r.StartInConsole(`C:\this\does\not\exist.exe`, nil, func(Event) {}); err == nil {
		t.Fatal("不存在的路径应返回错误")
	}
	if r.Running() {
		t.Fatal("启动失败后不应处于运行状态")
	}
}

// 终端窗口模式同样受"同一时刻仅一个任务"约束。
func TestStartInConsoleSingleTask(t *testing.T) {
	r := NewRunner()
	exits := make(chan ExitEvent, 1)
	if err := r.StartInConsole("cmd.exe", []string{"/c", "pause"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	}); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	if err := r.StartInConsole("cmd.exe", []string{"/c", "exit 0"}, func(Event) {}); err == nil {
		t.Fatal("已有任务运行时再次启动应报错")
	}
	r.Abort()
	select {
	case <-exits:
	case <-time.After(10 * time.Second):
		t.Fatal("等待退出事件超时")
	}
}
