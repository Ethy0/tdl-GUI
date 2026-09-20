package ui

import (
	"errors"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"tdl-gui/internal/cmdgen"
	"tdl-gui/internal/store"
)

type settingsPage struct {
	ctx  *pageCtx
	view fyne.CanvasObject

	tdlPathEntry *widget.Entry
	tdlPathBtn   *widget.Button

	nsEntry      *widget.Entry
	proxyEntry   *widget.Entry
	ntpEntry     *widget.Entry
	reconEntry   *widget.Entry
	poolEntry    *widget.Entry
	delayEntry   *widget.Entry
	debugCheck   *widget.Check
	noPSCheck    *widget.Check
	storageEntry *widget.Entry

	testBtn  *widget.Button
	testHint *canvas.Text

	loginNSEntry *widget.Entry
	passEntry    *widget.Entry
	tdDirEntry   *widget.Entry
	tdDirBtn     *widget.Button
	loginBtn     *widget.Button
	loginHint    *canvas.Text
}

func newSettingsPage(ctx *pageCtx) *settingsPage {
	p := &settingsPage{ctx: ctx}
	st := ctx.st

	// ---- 区块 A：全局参数 ----
	p.tdlPathEntry = newReadOnlyEntry()
	p.tdlPathEntry.SetText(st.Str(store.KeySettingsTdlPath, ""))
	p.tdlPathEntry.SetPlaceHolder("请选择 tdl.exe（必填）")
	p.tdlPathBtn = widget.NewButton("浏览…", nil)

	p.nsEntry = widget.NewEntry()
	p.nsEntry.SetPlaceHolder(cmdgen.DefaultNS)
	p.nsEntry.SetText(st.Str(store.KeySettingsNS, cmdgen.DefaultNS))

	p.proxyEntry = widget.NewEntry()
	p.proxyEntry.SetPlaceHolder("如 socks5://localhost:1080（留空 = 不使用）")
	p.proxyEntry.SetText(st.Str(store.KeySettingsProxy, ""))

	p.ntpEntry = widget.NewEntry()
	p.ntpEntry.SetPlaceHolder("如 ntp.aliyun.com（留空 = 使用系统时间）")
	p.ntpEntry.SetText(st.Str(store.KeySettingsNTP, ""))

	p.reconEntry = widget.NewEntry()
	p.reconEntry.SetPlaceHolder("如 5m、1m30s（默认 5m）")
	p.reconEntry.SetText(st.Str(store.KeySettingsReconTimeout, cmdgen.DefaultReconTimeout))

	p.poolEntry = widget.NewEntry()
	p.poolEntry.SetPlaceHolder("默认 8；填 0 表示不限制（tdl 语义）")
	nonNegativeIntEntry(p.poolEntry)
	p.poolEntry.SetText(st.Str(store.KeySettingsPool, cmdgen.DefaultPool))

	p.delayEntry = widget.NewEntry()
	p.delayEntry.SetPlaceHolder("如 5s（默认 0s）")
	p.delayEntry.SetText(st.Str(store.KeySettingsDelay, cmdgen.DefaultDelay))

	p.debugCheck = widget.NewCheck("调试模式（--debug）", nil)
	p.debugCheck.SetChecked(st.Bool(store.KeySettingsDebug, false))
	p.noPSCheck = widget.NewCheck("禁用进度 CPU/内存统计（--disable-progress-ps）", nil)
	p.noPSCheck.SetChecked(st.Bool(store.KeySettingsDisableProgressPS, false))

	p.storageEntry = widget.NewEntry()
	p.storageEntry.SetPlaceHolder("高级：type=driver,key=value,…（留空 = tdl 默认）")
	p.storageEntry.SetText(st.Str(store.KeySettingsStorage, ""))

	p.testBtn = widget.NewButton("测试", nil)
	p.testHint = newRedHint()

	// ---- 区块 B：登录管理（最小化：仅桌面客户端登录）----
	p.loginNSEntry = widget.NewEntry()
	p.loginNSEntry.SetPlaceHolder("命名空间，如 default、work")
	p.loginNSEntry.Validator = func(s string) error {
		if strings.ContainsAny(s, " \t\r\n") {
			return errors.New("命名空间不能包含空白字符")
		}
		return nil
	}
	p.loginNSEntry.SetText(st.Str(store.KeyLoginNS, cmdgen.DefaultNS))

	p.passEntry = widget.NewPasswordEntry()
	p.passEntry.SetPlaceHolder("桌面客户端本地密码（没有则留空）")
	p.passEntry.SetText(st.Str(store.KeyLoginPasscode, ""))

	p.tdDirEntry = newReadOnlyEntry()
	p.tdDirEntry.SetText(st.Str(store.KeyLoginTdDir, ""))
	p.tdDirEntry.SetPlaceHolder("留空 = 自动查找 Telegram Desktop")
	p.tdDirBtn = widget.NewButton("浏览…", nil)

	p.loginBtn = widget.NewButton("登录新账户", nil)
	p.loginHint = newRedHint()
	// 登录新账户 需前端校验通过（tdl 桌面登录本身还需交互输入，见下方说明）
	p.loginBtn.Importance = widget.MediumImportance

	// ---- 事件绑定 ----
	p.tdlPathBtn.OnTapped = func() {
		pickFile(ctx.win, []string{".exe"}, func(path string) {
			p.tdlPathEntry.SetText(path)
			st.SetStr(store.KeySettingsTdlPath, path)
			ctx.log.AppendBanner("tdl.exe 路径已设置：" + path)
			ctx.refreshAll()
		})
	}
	p.nsEntry.OnChanged = func(s string) { st.SetStr(store.KeySettingsNS, s) }
	p.proxyEntry.OnChanged = func(s string) { st.SetStr(store.KeySettingsProxy, s) }
	p.ntpEntry.OnChanged = func(s string) { st.SetStr(store.KeySettingsNTP, s) }
	p.reconEntry.OnChanged = func(s string) { st.SetStr(store.KeySettingsReconTimeout, s) }
	p.poolEntry.OnChanged = func(s string) {
		st.SetStr(store.KeySettingsPool, s)
		ctx.refreshAll() // 连接池参与所有命令，需重算各页 [开始] 状态
	}
	p.delayEntry.OnChanged = func(s string) { st.SetStr(store.KeySettingsDelay, s) }
	p.debugCheck.OnChanged = func(v bool) { st.SetBool(store.KeySettingsDebug, v) }
	p.noPSCheck.OnChanged = func(v bool) { st.SetBool(store.KeySettingsDisableProgressPS, v) }
	p.storageEntry.OnChanged = func(s string) { st.SetStr(store.KeySettingsStorage, s) }
	p.testBtn.OnTapped = func() { p.onTest() }

	p.loginNSEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyLoginNS, s)
		p.refresh()
	}
	p.passEntry.OnChanged = func(s string) { st.SetStr(store.KeyLoginPasscode, s) }
	p.tdDirBtn.OnTapped = func() {
		pickFolder(ctx.win, func(path string) {
			p.tdDirEntry.SetText(path)
			st.SetStr(store.KeyLoginTdDir, path)
		})
	}
	p.loginBtn.OnTapped = func() { p.onLogin() }

	// ---- 布局 ----
	tdlPathBox := container.NewBorder(nil, nil, nil, p.tdlPathBtn, p.tdlPathEntry)
	tdDirBox := container.NewBorder(nil, nil, nil, p.tdDirBtn, p.tdDirEntry)

	formA := newForm(
		widget.NewFormItem("tdl.exe 路径", tdlPathBox),
		widget.NewFormItem("当前命名空间", p.nsEntry),
		widget.NewFormItem("代理", p.proxyEntry),
		widget.NewFormItem("NTP 服务器", p.ntpEntry),
		widget.NewFormItem("重连超时", p.reconEntry),
		widget.NewFormItem("DC 连接池大小", p.poolEntry),
		widget.NewFormItem("任务间延迟", p.delayEntry),
		widget.NewFormItem("存储（高级）", p.storageEntry),
		widget.NewFormItem("", container.NewVBox(p.debugCheck, p.noPSCheck)),
	)

	blockA := container.NewVBox(
		widget.NewLabelWithStyle("全局参数", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		newHint("以下全局参数不会被 tdl 持久化，每次执行任务时由本软件自动附加。"),
		formA,
		container.NewVBox(
			container.NewHBox(p.testBtn, newHint("执行 tdl version 验证 tdl.exe 是否可用")),
			p.testHint,
		),
		widget.NewSeparator(),
	)

	formB := newForm(
		widget.NewFormItem("命名空间", p.loginNSEntry),
		widget.NewFormItem("本地密码", p.passEntry),
		widget.NewFormItem("客户端目录", tdDirBox),
	)

	loginHint := container.NewVBox(
		newHint("· 仅支持桌面客户端登录（读取 Telegram Desktop 会话数据），不支持二维码/验证码登录。"),
		newHint("· 客户端须从 Telegram 官网下载（非 Microsoft Store / App Store 版本）。"),
		newHint("· 点击 [登录新账户] 会新开一个终端窗口，请在该窗口中用方向键选择账号并回答后续提示；关闭该窗口或完成后本软件自动解锁。"),
		newHintColored("· 注意：该窗口中若选择登出桌面会话（logout existing desktop session），tdl 会删除 Telegram Desktop 的会话密钥，须确保你能重新登录该客户端。", colorLogError),
		newHint("· 若该命名空间已存在登录数据，将被覆盖。"),
		newHint("· 登录成功后，请在【当前命名空间】中填写该命名空间，其他页面的任务才会使用该账户。"),
		newHint("· 当前 tdl 版本（0.20.4）不提供列出账户/切换默认/登出命令，故本页不提供这些按钮。"),
	)

	blockB := widget.NewCard("登录管理", "多账户通过命名空间（-n）实现，每个命名空间即一个独立账户", container.NewVBox(
		formB,
		container.NewVBox(container.NewHBox(p.loginBtn), p.loginHint),
		loginHint,
	))

	content := container.NewVScroll(container.NewVBox(blockA, blockB, widget.NewLabel("")))

	p.view = content
	p.refresh()
	return p
}

// toPage 组装为导航页。
func (p *settingsPage) toPage() *Page {
	return &Page{
		Name: "设置",
		View: p.view,
		Controls: []fyne.Disableable{
			p.tdlPathEntry, p.tdlPathBtn,
			p.nsEntry, p.proxyEntry, p.ntpEntry, p.reconEntry, p.poolEntry, p.delayEntry,
			p.debugCheck, p.noPSCheck, p.storageEntry,
			p.testBtn,
			p.loginNSEntry, p.passEntry, p.tdDirEntry, p.tdDirBtn, p.loginBtn,
		},
		Refresh: p.refresh,
	}
}

func (p *settingsPage) refresh() {
	if p.testBtn == nil {
		return
	}
	p.tdlPathEntry.Disable() // 只读：仅可通过 [浏览] 修改
	p.tdDirEntry.Disable()

	// 校验与实际执行同一来源，并给出禁用原因
	testReason := ""
	if _, err := cmdgen.BuildTest(p.ctx.global()); err != nil {
		p.testBtn.Disable()
		testReason = firstLine(err)
	} else {
		p.testBtn.Enable()
	}
	setRedHint(p.testHint, testReason)

	loginReason := ""
	if _, err := cmdgen.BuildLogin(p.ctx.global(), p.loginArgs()); err != nil {
		p.loginBtn.Disable()
		loginReason = firstLine(err)
	} else {
		p.loginBtn.Enable()
	}
	setRedHint(p.loginHint, loginReason)
}

// loginArgs 收集登录区表单。
func (p *settingsPage) loginArgs() cmdgen.LoginArgs {
	return cmdgen.LoginArgs{
		NS:       strings.TrimSpace(p.loginNSEntry.Text),
		Passcode: p.passEntry.Text,
		TdDir:    p.tdDirEntry.Text,
	}
}

func (p *settingsPage) onTest() {
	args, err := cmdgen.BuildTest(p.ctx.global())
	if err != nil {
		dialog.ShowError(err, p.ctx.win)
		return
	}
	tp := p.ctx.tdlPath()
	p.ctx.ctrl.Launch(tp, args, cmdgen.EchoString(tp, args), p.testBtn, p.resetTestBtn)
}

func (p *settingsPage) resetTestBtn() {
	p.testBtn.SetText("测试")
	p.testBtn.Importance = widget.MediumImportance
	p.testBtn.OnTapped = func() { p.onTest() }
	p.testBtn.Refresh()
}

func (p *settingsPage) onLogin() {
	args, err := cmdgen.BuildLogin(p.ctx.global(), p.loginArgs())
	if err != nil {
		dialog.ShowError(err, p.ctx.win)
		return
	}
	p.ctx.log.AppendBanner("已为新终端窗口启动 tdl login：请在该窗口中用方向键选择账号并回答提示（本软件将在其结束后自动解锁）")
	p.ctx.log.AppendBanner("提示：登录成功后，请在【当前命名空间】中填写该命名空间，其他页面的任务才会使用该账户")
	tp := p.ctx.tdlPath()
	p.ctx.ctrl.LaunchConsole(tp, args, cmdgen.EchoString(tp, args), p.loginBtn, p.resetLoginBtn)
}

func (p *settingsPage) resetLoginBtn() {
	p.loginBtn.SetText("登录新账户")
	p.loginBtn.Importance = widget.MediumImportance
	p.loginBtn.OnTapped = func() { p.onLogin() }
	p.loginBtn.Refresh()
}
