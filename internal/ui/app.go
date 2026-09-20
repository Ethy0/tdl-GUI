// Package ui 组装 tdl-GUI 的界面：主窗口、左侧导航、日志黑框、四个功能页与任务控制。
package ui

import (
	"errors"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"tdl-gui/internal/cmdgen"
	"tdl-gui/internal/store"
)

const (
	navWidth = 132

	pageDownload = 0
	pageUpload   = 1
	pageExport   = 2
	pageSettings = 3
)

// Page 是一个导航入口页。
type Page struct {
	Name string
	View fyne.CanvasObject
	// Controls 是本页全部可禁用控件（含 [开始] 按钮）；执行期间统一变灰，
	// 发起任务的触发按钮除外（变红为 [中止]）。
	Controls []fyne.Disableable
	// Refresh 按当前合法性与联动关系重算控件状态。
	Refresh func()
}

// pageCtx 是各页共享的上下文。
type pageCtx struct {
	app  fyne.App
	win  fyne.Window
	st   *store.Store
	log  *LogView
	ctrl *TaskController

	refreshAll func()
}

// tdlPath 返回设置页配置的 tdl.exe 路径（可能为空）。
func (c *pageCtx) tdlPath() string { return c.st.Str(store.KeySettingsTdlPath, "") }

// global 汇总设置页的全局参数。
func (c *pageCtx) global() cmdgen.GlobalFlags {
	return cmdgen.GlobalFlags{
		TdlPath:      c.st.Str(store.KeySettingsTdlPath, ""),
		NS:           c.st.Str(store.KeySettingsNS, cmdgen.DefaultNS),
		Proxy:        c.st.Str(store.KeySettingsProxy, ""),
		NTP:          c.st.Str(store.KeySettingsNTP, ""),
		ReconTimeout: c.st.Str(store.KeySettingsReconTimeout, cmdgen.DefaultReconTimeout),
		Pool:         c.st.Str(store.KeySettingsPool, cmdgen.DefaultPool),
		Delay:        c.st.Str(store.KeySettingsDelay, cmdgen.DefaultDelay),
		Debug:        c.st.Bool(store.KeySettingsDebug, false),
		NoProgPS:     c.st.Bool(store.KeySettingsDisableProgressPS, false),
		Storage:      c.st.Str(store.KeySettingsStorage, ""),
	}
}

// App 持有窗口与页面集合。
type App struct {
	fyneApp fyne.App
	win     fyne.Window
	ctx     *pageCtx
	pages   []*Page
	navBtns []*widget.Button
	current int
}

// Build 组装并返回主窗口（main 与测试共用入口）。
func Build(a fyne.App, version string) fyne.Window {
	st := store.NewStore(a.Preferences())
	// 一次性清理旧版本遗留的"任务目标范围"持久化记录：
	// 上次未完成任务的链接/文件列表不应在本次启动时被恢复。
	st.ClearTaskScope()
	win := a.NewWindow("tdl-GUI " + version)
	win.Resize(fyne.NewSize(1000, 640))

	logv := NewLogView(a)
	ctrl := NewTaskController(win, logv)
	ctx := &pageCtx{app: a, win: win, st: st, log: logv, ctrl: ctrl, refreshAll: func() {}}

	pages := []*Page{
		newDownloadPage(ctx).toPage(),
		newUploadPage(ctx).toPage(),
		newExportPage(ctx).toPage(),
		newSettingsPage(ctx).toPage(),
	}
	ctx.refreshAll = func() {
		for _, p := range pages {
			if p.Refresh != nil {
				p.Refresh()
			}
		}
	}

	app := &App{fyneApp: a, win: win, ctx: ctx, pages: pages, current: -1}

	// 左侧导航栏（宽度固定）
	navBox := container.NewVBox()
	for i, p := range pages {
		idx := i
		b := widget.NewButton(p.Name, func() { app.show(idx) })
		app.navBtns = append(app.navBtns, b)
		navBox.Add(container.NewGridWrap(fyne.NewSize(navWidth, 42), b))
	}

	// 右侧：上部日志（40%）+ 下部当前页
	views := make([]fyne.CanvasObject, 0, len(pages))
	for _, p := range pages {
		views = append(views, p.View)
	}
	stack := container.NewStack(views...)
	right := container.NewVSplit(logv.CanvasObject(), stack)
	right.SetOffset(0.4)

	win.SetContent(container.NewBorder(nil, nil, navBox, nil, right))

	// 全局锁定集合：导航按钮 + 各页全部可禁用控件
	lockables := make([]fyne.Disableable, 0, 64)
	for _, b := range app.navBtns {
		lockables = append(lockables, b)
	}
	for _, p := range pages {
		lockables = append(lockables, p.Controls...)
	}
	ctrl.SetLockables(lockables)
	ctrl.SetRefresh(func() { ctx.refreshAll() })

	// 关窗拦截：任务运行中先确认
	win.SetCloseIntercept(func() {
		if !ctrl.Running() {
			win.Close()
			return
		}
		dialog.ShowConfirm("任务运行中", "关闭窗口将强制终止任务，确认？", func(ok bool) {
			if !ok {
				return
			}
			ctrl.Abort()
			win.Close()
		}, win)
	})

	// 首次启动引导
	if ctx.tdlPath() == "" {
		app.show(pageSettings)
		logv.AppendBanner("首次使用：请在设置页选择 tdl.exe 路径")
		logv.AppendBanner("提示：tdl.exe 需自行准备（可从 https://github.com/iyear/tdl 获取）")
	} else {
		app.show(pageDownload)
	}
	return win
}

func (a *App) show(i int) {
	if i < 0 || i >= len(a.pages) {
		return
	}
	for j, p := range a.pages {
		if j == i {
			p.View.Show()
		} else {
			p.View.Hide()
		}
	}
	a.current = i
	for j, b := range a.navBtns {
		if j == i {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.MediumImportance
		}
		b.Refresh()
	}
	if a.pages[i].Refresh != nil {
		a.pages[i].Refresh()
	}
}

// ---------- 共享辅助 ----------

// uriToPath 把 fyne URI 转为本地路径（兼容 Windows 下 "/C:/x" 形式）。
func uriToPath(u fyne.URI) string {
	if u == nil {
		return ""
	}
	p := u.Path()
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// positiveIntEntry 为数字输入框附加"留空或正整数"校验。
func positiveIntEntry(e *widget.Entry) {
	e.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n <= 0 {
			return errors.New("必须为正整数")
		}
		return nil
	}
}

// nonNegativeIntEntry 为数字输入框附加"留空或非负整数"校验
// （用于 0 有特定语义的字段，如 tdl 的 --pool 0 表示不限制）。
func nonNegativeIntEntry(e *widget.Entry) {
	e.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 0 {
			return errors.New("必须为非负整数")
		}
		return nil
	}
}

// newTaskRow 组装页面底部任务行：[开始/中止] 按钮 +（可选）额外按钮 + 禁用原因提示。
// extras 中传入的按钮须由调用方自行登记到 Page.Controls，以纳入执行期全局锁定。
func newTaskRow(btn *widget.Button, hint *canvas.Text, extras ...*widget.Button) fyne.CanvasObject {
	row := container.NewHBox(btn)
	for _, b := range extras {
		if b != nil {
			row.Add(b)
		}
	}
	return container.NewVBox(widget.NewSeparator(), row, hint)
}

// firstLine 取多行错误（errors.Join 聚合）的首行作为提示文案。
func firstLine(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}

// newForm 创建无提交/取消按钮的表单。
func newForm(items ...*widget.FormItem) *widget.Form {
	f := widget.NewForm(items...)
	f.SubmitText = ""
	f.CancelText = ""
	return f
}

// newHint 灰色说明小字。
func newHint(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.TextStyle = fyne.TextStyle{Italic: true}
	return l
}

// newReadOnlyEntry 只读输入框（仅可通过配套 [浏览] 按钮修改）。
func newReadOnlyEntry() *widget.Entry {
	e := widget.NewEntry()
	e.Disable()
	return e
}

// newHintColored 带颜色的说明小字。
func newHintColored(s string, c color.Color) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = 12
	return t
}

// newRedHint 红色即时提示（默认隐藏）。
func newRedHint() *canvas.Text {
	t := canvas.NewText("", colorLogError)
	t.TextSize = 13
	t.Hide()
	return t
}

// setRedHint 设置红色提示内容（空串隐藏）。
func setRedHint(t *canvas.Text, s string) {
	if s == "" {
		t.Hide()
		return
	}
	t.Text = s
	t.Show()
	t.Refresh()
}

// pickFile 打开文件选择器（带扩展名过滤）。
func pickFile(win fyne.Window, exts []string, onPick func(path string)) {
	d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		path := uriToPath(rc.URI())
		_ = rc.Close()
		onPick(path)
	}, win)
	if len(exts) > 0 {
		d.SetFilter(storage.NewExtensionFileFilter(exts))
	}
	// 顺序要求：必须先 Show（此时 fyne 内部 f.dialog 才完成创建），
	// 再 Resize；v2.8.1 的 FileDialog.Resize 会先解引用 f.dialog 判空，
	// pre-Show 调用会空指针 panic（见 doc/fix-plan-dialog-crash-and-cjk-render.md §2.1）。
	d.Show()
	d.Resize(fyne.NewSize(820, 560))
}

// pickFolder 打开目录选择器。
func pickFolder(win fyne.Window, onPick func(path string)) {
	d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
		if err != nil || lu == nil {
			return
		}
		onPick(uriToPath(lu))
	}, win)
	d.Show()
	d.Resize(fyne.NewSize(820, 560))
}

// pickSaveFile 打开"另存为"选择器（仅取路径，tdl 稍后写入该文件）。
func pickSaveFile(win fyne.Window, defaultName string, onPick func(path string)) {
	d := dialog.NewFileSave(func(wc fyne.URIWriteCloser, err error) {
		if err != nil || wc == nil {
			return
		}
		path := uriToPath(wc.URI())
		_ = wc.Close()
		onPick(path)
	}, win)
	if defaultName != "" {
		d.SetFileName(defaultName)
	}
	d.Show()
	d.Resize(fyne.NewSize(820, 560))
}

// newPathListView 创建路径列表控件；items/selected 由调用方持有并维护。
// locked 返回 true 时忽略选中操作（widget.List 不可 Disable，需自行在运行期间屏蔽交互）。
func newPathListView(items func() []string, selected func() int, onSelect func(int), onRemove func(),
	locked func() bool) (*widget.List, *widget.Button) {
	list := widget.NewList(
		func() int { return len(items()) },
		func() fyne.CanvasObject { return widget.NewLabel("模板") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			all := items()
			if i < 0 || i >= len(all) {
				return
			}
			o.(*widget.Label).SetText(all[i])
		},
	)
	del := widget.NewButton("移除选中", func() { onRemove() })
	del.Disable()
	list.OnSelected = func(id widget.ListItemID) {
		if locked != nil && locked() {
			// 执行期间全局锁定：不接受选中，避免改任务参数
			list.UnselectAll()
			return
		}
		onSelect(id)
		del.Enable()
	}
	list.OnUnselected = func(id widget.ListItemID) {
		if selected() == id {
			onSelect(-1)
		}
		del.Disable()
	}
	return list, del
}
