package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"tdl-gui/internal/cmdgen"
	"tdl-gui/internal/runner"
	"tdl-gui/internal/store"
)

func newTestCtx(t *testing.T) (*pageCtx, *widget.Button) {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)

	win := a.NewWindow("test")
	logv := NewLogView(a)
	ctrl := NewTaskController(win, logv)
	ctx := &pageCtx{app: a, win: win, st: store.NewStore(a.Preferences()), log: logv, ctrl: ctrl}
	ctx.refreshAll = func() {}
	return ctx, nil
}

func TestBuildFirstRun(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := Build(a, "test")
	if win == nil {
		t.Fatal("Build 应返回窗口")
	}
	if win.Content() == nil {
		t.Fatal("窗口应有内容")
	}
	if !strings.HasPrefix(win.Title(), "tdl-GUI") {
		t.Fatalf("窗口标题不含版本：%q", win.Title())
	}
}

func TestLogViewLines(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	l.AppendLine("line1")
	l.AppendBanner("banner")
	l.AppendError("err")

	lines := l.Lines()
	if len(lines) != 3 || lines[0] != "line1" || lines[1] != "banner" || lines[2] != "err" {
		t.Fatalf("日志行不符：%#v", lines)
	}

	l.Clear()
	if len(l.Lines()) != 0 {
		t.Fatalf("清空后应为空：%#v", l.Lines())
	}
}

func TestLogViewProgressCommit(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	l.AppendLine("start")
	l.UpdateProgress("10%")
	l.UpdateProgress("90%")
	l.CommitProgress()

	lines := l.Lines()
	if len(lines) != 2 {
		t.Fatalf("进度行应被固化为正式行：%#v", lines)
	}
	if lines[1] != "90%" {
		t.Fatalf("应固化最新进度值，实际 %q", lines[1])
	}
}

func TestLogViewProgressThenLine(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	l.UpdateProgress("50%")
	l.AppendLine("after")
	l.AppendLine("more")

	lines := l.Lines()
	if len(lines) != 3 || lines[0] != "50%" {
		t.Fatalf("追加普通行前应先固化进度行：%#v", lines)
	}
}

func TestLogViewTrim(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	// 白盒填充缓冲：避免逐行渲染导致测试退化为 O(n²)
	for i := 0; i < maxLines+10; i++ {
		l.lines = append(l.lines, logLine{text: "x"})
	}
	l.AppendLine("last")

	lines := l.Lines()
	if len(lines) >= maxLines+11 {
		t.Fatalf("超过上限时应丢弃最旧行，实际 %d 行", len(lines))
	}
	if lines[len(lines)-1] != "last" {
		t.Fatalf("最后一行应为新追加内容，实际 %q", lines[len(lines)-1])
	}
	if len(lines) != maxLines-trimLines+11 {
		t.Fatalf("裁剪结果不符：%d（期望 %d）", len(lines), maxLines-trimLines+11)
	}
}

func TestLogViewManyAppendsIsThrottled(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	start := time.Now()
	for i := 0; i < 3000; i++ {
		l.AppendLine("streaming line")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("高频追加耗时过长（重绘未节流）：%v", elapsed)
	}
	if got := len(l.Lines()); got != 3000 {
		t.Fatalf("应保留 3000 行，实际 %d", got)
	}
}

func TestTaskControllerLifecycle(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := a.NewWindow("test")
	logv := NewLogView(a)
	ctrl := NewTaskController(win, logv)

	btn := widget.NewButton("开始", nil)
	other := widget.NewButton("其他", nil)
	ctrl.SetLockables([]fyne.Disableable{btn, other})

	done := make(chan struct{})
	var doneOnce bool
	ctrl.SetRefresh(func() {
		if !doneOnce {
			doneOnce = true
			close(done)
		}
	})

	reset := func() {
		btn.SetText("开始")
		btn.Importance = widget.MediumImportance
		btn.OnTapped = func() {}
		btn.Refresh()
	}
	btn.OnTapped = func() {
		ctrl.Launch("cmd.exe", []string{"/c", "echo hello-ui"}, "$ test cmd", btn, reset)
	}

	test.Tap(btn)

	if btn.Text != "中止" {
		t.Fatalf("点击后按钮应立即变为中止，实际 %q", btn.Text)
	}
	if btn.Importance != widget.DangerImportance {
		t.Fatal("中止按钮应为红色（DangerImportance）")
	}
	if btn.Disabled() {
		t.Fatal("触发按钮必须保持可交互")
	}
	if !other.Disabled() {
		t.Fatal("执行期间其余控件应被锁定（变灰）")
	}
	if !ctrl.Running() {
		t.Fatal("执行期间 Running 应为 true")
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("等待任务结束超时")
	}

	if btn.Text != "开始" {
		t.Fatalf("任务结束后按钮应恢复，实际 %q", btn.Text)
	}
	if other.Disabled() {
		t.Fatal("任务结束后其余控件应恢复可交互")
	}
	if ctrl.Running() {
		t.Fatal("任务结束后 Running 应为 false")
	}

	logs := strings.Join(logv.Lines(), "\n")
	if !strings.Contains(logs, "hello-ui") {
		t.Fatalf("日志应包含子进程输出：%s", logs)
	}
	if !strings.Contains(logs, "退出码：0（正常）") {
		t.Fatalf("日志应包含退出码标注：%s", logs)
	}
	if !strings.Contains(logs, "$ test cmd") {
		t.Fatalf("日志应回显命令行：%s", logs)
	}
}

func TestTaskControllerAbort(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := a.NewWindow("test")
	logv := NewLogView(a)
	ctrl := NewTaskController(win, logv)

	btn := widget.NewButton("开始", nil)
	ctrl.SetLockables([]fyne.Disableable{btn})

	done := make(chan struct{})
	ctrl.SetRefresh(func() { close(done) })

	btn.OnTapped = func() {
		ctrl.Launch("cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 > nul"}, "$ long cmd", btn, func() {
			btn.SetText("开始")
			btn.Importance = widget.MediumImportance
			btn.Refresh()
		})
	}
	test.Tap(btn)
	if btn.Text != "中止" {
		t.Fatalf("应变为中止按钮，实际 %q", btn.Text)
	}

	time.Sleep(300 * time.Millisecond)
	start := time.Now()
	test.Tap(btn) // 点击 [中止]

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("中止后等待结束超时")
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("中止耗时过长：%v", elapsed)
	}

	logs := strings.Join(logv.Lines(), "\n")
	if !strings.Contains(logs, "任务已被用户中止") {
		t.Fatalf("日志应醒目标注已中止：%s", logs)
	}
	if !strings.Contains(logs, "已中止") || !strings.Contains(logs, "退出码：") {
		t.Fatalf("日志应包含已中止退出码：%s", logs)
	}
	if btn.Text != "开始" {
		t.Fatalf("中止后按钮应恢复，实际 %q", btn.Text)
	}
}

func TestDownloadPageValidation(t *testing.T) {
	ctx, _ := newTestCtx(t)
	p := newDownloadPage(ctx)
	p.refresh()

	if !p.startBtn.Disabled() {
		t.Fatal("未设置 tdl.exe 路径时应禁用 [开始]")
	}

	ctx.st.SetStr(store.KeySettingsTdlPath, `C:\tdl\tdl.exe`)
	p.refresh()
	if !p.startBtn.Disabled() {
		t.Fatal("无输入源时应禁用 [开始]")
	}

	p.linksEntry.SetText("https://t.me/a/1\nhttps://t.me/a/2")
	if p.startBtn.Disabled() {
		t.Fatal("有链接输入时应启用 [开始]")
	}

	p.includeEntry.SetText("jpg,png")
	p.excludeEntry.SetText("mp4")
	if !p.startBtn.Disabled() {
		t.Fatal("白名单与黑名单同时填写时应禁用 [开始]")
	}
	if p.conflictHint.Visible() == false {
		t.Fatal("冲突时应显示红字提示")
	}

	p.excludeEntry.SetText("")
	if p.startBtn.Disabled() {
		t.Fatal("冲突解除后应恢复 [开始]")
	}
	if p.conflictHint.Visible() {
		t.Fatal("冲突解除后红字提示应隐藏")
	}

	// serve 联动端口
	if !p.portEntry.Disabled() {
		t.Fatal("未勾选 serve 时端口应禁用")
	}
	p.serveCheck.SetChecked(true)
	if p.portEntry.Disabled() {
		t.Fatal("勾选 serve 后端口应可用")
	}
	p.serveCheck.SetChecked(false)
}

func TestDownloadPageCollect(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	ctx.st.SetStr(store.KeySettingsNS, "work")
	p := newDownloadPage(ctx)

	p.linksEntry.SetText("https://t.me/a/1")
	p.threadsEntry.SetText("4")
	p.resumeRadio.SetSelected(resumeLabels[1])
	p.serveCheck.SetChecked(true)
	p.portEntry.SetText("9090")

	args, links, err := p.buildArgs()
	if err != nil {
		t.Fatalf("buildArgs 出错：%v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-n work", "dl", "-u https://t.me/a/1", "-t 4", "--serve", "--port 9090", "--continue"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("命令行缺少 %q：%s", want, joined)
		}
	}
	if len(links) != 1 {
		t.Fatalf("链接数应为 1，实际 %d", len(links))
	}
}

// 任务目标范围输入不持久化：启动时一律为空，绝不恢复上次未完成任务的链接/列表。
func TestTaskScopeInputsNotRestored(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	// 伪造"上次未完成的任务"遗留持久化数据
	ctx.st.SetStr(store.KeyDownloadLinks, "https://t.me/old/1")
	ctx.st.SetStr(store.KeyDownloadTxtPath, `C:\gone\links.txt`)
	ctx.st.SetList(store.KeyDownloadJSONFiles, []string{`C:\old\e.json`})
	ctx.st.SetList(store.KeyUploadPaths, []string{`D:\old.zip`})
	ctx.st.SetStr(store.KeyUploadChat, "@old")
	ctx.st.SetStr(store.KeyExportChat, "@old")
	ctx.st.SetStr(store.KeyExportOutput, `C:\old\out.json`)

	dp := newDownloadPage(ctx)
	if dp.linksEntry.Text != "" {
		t.Fatalf("消息链接不应被恢复，实际 %q", dp.linksEntry.Text)
	}
	if len(dp.txtLinks) != 0 || dp.txtPathLbl.Text != "未选择文件" {
		t.Fatalf("txt 导入不应被恢复：links=%d path=%q", len(dp.txtLinks), dp.txtPathLbl.Text)
	}
	if len(dp.jsonFiles) != 0 {
		t.Fatalf("JSON 文件列表不应被恢复：%#v", dp.jsonFiles)
	}
	if !dp.startBtn.Disabled() {
		t.Fatal("无输入源时应禁用 [开始]")
	}

	up := newUploadPage(ctx)
	if len(up.paths) != 0 || up.chatEntry.Text != "" {
		t.Fatalf("上传页任务目标范围不应被恢复：paths=%#v chat=%q", up.paths, up.chatEntry.Text)
	}

	ep := newExportPage(ctx)
	if ep.chatEntry.Text != "" || ep.outputEntry.Text != cmdgen.DefaultExportOutput {
		t.Fatalf("导出页任务目标范围不应被恢复：chat=%q output=%q", ep.chatEntry.Text, ep.outputEntry.Text)
	}
}

// 任务目标范围输入不再写入 Preferences（含输入框与列表增删）。
func TestTaskScopeInputsNotPersisted(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")

	ctx.st.SetStr(store.KeyDownloadLinks, "") // 先清空，便于断言"未写入"
	dp := newDownloadPage(ctx)
	dp.linksEntry.SetText("https://t.me/a/1")
	dp.addJSONPath(`C:\a\e.json`)
	if got := ctx.st.Str(store.KeyDownloadLinks, ""); got != "" {
		t.Fatalf("消息链接不应写入持久化，实际 %q", got)
	}
	if got := ctx.st.List(store.KeyDownloadJSONFiles); got != nil {
		t.Fatalf("JSON 列表不应写入持久化，实际 %#v", got)
	}
	dp.removeJSON()
	if got := ctx.st.List(store.KeyDownloadJSONFiles); got != nil {
		t.Fatalf("移除 JSON 后不应写入持久化，实际 %#v", got)
	}

	up := newUploadPage(ctx)
	up.addPath(`D:\a.zip`)
	up.chatEntry.SetText("@iyear")
	if got := ctx.st.List(store.KeyUploadPaths); got != nil {
		t.Fatalf("上传路径列表不应写入持久化，实际 %#v", got)
	}
	if got := ctx.st.Str(store.KeyUploadChat, ""); got != "" {
		t.Fatalf("上传目标聊天不应写入持久化，实际 %q", got)
	}

	ep := newExportPage(ctx)
	ep.chatEntry.SetText("@iyear")
	ep.rangeEntry.SetText("1,2")
	ep.typeRadio.SetSelected(exportTypeLabels[2])
	if got := ctx.st.Str(store.KeyExportChat, ""); got != "" {
		t.Fatalf("导出目标聊天不应写入持久化，实际 %q", got)
	}
	if got := ctx.st.Str(store.KeyExportRange, ""); got != "" {
		t.Fatalf("导出范围不应写入持久化，实际 %q", got)
	}
	if got := ctx.st.Str(store.KeyExportType, ""); got != "" {
		t.Fatalf("导出类型不应写入持久化，实际 %q", got)
	}
}

// [清空列表] 一并清空消息链接、txt 导入与 JSON 文件列表。
func TestDownloadClearList(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newDownloadPage(ctx)

	p.linksEntry.SetText("https://t.me/a/1\nhttps://t.me/a/2")
	p.txtLinks = []string{"https://t.me/b/1"}
	p.txtPathLbl.SetText(`C:\links.txt`)
	p.txtCountLbl.SetText("已解析 1 条链接")
	p.jsonFiles = []string{`C:\a\e.json`}
	p.jsonSel = 0
	p.refresh()
	if p.startBtn.Disabled() {
		t.Fatal("填充输入后 [开始] 应可用")
	}

	test.Tap(p.clearBtn)

	if p.linksEntry.Text != "" {
		t.Fatalf("消息链接应被清空，实际 %q", p.linksEntry.Text)
	}
	if len(p.txtLinks) != 0 || p.txtPathLbl.Text != "未选择文件" || p.txtCountLbl.Text != txtCountHintDefault {
		t.Fatalf("txt 导入应被清空：links=%d path=%q hint=%q", len(p.txtLinks), p.txtPathLbl.Text, p.txtCountLbl.Text)
	}
	if len(p.jsonFiles) != 0 || p.jsonSel != -1 {
		t.Fatalf("JSON 列表应被清空：files=%#v sel=%d", p.jsonFiles, p.jsonSel)
	}
	if !p.startBtn.Disabled() {
		t.Fatal("清空后无输入源，应禁用 [开始]")
	}
	if !strings.Contains(strings.Join(ctx.log.Lines(), "\n"), "已清空") {
		t.Fatal("清空操作应写入日志")
	}
}

// 执行期间 [清空列表] 必须被全局锁定，任务结束后恢复。
func TestDownloadClearListLockedWhileRunning(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newDownloadPage(ctx)
	pg := p.toPage()

	ctx.ctrl.SetLockables(pg.Controls)
	ctx.ctrl.disableAll(p.startBtn) // 模拟执行期锁定（trigger 除外）
	if !p.clearBtn.Disabled() {
		t.Fatal("执行期间 [清空列表] 应被锁定")
	}
	ctx.ctrl.finish()
	if p.clearBtn.Disabled() {
		t.Fatal("任务结束后 [清空列表] 应恢复可点")
	}
}

func TestUploadPageValidation(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newUploadPage(ctx)
	p.refresh()

	if !p.startBtn.Disabled() {
		t.Fatal("无待上传路径时应禁用 [开始]")
	}

	p.addPath(`D:\media\a.zip`)
	if p.startBtn.Disabled() {
		t.Fatal("添加路径后应启用 [开始]")
	}

	p.toEntry.SetText("expr://x")
	if !p.chatEntry.Disabled() || !p.topicEntry.Disabled() {
		t.Fatal("启用消息路由后应禁用目标聊天与话题")
	}
	p.toEntry.SetText("")
	if p.chatEntry.Disabled() || p.topicEntry.Disabled() {
		t.Fatal("清空消息路由后应恢复目标聊天与话题")
	}

	p.includeEntry.SetText("zip")
	p.excludeEntry.SetText("rar")
	if !p.startBtn.Disabled() {
		t.Fatal("白黑名单冲突时应禁用 [开始]")
	}
}

func TestExportPageValidation(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newExportPage(ctx)
	p.refresh()

	if p.startBtn.Disabled() {
		t.Fatal("设置 tdl.exe 后应启用 [开始]")
	}
	if !strings.Contains(p.rangeEntry.PlaceHolder, "时间戳") {
		t.Fatalf("默认类型占位符应为时间范围，实际 %q", p.rangeEntry.PlaceHolder)
	}

	p.typeRadio.SetSelected(exportTypeLabels[2])
	if !strings.Contains(p.rangeEntry.PlaceHolder, "数量") {
		t.Fatalf("切换到 last 后占位符应变化，实际 %q", p.rangeEntry.PlaceHolder)
	}

	if err := p.rangeEntry.Validator("1,2"); err != nil {
		t.Fatalf("合法范围不应报错：%v", err)
	}
	if err := p.rangeEntry.Validator("x"); err == nil {
		t.Fatal("非法范围应报错")
	}
}

func TestSettingsPageLoginAndTest(t *testing.T) {
	ctx, _ := newTestCtx(t)
	p := newSettingsPage(ctx)
	p.refresh()

	if !p.testBtn.Disabled() {
		t.Fatal("未设置 tdl.exe 时应禁用 [测试]")
	}
	if !p.loginBtn.Disabled() {
		t.Fatal("未设置 tdl.exe 时应禁用 [登录新账户]")
	}

	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p.refresh()
	if p.testBtn.Disabled() {
		t.Fatal("设置 tdl.exe 后应启用 [测试]")
	}
	if p.testBtn.Text != "测试" {
		t.Fatalf("按钮文案应为「测试」，实际 %q", p.testBtn.Text)
	}

	p.loginNSEntry.SetText("work")
	if p.loginBtn.Disabled() {
		t.Fatal("命名空间非空时应启用 [登录新账户]")
	}

	p.loginNSEntry.SetText("   ")
	if !p.loginBtn.Disabled() {
		t.Fatal("命名空间为空白时应禁用 [登录新账户]")
	}
}

// 宽字符（汉字/全角）必须按显示宽度补列，否则字形会被相邻字符覆盖。
func TestLogViewWideCharCells(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	l.AppendLine("首次使用：请在设置页选择 tdl.exe 路径")

	row := l.grid.Row(0)
	want := cellWidth("首次使用：请在设置页选择 tdl.exe 路径")
	if len(row.Cells) != want {
		t.Fatalf("单元格数应等于显示列数 %d，实际 %d", want, len(row.Cells))
	}
	if got := l.grid.RowText(0); got != "首次使用：请在设置页选择 tdl.exe 路径" {
		t.Fatalf("行还原文本不一致：%q", got)
	}
	if got := cellWidth("中文abc"); got != 7 {
		t.Fatalf("cellWidth(中文abc) 应为 7，实际 %d", got)
	}
	if got := cellWidth("a\tb"); got != 9 {
		t.Fatalf("制表符应展开到制表位：cellWidth(a\\tb)=%d，期望 9", got)
	}
}

// 执行期间列表控件不接受选中（widget.List 不可 Disable，需自行屏蔽）。
func TestListLockedWhileRunning(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newDownloadPage(ctx)

	p.jsonFiles = []string{`C:\a\e.json`}
	p.refresh()

	ctx.ctrl.busy.Store(true) // 模拟任务运行中
	p.jsonList.Select(0)
	if p.jsonSel != -1 {
		t.Fatalf("运行期间不应接受列表选中，实际选中 %d", p.jsonSel)
	}
	if !p.jsonDel.Disabled() {
		t.Fatal("运行期间 [移除选中] 不应被启用")
	}

	ctx.ctrl.busy.Store(false)
	p.jsonList.Select(0)
	if p.jsonSel != 0 {
		t.Fatalf("结束锁定后应可正常选中，实际 %d", p.jsonSel)
	}
}

// 上传页：消息路由与目标聊天同时填写时 [开始] 必须禁用（§5.2）。
func TestUploadToChatConflictDisablesStart(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newUploadPage(ctx)

	p.addPath(`D:\a.zip`)
	if p.startBtn.Disabled() {
		t.Fatal("仅有路径时应可执行")
	}

	p.chatEntry.SetText("@iyear")
	p.toEntry.SetText("from(a)")
	if !p.startBtn.Disabled() {
		t.Fatal("路由与目标聊天冲突时应禁用 [开始]")
	}
	if !p.toHint.Visible() {
		t.Fatal("冲突时应给出提示")
	}

	p.toEntry.SetText("")
	if p.startBtn.Disabled() {
		t.Fatal("解除冲突后应恢复 [开始]")
	}
}

// 非法数字必须禁用执行（§6），而非仅点击后弹窗。
func TestInvalidNumbersDisableStart(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")

	dp := newDownloadPage(ctx)
	dp.linksEntry.SetText("https://t.me/a/1")
	if dp.startBtn.Disabled() {
		t.Fatal("合法输入应可执行")
	}
	for _, bad := range []string{"0", "-5", "abc", "1.5"} {
		dp.threadsEntry.SetText(bad)
		dp.refresh()
		if !dp.startBtn.Disabled() {
			t.Fatalf("线程数 %q 非法时应禁用 [开始]", bad)
		}
	}
	dp.threadsEntry.SetText("4")
	dp.refresh()
	if dp.startBtn.Disabled() {
		t.Fatal("恢复合法值后应可执行")
	}

	ep := newExportPage(ctx)
	if ep.startBtn.Disabled() {
		t.Fatal("导出页默认应可执行")
	}
	ep.rangeEntry.SetText("1,,2")
	ep.refresh()
	if !ep.startBtn.Disabled() {
		t.Fatal("非法范围应禁用 [开始]")
	}
	ep.rangeEntry.SetText("")
	ep.replyEntry.SetText("x")
	ep.refresh()
	if !ep.startBtn.Disabled() {
		t.Fatal("非法评论 ID 应禁用 [开始]")
	}
}

// 连接池 0 表示不限制，应被接受（tdl 语义）。
func TestPoolZeroAccepted(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newSettingsPage(ctx)
	ctx.refreshAll = func() { p.refresh() } // 与真实 App 一致：字段变化触发本页刷新

	p.poolEntry.SetText("0")
	if err := p.poolEntry.Validator("0"); err != nil {
		t.Fatalf("连接池 0 应被接受：%v", err)
	}
	if p.loginBtn.Disabled() {
		t.Fatal("连接池 0 时登录按钮应可用")
	}

	p.poolEntry.SetText("-1")
	if err := p.poolEntry.Validator("-1"); err == nil {
		t.Fatal("连接池 -1 应被拒绝")
	}
	p.refresh()
	if !p.loginBtn.Disabled() {
		t.Fatal("连接池非法时应禁用登录按钮")
	}

	p.poolEntry.SetText("0")
	ctx.st.SetStr(store.KeySettingsPool, "0")
	dp := newDownloadPage(ctx)
	dp.linksEntry.SetText("https://t.me/a/1")
	if dp.startBtn.Disabled() {
		t.Fatal("连接池 0 时下载页 [开始] 应可用")
	}
	args, _, err := dp.buildArgs()
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if !strings.Contains(strings.Join(args, " "), "--pool 0") {
		t.Fatalf("应拼入 --pool 0：%v", args)
	}
}

// 数字字段非法时必须"仅修改该字段即即时禁用 [开始]"（不得依赖其它字段触发刷新）。
func TestInvalidNumbersDisableStartImmediately(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")

	dp := newDownloadPage(ctx)
	dp.linksEntry.SetText("https://t.me/a/1")
	if dp.startBtn.Disabled() {
		t.Fatal("合法输入应可执行")
	}
	for _, tc := range []struct {
		name  string
		apply func()
		undo  func()
	}{
		{"下载-线程数", func() { dp.threadsEntry.SetText("0") }, func() { dp.threadsEntry.SetText("4") }},
		{"下载-并发数", func() { dp.limitEntry.SetText("abc") }, func() { dp.limitEntry.SetText("") }},
		{"下载-serve端口", func() {
			dp.serveCheck.SetChecked(true)
			dp.portEntry.SetText("-1")
		}, func() {
			dp.portEntry.SetText("8080")
			dp.serveCheck.SetChecked(false)
		}},
	} {
		tc.apply()
		if !dp.startBtn.Disabled() {
			t.Fatalf("%s 非法时应立即禁用 [开始]", tc.name)
		}
		tc.undo()
		if dp.startBtn.Disabled() {
			t.Fatalf("%s 恢复合法值后应可执行", tc.name)
		}
	}

	up := newUploadPage(ctx)
	up.addPath(`D:\a.zip`)
	up.threadsEntry.SetText("x")
	if !up.startBtn.Disabled() {
		t.Fatal("上传页线程数非法时应立即禁用 [开始]")
	}
	up.threadsEntry.SetText("")
	up.limitEntry.SetText("0")
	if !up.startBtn.Disabled() {
		t.Fatal("上传页并发数非法时应立即禁用 [开始]")
	}
	up.limitEntry.SetText("")
	if up.startBtn.Disabled() {
		t.Fatal("上传页恢复合法值后应可执行")
	}
}

// 连接池非法时设置页 [测试] 也必须禁用并说明原因。
func TestTestButtonNeedsValidGlobal(t *testing.T) {
	ctx, _ := newTestCtx(t)
	ctx.st.SetStr(store.KeySettingsTdlPath, "tdl.exe")
	p := newSettingsPage(ctx)
	ctx.refreshAll = func() { p.refresh() }

	if p.testBtn.Disabled() {
		t.Fatal("配置合法时 [测试] 应可用")
	}
	p.poolEntry.SetText("abc")
	if !p.testBtn.Disabled() {
		t.Fatal("连接池非法时 [测试] 应禁用")
	}
	if !p.testHint.Visible() {
		t.Fatal("应给出禁用原因提示")
	}
	p.poolEntry.SetText("8")
	if p.testBtn.Disabled() {
		t.Fatal("恢复合法值后 [测试] 应可用")
	}
}

func TestSettingsPagePersistence(t *testing.T) {
	ctx, _ := newTestCtx(t)
	p := newSettingsPage(ctx)

	p.nsEntry.SetText("work")
	p.proxyEntry.SetText("socks5://localhost:1080")
	p.debugCheck.SetChecked(true)

	if got := ctx.st.Str(store.KeySettingsNS, ""); got != "work" {
		t.Fatalf("命名空间应即时持久化，实际 %q", got)
	}
	if got := ctx.st.Str(store.KeySettingsProxy, ""); got != "socks5://localhost:1080" {
		t.Fatalf("代理应即时持久化，实际 %q", got)
	}
	if !ctx.st.Bool(store.KeySettingsDebug, false) {
		t.Fatal("调试开关应即时持久化")
	}
}

// ---------- D-13 / D-14 / D-15 回归 ----------

// waitThrottle 在测试结束前等待日志节流重绘（AfterFunc）的残留定时器完成，
// 并通过实例锁建立 happens-before（单纯 sleep 不是同步点，竞态检测器仍会
// 把无同步关系的先后访问视为竞态）。
// 须以 defer waitThrottle(l) 注册在 defer a.Quit() 之后（defer 逆序，先于 Quit 执行）。
func waitThrottle(l *LogView) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		pending := l.refreshPending.Load()
		l.mu.Unlock()
		if !pending {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// 超宽行仅截断渲染：grid 单元格数封顶，lines 原文与复制不受影响（D-13）。
func TestLogViewRowWidthCapped(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	defer waitThrottle(l)
	long := strings.Repeat("x", maxLogCols+300)
	progress := strings.Repeat("y", maxLogCols+500)
	l.AppendLine(long)
	l.UpdateProgress(progress)
	l.CommitProgress()

	lines := l.Lines()
	if len(lines) != 2 || lines[0] != long || lines[1] != progress {
		t.Fatalf("截断不应影响 lines 原文：%d 行", len(lines))
	}
	for i, row := range l.grid.Rows {
		if len(row.Cells) > maxLogCols {
			t.Fatalf("第 %d 行单元格数 %d 超过上限 %d", i, len(row.Cells), maxLogCols)
		}
	}
	if got := l.grid.RowText(0); !strings.HasSuffix(got, "…") {
		t.Fatalf("超宽行应以省略号提示截断：%q", got)
	}
}

// 进度行特征识别（D-14）：
// 正例取自真实 tdl 管道输出（含百分比/速度/ETA），负例为普通日志行。
func TestProgressLike(t *testing.T) {
	yes := []string{
		"4.0% [.........] [128.00 MB in 1m44.942s; ~ETA: 41m59s; 1.22 MB/s]",
		"] [1m45s; 2.43 MB/s]",
		"downloading ... 45%",
	}
	no := []string{
		"INF download completed",
		"退出码：0（正常）",
		"https://t.me/a/1",
		"已解析 3 条链接",
	}
	for _, s := range yes {
		if !progressLike(s) {
			t.Errorf("应识别为进度行：%q", s)
		}
	}
	for _, s := range no {
		if progressLike(s) {
			t.Errorf("不应识别为进度行：%q", s)
		}
	}
}

// 未中止时：进度刷新行重定向为单行覆盖，不逐行追加（D-14）。
func TestProgressLinesMergedIntoOne(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := a.NewWindow("test")
	logv := NewLogView(a)
	defer waitThrottle(logv)
	ctrl := NewTaskController(win, logv)

	ctrl.onEvent(runner.LineEvent{Text: "4.0% [.....] [128.00 MB in 1m44.9s; ~ETA: 41m59s; 1.22 MB/s]"})
	ctrl.onEvent(runner.LineEvent{Text: "5.2% [.....] [123.00 MB in 1m41.8s; ~ETA: 32m15s; 1.21 MB/s]"})

	// 等待事件被派发处理（uiDo 在测试环境可能异步）。
	deadline := time.Now().Add(5 * time.Second)
	for logv.progress == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if logv.progress == "" {
		t.Fatal("进度事件未被处理")
	}
	if got := logv.Lines(); len(got) != 0 {
		t.Fatalf("进度刷新不应逐行追加到日志：%#v", got)
	}

	logv.CommitProgress()
	lines := logv.Lines()
	if len(lines) != 1 {
		t.Fatalf("进度刷新应合并为单行：%#v", lines)
	}
	if !strings.Contains(lines[0], "5.2%") {
		t.Fatalf("应固化最新进度值，实际 %q", lines[0])
	}
}

// tdl 系统遥测行（CPU/内存/Goroutines）不得进入日志或抢占进度行（D-16）。
// 真实管道输出中每帧首行即该遥测行，若不丢弃会因节流=帧间隔导致进度恒显示遥测行。
func TestPinnedTelemetryDropped(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := a.NewWindow("test")
	logv := NewLogView(a)
	defer waitThrottle(logv)
	ctrl := NewTaskController(win, logv)

	ctrl.onEvent(runner.LineEvent{Text: "CPU: 9.37% Memory: 33.88 MB Goroutines: 53"})
	ctrl.onEvent(runner.LineEvent{Text: "5.2% [.....] [123.00 MB in 1m41.8s; ~ETA: 32m15s; 1.21 MB/s]"})

	// 等待事件被派发处理（uiDo 在测试环境可能异步）。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		logv.mu.Lock()
		p := logv.progress
		logv.mu.Unlock()
		if strings.Contains(p, "5.2%") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	logv.mu.Lock()
	p := logv.progress
	logv.mu.Unlock()
	committed := logv.Lines()
	logv.CommitProgress()
	final := logv.Lines()

	if !strings.Contains(p, "5.2%") {
		t.Fatalf("进度行应显示文件进度，实际 %q", p)
	}
	if strings.Contains(p, "Goroutines") {
		t.Fatalf("遥测行不应进入进度行：%q", p)
	}
	for _, ln := range committed {
		if strings.Contains(ln, "Goroutines") {
			t.Fatalf("遥测行不应进入日志：%#v", committed)
		}
	}
	if len(final) != 1 || !strings.Contains(final[0], "5.2%") {
		t.Fatalf("固化应为文件进度行，实际 %#v", final)
	}
}

// overall 聚合行（无百分比）不得覆盖已存的文件进度行（D-16）。
func TestUpdateProgressOverallDoesNotReplaceFile(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLogView(a)
	defer waitThrottle(l)

	l.UpdateProgress("5.2% [.....] [123.00 MB in 1m41.8s; ~ETA: 32m15s; 1.21 MB/s]")
	l.UpdateProgress("[#................................................] [30s; 5.31 MB/s]") // overall，无百分比
	l.CommitProgress()

	lines := l.Lines()
	if len(lines) != 1 || !strings.Contains(lines[0], "5.2%") {
		t.Fatalf("overall 行不应覆盖文件进度行，实际 %#v", lines)
	}
	if strings.Contains(lines[0], "[30s;") {
		t.Fatalf("不应固化 overall 行：%q", lines[0])
	}
}

// 中止后：残余输出事件必须被丢弃（D-15，防积压事件堵塞 ExitEvent）。
func TestAbortDropsBacklogEvents(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := a.NewWindow("test")
	logv := NewLogView(a)
	defer waitThrottle(logv)
	ctrl := NewTaskController(win, logv)

	ctrl.abortAsked.Store(true)
	ctrl.onEvent(runner.LineEvent{Text: "残留日志"})
	ctrl.onEvent(runner.ProgressEvent{Text: "50%"})

	if got := logv.Lines(); len(got) != 0 {
		t.Fatalf("中止后残余输出不应写入日志：%#v", got)
	}
}

// 中止发起后横幅必须标注"正在强制终止"（D-15：不再提前谎报已完成）。
func TestAbortBannerSemantics(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	win := a.NewWindow("test")
	logv := NewLogView(a)
	defer waitThrottle(logv)
	ctrl := NewTaskController(win, logv)

	ctrl.busy.Store(true)
	ctrl.Abort()
	ctrl.busy.Store(false)

	raw := strings.Join(logv.Lines(), "\n")
	if !strings.Contains(raw, "任务已被用户中止，正在强制终止进程树") {
		t.Fatalf("中止横幅应标注正在强制终止：%q", raw)
	}
}
