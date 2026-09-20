package runner

import (
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// 真实子进程冒烟测试（Windows）：启动 → 收集输出 → 退出事件。
func TestRunnerRunAndExit(t *testing.T) {
	r := NewRunner()
	if r.Running() {
		t.Fatal("初始状态不应在运行")
	}

	var lines []string
	exits := make(chan ExitEvent, 1)
	err := r.Start("cmd.exe", []string{"/c", "echo hello-tdl"}, func(ev Event) {
		switch e := ev.(type) {
		case LineEvent:
			lines = append(lines, e.Text)
		case ExitEvent:
			exits <- e
		}
	})
	if err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	if !r.Running() {
		t.Fatal("启动后应处于运行状态")
	}

	select {
	case e := <-exits:
		if e.Code != 0 {
			t.Fatalf("退出码应为 0，实际 %d（err=%v）", e.Code, e.Err)
		}
		if e.Aborted {
			t.Fatal("未中止的任务 Aborted 应为 false")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("等待退出事件超时")
	}

	if r.Running() {
		t.Fatal("退出后不应处于运行状态")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "hello-tdl") {
		t.Fatalf("输出未捕获到 hello-tdl：%q", joined)
	}
}

// 中止测试：长任务被 Abort 后必须尽快结束（taskkill /T /F）。
func TestRunnerAbort(t *testing.T) {
	r := NewRunner()
	exits := make(chan ExitEvent, 1)
	err := r.Start("cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 > nul"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	})
	if err != nil {
		t.Fatalf("启动失败：%v", err)
	}

	time.Sleep(300 * time.Millisecond)
	start := time.Now()
	r.Abort()

	select {
	case e := <-exits:
		if !e.Aborted {
			t.Fatalf("Aborted 应为 true，实际 %+v", e)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("中止耗时过长：%v", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("中止后等待退出事件超时")
	}

	if r.Running() {
		t.Fatal("中止后不应处于运行状态")
	}
}

// 重复 Abort 必须幂等，不 panic、不阻塞。
func TestRunnerAbortIdempotent(t *testing.T) {
	r := NewRunner()
	exits := make(chan ExitEvent, 1)
	if err := r.Start("cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 > nul"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	}); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	r.Abort()
	r.Abort()
	r.Abort()

	select {
	case <-exits:
	case <-time.After(10 * time.Second):
		t.Fatal("等待退出事件超时")
	}
}

// 空转 Abort（无任务）不得 panic。
func TestRunnerAbortWhenIdle(t *testing.T) {
	r := NewRunner()
	r.Abort()
}

// 启动不存在的可执行文件必须返回错误且不占用运行态。
func TestRunnerStartInvalidPath(t *testing.T) {
	r := NewRunner()
	err := r.Start(`C:\this\does\not\exist.exe`, nil, func(Event) {})
	if err == nil {
		t.Fatal("不存在的路径应返回错误")
	}
	if r.Running() {
		t.Fatal("启动失败后不应处于运行状态")
	}
}

// ---------- D-15：中止结果上报与失败可重试 ----------

type killResult struct {
	ok     bool
	pid    int
	detail string
}

// stubKillTree 注入假 killTree 并在测试结束后恢复真实实现。
func stubKillTree(t *testing.T, f func(pid int) (bool, string)) {
	t.Helper()
	orig := killTree
	killTree = f
	t.Cleanup(func() { killTree = orig })
}

// 真实 killTree（绝对路径 taskkill /T /F）必须能强杀真实进程。
func TestKillTreeRealProcess(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping -n 30 127.0.0.1 > nul")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动桩进程失败：%v", err)
	}
	ok, detail := killTree(cmd.Process.Pid)
	if !ok {
		t.Fatalf("taskkill 应成功强杀真实进程，detail=%q", detail)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("进程应被强制终止（退出码非 0）")
	}
}

// 中止成功：结果回调上报 ok=true 与有效 PID，退出事件标注 Aborted。
func TestRunnerAbortReportsSuccess(t *testing.T) {
	r := NewRunner()
	res := make(chan killResult, 1)
	r.SetOnKillResult(func(ok bool, pid int, detail string) { res <- killResult{ok, pid, detail} })

	exits := make(chan ExitEvent, 1)
	if err := r.Start("cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 > nul"}, func(ev Event) {
		if e, ok := ev.(ExitEvent); ok {
			exits <- e
		}
	}); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	time.Sleep(200 * time.Millisecond)
	r.Abort()

	select {
	case kr := <-res:
		if !kr.ok {
			t.Fatalf("中止应成功并上报 ok=true，detail=%q", kr.detail)
		}
		if kr.pid <= 0 {
			t.Fatalf("应上报有效 PID，实际 %d", kr.pid)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("未收到中止结果回调")
	}

	select {
	case e := <-exits:
		if !e.Aborted {
			t.Fatalf("Aborted 应为 true：%+v", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("等待退出事件超时")
	}
}

// taskkill 与兜底均失败时：上报失败诊断，且失败后可重试（killing 复位）。
func TestRunnerAbortFailureAndRetry(t *testing.T) {
	r := NewRunner()
	var calls atomic.Int32
	stubKillTree(t, func(pid int) (bool, string) {
		calls.Add(1)
		return false, "stub：拒绝访问"
	})

	// 用"已退出的进程"作为 cmd 桩：兜底 Process.Kill() 必然失败。
	stubCmd := exec.Command("cmd.exe", "/c", "exit")
	if err := stubCmd.Start(); err != nil {
		t.Fatalf("启动桩进程失败：%v", err)
	}
	_ = stubCmd.Wait()

	r.running.Store(true) // 白盒模拟：任务仍在运行
	r.cmd = stubCmd
	r.killDone = make(chan struct{})

	res := make(chan killResult, 2)
	r.SetOnKillResult(func(ok bool, pid int, detail string) { res <- killResult{ok, pid, detail} })

	r.Abort()
	r.dispatchKillResult() // 回调由退出路径统一派发（与 ExitEvent 同序）
	select {
	case kr := <-res:
		if kr.ok {
			t.Fatal("taskkill 与兜底均失败时应上报 ok=false")
		}
		if !strings.Contains(kr.detail, "stub：拒绝访问") {
			t.Fatalf("detail 应包含 taskkill 诊断输出，实际 %q", kr.detail)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("未收到中止结果回调")
	}

	// killing 已复位：再次 Abort 必须重新执行终止动作（重试）。
	// 动作在 goroutine 中异步执行，轮询等待其被调用。
	r.Abort()
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("killTree 应被调用 2 次（首次 + 重试），实际 %d", got)
	}
}
