package ui

import (
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"tdl-gui/internal/runner"
)

// abortWatchdogDelay 中止看门狗：发起终止动作后该时长内进程仍未退出则告警。
const abortWatchdogDelay = 5 * time.Second

// progressLineRe 进度行特征（tdl 管道模式下进度刷新以整行输出：百分比/速度/ETA），
// 命中者重定向为日志框单行覆盖而非逐行追加（D-14）。
var progressLineRe = regexp.MustCompile(`\d+(?:\.\d+)?%|[\d.]+\s*(?:B|KB|MB|GB|TB)/s|~ETA[:：]`)

// progressLike 报告文本是否为 tdl 进度刷新行。
func progressLike(text string) bool { return progressLineRe.MatchString(text) }

// pinnedTelemetryRe 匹配 tdl 进度面板顶部的系统遥测行（CPU/内存/Goroutines）。
// 该行含百分比会被 progressLineRe 误判为进度行，但它不是下载进度；
// 在 onEvent 中直接丢弃，避免它每帧抢占进度行渲染（D-16：进度恒为 0 的根因之一）。
var pinnedTelemetryRe = regexp.MustCompile(`CPU:.*Memory:.*Goroutines:`)

// isPinnedTelemetry 报告文本是否为 tdl 的系统遥测行（非下载进度）。
func isPinnedTelemetry(text string) bool { return pinnedTelemetryRe.MatchString(text) }

// TaskController 是任务状态机：
//
//	点击开始 → 触发按钮立即变为红色 [中止] + 全部导航/表单控件变灰锁定
//	任务结束/被中止 → 解锁、恢复按钮、按各页当前合法性重算可交互状态
//
// 同一时刻仅允许一个任务（busy CAS 保证）。
type TaskController struct {
	win fyne.Window
	log *LogView
	run *runner.Runner

	busy       atomic.Bool
	abortAsked atomic.Bool

	lockables []fyne.Disableable
	refresh   func()

	trigger fyne.Disableable
	reset   func()
}

// NewTaskController 创建任务控制器。
func NewTaskController(win fyne.Window, log *LogView) *TaskController {
	c := &TaskController{win: win, log: log, run: runner.NewRunner()}
	// 中止结果上报：杀进程动作（含兜底）完成后写入日志，失败可重试（D-15）。
	c.run.SetOnKillResult(func(ok bool, pid int, detail string) {
		uiDo(func() {
			if ok {
				msg := fmt.Sprintf("已强制终止进程树（PID %d）", pid)
				if detail != "" {
					msg = fmt.Sprintf("已强制终止进程树（PID %d，%s）", pid, detail)
				}
				c.log.AppendBanner(msg)
				return
			}
			c.log.AppendError(fmt.Sprintf("终止失败：%s，可再次点击 [中止] 重试", detail))
		})
	})
	return c
}

// SetLockables 登记执行期间需要变灰的全部控件（导航按钮 + 各页表单控件与 [开始] 按钮）。
func (c *TaskController) SetLockables(items []fyne.Disableable) { c.lockables = items }

// SetRefresh 登记解锁后的重算回调（各页按当前合法性恢复状态）。
func (c *TaskController) SetRefresh(f func()) { c.refresh = f }

// Running 报告是否有任务在运行。
func (c *TaskController) Running() bool { return c.busy.Load() }

// Launch 启动任务：回显命令行 → 触发按钮变形为红色 [中止] → 全局锁定 → 启动子进程。
func (c *TaskController) Launch(tdlPath string, args []string, echo string, trigger *widget.Button, reset func()) {
	c.launch(tdlPath, args, echo, trigger, reset, false)
}

// LaunchConsole 与 Launch 相同，但子进程在新建的终端窗口中运行，
// 供用户完成交互式输入（如 tdl login 的账号选择）；其输出显示在该终端窗口。
func (c *TaskController) LaunchConsole(tdlPath string, args []string, echo string, trigger *widget.Button, reset func()) {
	c.launch(tdlPath, args, echo, trigger, reset, true)
}

func (c *TaskController) launch(tdlPath string, args []string, echo string, trigger *widget.Button, reset func(), inConsole bool) {
	if !c.busy.CompareAndSwap(false, true) {
		return
	}
	c.abortAsked.Store(false)
	c.log.AppendBanner(echo)

	c.trigger = trigger
	c.reset = reset
	trigger.SetText("中止")
	trigger.Importance = widget.DangerImportance
	trigger.OnTapped = func() { c.Abort() }
	trigger.Refresh()

	c.disableAll(trigger)

	var err error
	if inConsole {
		err = c.run.StartInConsole(tdlPath, args, c.onEvent)
	} else {
		err = c.run.Start(tdlPath, args, c.onEvent)
	}
	if err != nil {
		dialog.ShowError(fmt.Errorf("启动失败：%w", err), c.win)
		c.finish()
	}
}

// Abort 立即强制终止任务进程树（失败可重试）。
func (c *TaskController) Abort() {
	if !c.busy.Load() {
		return
	}
	if c.abortAsked.CompareAndSwap(false, true) {
		c.log.AppendError("========= 任务已被用户中止，正在强制终止进程树……=========")
	} else {
		c.log.AppendError("========= 再次尝试强制终止进程树……=========")
	}
	c.run.Abort()
	c.armAbortWatchdog()
}

// armAbortWatchdog 发起中止后监听：超时进程仍未退出则告警。
// 不强制解锁，维持"进程未结束不解锁"语义；告警是取证线索（D-15）。
func (c *TaskController) armAbortWatchdog() {
	time.AfterFunc(abortWatchdogDelay, func() {
		if !c.busy.Load() {
			return
		}
		uiDo(func() {
			if c.busy.Load() && c.run.Running() {
				c.log.AppendError(fmt.Sprintf(
					"警告：终止请求已发出 %v 任务仍未退出，可再次点击 [中止] 重试", abortWatchdogDelay))
			}
		})
	})
}

func (c *TaskController) onEvent(ev runner.Event) {
	switch e := ev.(type) {
	case runner.LineEvent:
		if c.abortAsked.Load() {
			return // 中止后丢弃残余输出：避免积压事件把 ExitEvent 堵在队列尾部（D-15）
		}
		if isPinnedTelemetry(e.Text) {
			return // 丢弃 tdl 系统遥测行（CPU/内存/Goroutines）：非下载进度，不进日志也不进进度行（D-16）
		}
		if progressLike(e.Text) {
			uiDo(func() { c.log.UpdateProgress(e.Text) }) // 进度刷新 → 单行就地覆盖（D-14）
			return
		}
		uiDo(func() { c.log.AppendLine(e.Text) })
	case runner.ProgressEvent:
		if c.abortAsked.Load() {
			return
		}
		uiDo(func() { c.log.UpdateProgress(e.Text) })
	case runner.ExitEvent:
		uiDo(func() { c.handleExit(e) })
	}
}

func (c *TaskController) handleExit(e runner.ExitEvent) {
	c.log.CommitProgress()

	status, isErr := "正常", false
	switch {
	case e.Aborted:
		status, isErr = "已中止", true
	case e.Code != 0:
		status, isErr = "失败", true
	}
	msg := fmt.Sprintf("退出码：%d（%s）", e.Code, status)
	if isErr {
		c.log.AppendError(msg)
	} else {
		c.log.AppendBanner(msg)
	}
	c.finish()
}

func (c *TaskController) finish() {
	for _, d := range c.lockables {
		if d != nil {
			d.Enable()
		}
	}
	if c.reset != nil {
		c.reset()
	}
	if c.refresh != nil {
		c.refresh()
	}
	c.trigger = nil
	c.reset = nil
	c.abortAsked.Store(false)
	c.busy.Store(false)
}

func (c *TaskController) disableAll(trigger fyne.Disableable) {
	for _, d := range c.lockables {
		if d == nil || d == trigger {
			continue
		}
		d.Disable()
	}
}

// uiDo 在主线程执行 UI 变更（单测无 Fyne App 时直接执行）。
func uiDo(fn func()) {
	if fyne.CurrentApp() == nil {
		fn()
		return
	}
	fyne.Do(fn)
}
