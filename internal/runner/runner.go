// Package runner 负责 tdl 子进程的生命周期：启动、输出流解析、强制中止。
//
// 该包不依赖 fyne；onEvent 由调用方保证在 UI 线程语义下执行（调用方自行 fyne.Do）。
package runner

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// CREATE_NO_WINDOW：GUI 进程无控制台，避免子进程闪现黑窗。
const createNoWindow = 0x08000000

// Event 是 Runner 派发的事件（LineEvent / ProgressEvent / ExitEvent）。
type Event interface{}

// LineEvent 表示以 \n 结束的完整行。
type LineEvent struct{ Text string }

// ProgressEvent 表示以 \r 触发的行内刷新。
type ProgressEvent struct{ Text string }

// ExitEvent 表示进程退出。
type ExitEvent struct {
	Code    int
	Err     error
	Aborted bool
}

// Runner 管理单个子进程（同一时刻仅允许一个任务）。
type Runner struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	cpid    *consoleProc
	running atomic.Bool
	aborted atomic.Bool
	killing atomic.Bool // 杀进程动作进行中（去重；完成后复位，失败可重试）

	// 中止结果：由杀进程 goroutine 写入，由退出路径（dispatchKillResult）统一派发，
	// 保证回调与 ExitEvent 同序、同 goroutine，避免与退出处理并发写日志（D-15）。
	killDone       chan struct{}
	killDoneClosed bool
	killOK         bool
	killPID        int
	killDetail     string

	onKillResult func(ok bool, pid int, detail string)
}

// NewRunner 创建 Runner。
func NewRunner() *Runner { return &Runner{} }

// Running 报告当前是否有子进程在运行。
func (r *Runner) Running() bool { return r.running.Load() }

// Start 启动子进程并异步派发事件。失败（如路径无效）时立即返回错误且不占用运行态。
func (r *Runner) Start(path string, args []string, onEvent func(Event)) error {
	if !r.running.CompareAndSwap(false, true) {
		return errors.New("已有任务正在运行")
	}

	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}

	parser := NewParser(func(text string, progress bool) {
		if progress {
			onEvent(ProgressEvent{Text: text})
			return
		}
		onEvent(LineEvent{Text: text})
	})
	// stdout 与 stderr 指向同一 Writer：os/exec 内部只用一个管道与 goroutine，
	// 且 Wait 会等待全部输出写完，保证退出事件不丢尾部输出。
	cmd.Stdout = parser
	cmd.Stderr = parser

	if err := cmd.Start(); err != nil {
		r.running.Store(false)
		return err
	}

	r.mu.Lock()
	r.cmd = cmd
	r.aborted.Store(false)
	r.killDone = make(chan struct{})
	r.killDoneClosed = false
	r.mu.Unlock()

	go r.wait(cmd, parser, onEvent)
	return nil
}

func (r *Runner) wait(cmd *exec.Cmd, parser *Parser, onEvent func(Event)) {
	err := cmd.Wait()
	parser.Flush()

	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	aborted := r.aborted.Load()

	r.mu.Lock()
	r.cmd = nil
	r.mu.Unlock()
	r.running.Store(false)

	r.dispatchKillResult() // 若有中止：与 ExitEvent 同序派发其结果
	onEvent(ExitEvent{Code: code, Err: err, Aborted: aborted})
}

// StartInConsole 在新建的终端窗口中启动子进程（用于需要交互输入的命令，如 tdl login）。
//
// 子进程的输入输出直接与新终端窗口交互，不经过本进程管道；退出码仍会回传，
// 按钮变形/全局锁定/中止等任务语义与 Start 完全一致。
func (r *Runner) StartInConsole(path string, args []string, onEvent func(Event)) error {
	if !r.running.CompareAndSwap(false, true) {
		return errors.New("已有任务正在运行")
	}

	cp, err := startInNewConsole(path, args)
	if err != nil {
		r.running.Store(false)
		return err
	}

	r.mu.Lock()
	r.cpid = cp
	r.aborted.Store(false)
	r.killDone = make(chan struct{})
	r.killDoneClosed = false
	r.mu.Unlock()

	go func() {
		code := cp.wait()

		r.mu.Lock()
		r.cpid = nil
		r.mu.Unlock()
		r.running.Store(false)

		r.dispatchKillResult()
		onEvent(ExitEvent{Code: code, Aborted: r.aborted.Load()})
	}()
	return nil
}

// SetOnKillResult 注册"中止动作结果"回调。
//
// 回调由退出路径（wait，在 ExitEvent 之前）统一派发，与退出事件严格同序，
// 不会与退出处理（解锁/按钮恢复）并发；回调可能不在 UI 线程执行，
// 调用方自行派发（TaskController 已用 uiDo 包裹）。
func (r *Runner) SetOnKillResult(f func(ok bool, pid int, detail string)) {
	r.mu.Lock()
	r.onKillResult = f
	r.mu.Unlock()
}

// Abort 强制结束整棵进程树。
//
// 语义：
//   - aborted 记录"用户请求过中止"（用于 ExitEvent 标注），不拦截重复请求；
//   - killing 对杀进程动作去重（同一时刻只跑一个 taskkill），动作完成后复位：
//     失败时用户可再次点击 [中止] 重试（D-15：修复"首次失败后所有后续中止全部空转"）；
//   - 结果经 SetOnKillResult 回调上报（成功/失败与 taskkill 诊断输出），不再静默。
func (r *Runner) Abort() {
	if !r.running.Load() {
		return
	}
	r.aborted.Store(true)
	if !r.killing.CompareAndSwap(false, true) {
		return // 已有终止动作在进行，等待其结果（失败后可重试）
	}

	r.mu.Lock()
	cmd := r.cmd
	cp := r.cpid
	kd := r.killDone
	r.mu.Unlock()

	pid := 0
	switch {
	case cmd != nil && cmd.Process != nil:
		pid = cmd.Process.Pid
	case cp != nil:
		pid = cp.pid
	default:
		r.killing.Store(false)
		return
	}

	go func() {
		ok, detail := killTree(pid)
		if !ok {
			// 兜底：taskkill 不可用/失败时退回直接强杀
			switch {
			case cmd != nil && cmd.Process != nil:
				if kerr := cmd.Process.Kill(); kerr == nil {
					ok, detail = true, ""
				} else {
					detail = fmt.Sprintf("%s；兜底强杀失败：%v", detail, kerr)
				}
			case cp != nil:
				if kerr := cp.kill(); kerr == nil {
					ok, detail = true, ""
				} else {
					detail = fmt.Sprintf("%s；兜底强杀失败：%v", detail, kerr)
				}
			}
		}
		// 写入结果并通知退出路径（回调在 dispatchKillResult 中派发）。
		// 注意：killing 必须在 close 之前复位——退出路径收到通知即代表
		// "本次终止动作已彻底结束"，此时允许失败后重试（D-15）。
		r.mu.Lock()
		r.killOK, r.killPID, r.killDetail = ok, pid, detail
		r.killing.Store(false)
		if kd != nil && !r.killDoneClosed {
			r.killDoneClosed = true
			close(kd)
		}
		r.mu.Unlock()
	}()
}

// killWaitTimeout 退出路径等待中止动作完成的超时（taskkill 卡死时的兜底）。
const killWaitTimeout = 5 * time.Second

// dispatchKillResult 在 ExitEvent 之前派发中止结果回调（仅当本次任务发起过中止）。
//
// 统一由退出路径调用：保证回调与 ExitEvent 同序、同 goroutine，
// 否则杀进程 goroutine 的日志写入会与退出处理（finish/解锁）并发（D-15）。
func (r *Runner) dispatchKillResult() {
	if !r.aborted.Load() {
		return
	}
	r.mu.Lock()
	d, cb := r.killDone, r.onKillResult
	r.mu.Unlock()
	if d == nil || cb == nil {
		return
	}
	select {
	case <-d:
		r.mu.Lock()
		ok, pid, detail := r.killOK, r.killPID, r.killDetail
		r.mu.Unlock()
		cb(ok, pid, detail)
	case <-time.After(killWaitTimeout):
		// 极端故障（taskkill 卡死）：放弃等待，结果不再上报（看门狗会提示重试）
	}
}

// killTree 强制结束指定 PID 及其进程树（taskkill /T /F），返回（是否成功, 诊断信息）。
//
// 包级变量以便单测注入假实现。要点：
//   - 使用绝对路径（%SystemRoot%\System32\taskkill.exe），摆脱 PATH 依赖；
//   - 捕获 taskkill 输出用于失败诊断（不再静默丢弃）；
//   - 退出码 128（没有找到进程）视为成功（进程已退出）。
var killTree = func(pid int) (bool, string) {
	exe := "taskkill"
	if root := os.Getenv("SystemRoot"); root != "" {
		if p := filepath.Join(root, "System32", "taskkill.exe"); fileExists(p) {
			exe = p
		}
	}
	var out bytes.Buffer
	kill := exec.Command(exe, "/T", "/F", "/PID", strconv.Itoa(pid))
	kill.Stdout = &out
	kill.Stderr = &out
	kill.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}

	err := kill.Run()
	if err == nil {
		return true, ""
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 128 {
		return true, "进程已不存在"
	}
	msg := strings.TrimSpace(out.String())
	if msg == "" {
		msg = err.Error()
	}
	return false, msg
}

// fileExists 报告路径指向一个存在的文件。
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
