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
	"tdl-gui/internal/i18n"
	"tdl-gui/internal/store"
)

// langPrefValues 是界面语言下拉框各选项对应的 Preferences 值（索引即协议）。
// 索引 0 = 跟随系统；1 = 简体中文（源语言）；2 = English。
var langPrefValues = []string{"", i18n.LangZh, i18n.LangEn}

// langPrefIndex 把语言偏好值转为下拉框索引。
func langPrefIndex(pref string) int {
	for i, v := range langPrefValues {
		if v == pref {
			return i
		}
	}
	return 0
}

// indexInOptions 在选项列表中反查文本索引（找不到时 0）。
// 比较对象是控件自身的 Options（同一语言下的同一份文本），故不受翻译影响。
func indexInOptions(options []string, label string) int {
	for i, o := range options {
		if o == label {
			return i
		}
	}
	return 0
}

type settingsPage struct {
	ctx  *pageCtx
	view fyne.CanvasObject

	langSelect *widget.Select

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

	// ---- 区块 0：界面（语言）----
	// 语言名用各自母语显示（业界惯例）；仅「跟随系统」参与翻译。
	// 语言选择在重启后生效（v1.2.0 方法 A）；选择时若会改变生效语言则提示重启。
	p.langSelect = widget.NewSelect(nil, nil)
	langOptions := []string{i18n.T("跟随系统"), "简体中文", "English"}
	p.langSelect.SetOptions(langOptions)
	p.langSelect.SetSelected(langOptions[langPrefIndex(st.Str(store.KeySettingsLang, ""))])
	p.langSelect.OnChanged = func(label string) {
		// 索引即协议：比较对象是本控件自身的 Options（同一语言下的同一份文本）
		pref := langPrefValues[indexInOptions(p.langSelect.Options, label)]
		if pref == st.Str(store.KeySettingsLang, "") {
			return
		}
		st.SetStr(store.KeySettingsLang, pref)
		// 方法 A：不热切换；仅当新值确实会改变生效语言时提示重启
		if i18n.Resolve(pref) != i18n.Lang() {
			dialog.ShowInformation(i18n.T("界面语言"),
				i18n.T("语言更改将在重启应用后生效。"), ctx.win)
		}
	}

	// ---- 区块 A：全局参数 ----
	p.tdlPathEntry = newReadOnlyEntry()
	p.tdlPathEntry.SetText(st.Str(store.KeySettingsTdlPath, ""))
	p.tdlPathEntry.SetPlaceHolder(i18n.T("请选择 tdl.exe（必填）"))
	p.tdlPathBtn = widget.NewButton(i18n.T("浏览…"), nil)

	p.nsEntry = widget.NewEntry()
	p.nsEntry.SetPlaceHolder(cmdgen.DefaultNS)
	p.nsEntry.SetText(st.Str(store.KeySettingsNS, cmdgen.DefaultNS))

	p.proxyEntry = widget.NewEntry()
	p.proxyEntry.SetPlaceHolder(i18n.T("如 socks5://localhost:1080（留空 = 不使用）"))
	p.proxyEntry.SetText(st.Str(store.KeySettingsProxy, ""))

	p.ntpEntry = widget.NewEntry()
	p.ntpEntry.SetPlaceHolder(i18n.T("如 ntp.aliyun.com（留空 = 使用系统时间）"))
	p.ntpEntry.SetText(st.Str(store.KeySettingsNTP, ""))

	p.reconEntry = widget.NewEntry()
	p.reconEntry.SetPlaceHolder(i18n.T("如 5m、1m30s（默认 5m）"))
	p.reconEntry.SetText(st.Str(store.KeySettingsReconTimeout, cmdgen.DefaultReconTimeout))

	p.poolEntry = widget.NewEntry()
	p.poolEntry.SetPlaceHolder(i18n.T("默认 8；填 0 表示不限制（tdl 语义）"))
	nonNegativeIntEntry(p.poolEntry)
	p.poolEntry.SetText(st.Str(store.KeySettingsPool, cmdgen.DefaultPool))

	p.delayEntry = widget.NewEntry()
	p.delayEntry.SetPlaceHolder(i18n.T("如 5s（默认 0s）"))
	p.delayEntry.SetText(st.Str(store.KeySettingsDelay, cmdgen.DefaultDelay))

	p.debugCheck = widget.NewCheck(i18n.T("调试模式（--debug）"), nil)
	p.debugCheck.SetChecked(st.Bool(store.KeySettingsDebug, false))
	p.noPSCheck = widget.NewCheck(i18n.T("禁用进度 CPU/内存统计（--disable-progress-ps）"), nil)
	p.noPSCheck.SetChecked(st.Bool(store.KeySettingsDisableProgressPS, false))

	p.storageEntry = widget.NewEntry()
	p.storageEntry.SetPlaceHolder(i18n.T("高级：type=driver,key=value,…（留空 = tdl 默认）"))
	p.storageEntry.SetText(st.Str(store.KeySettingsStorage, ""))

	p.testBtn = widget.NewButton(i18n.T("测试"), nil)
	p.testHint = newRedHint()

	// ---- 区块 B：登录管理（最小化：仅桌面客户端登录）----
	p.loginNSEntry = widget.NewEntry()
	p.loginNSEntry.SetPlaceHolder(i18n.T("命名空间，如 default、work"))
	p.loginNSEntry.Validator = func(s string) error {
		if strings.ContainsAny(s, " \t\r\n") {
			return errors.New(i18n.T("命名空间不能包含空白字符"))
		}
		return nil
	}
	p.loginNSEntry.SetText(st.Str(store.KeyLoginNS, cmdgen.DefaultNS))

	p.passEntry = widget.NewPasswordEntry()
	p.passEntry.SetPlaceHolder(i18n.T("桌面客户端本地密码（没有则留空）"))
	p.passEntry.SetText(st.Str(store.KeyLoginPasscode, ""))

	p.tdDirEntry = newReadOnlyEntry()
	p.tdDirEntry.SetText(st.Str(store.KeyLoginTdDir, ""))
	p.tdDirEntry.SetPlaceHolder(i18n.T("留空 = 自动查找 Telegram Desktop"))
	p.tdDirBtn = widget.NewButton(i18n.T("浏览…"), nil)

	p.loginBtn = widget.NewButton(i18n.T("登录新账户"), nil)
	p.loginHint = newRedHint()
	// 登录新账户 需前端校验通过（tdl 桌面登录本身还需交互输入，见下方说明）
	p.loginBtn.Importance = widget.MediumImportance

	// ---- 事件绑定 ----
	p.tdlPathBtn.OnTapped = func() {
		pickFile(ctx.win, []string{".exe"}, func(path string) {
			p.tdlPathEntry.SetText(path)
			st.SetStr(store.KeySettingsTdlPath, path)
			ctx.log.AppendBanner(i18n.Tf("tdl.exe 路径已设置：{{.p}}", map[string]any{"p": path}))
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
		widget.NewFormItem(i18n.T("tdl.exe 路径"), tdlPathBox),
		widget.NewFormItem(i18n.T("当前命名空间"), p.nsEntry),
		widget.NewFormItem(i18n.T("代理"), p.proxyEntry),
		widget.NewFormItem(i18n.T("NTP 服务器"), p.ntpEntry),
		widget.NewFormItem(i18n.T("重连超时"), p.reconEntry),
		widget.NewFormItem(i18n.T("DC 连接池大小"), p.poolEntry),
		widget.NewFormItem(i18n.T("任务间延迟"), p.delayEntry),
		widget.NewFormItem(i18n.T("存储（高级）"), p.storageEntry),
		widget.NewFormItem("", container.NewVBox(p.debugCheck, p.noPSCheck)),
	)

	blockA := container.NewVBox(
		widget.NewLabelWithStyle(i18n.T("全局参数"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		newHint(i18n.T("以下全局参数不会被 tdl 持久化，每次执行任务时由本软件自动附加。")),
		formA,
		container.NewVBox(
			container.NewHBox(p.testBtn, newHint(i18n.T("执行 tdl version 验证 tdl.exe 是否可用"))),
			p.testHint,
		),
		widget.NewSeparator(),
	)

	formB := newForm(
		widget.NewFormItem(i18n.T("命名空间"), p.loginNSEntry),
		widget.NewFormItem(i18n.T("本地密码"), p.passEntry),
		widget.NewFormItem(i18n.T("客户端目录"), tdDirBox),
	)

	loginHint := container.NewVBox(
		newHint(i18n.T("· 仅支持桌面客户端登录（读取 Telegram Desktop 会话数据），不支持二维码/验证码登录。")),
		newHint(i18n.T("· 客户端须从 Telegram 官网下载（非 Microsoft Store / App Store 版本）。")),
		newHint(i18n.T("· 点击 [登录新账户] 会新开一个终端窗口，请在该窗口中用方向键选择账号并回答后续提示；关闭该窗口或完成后本软件自动解锁。")),
		newHintColored(i18n.T("· 注意：该窗口中若选择登出桌面会话（logout existing desktop session），tdl 会删除 Telegram Desktop 的会话密钥，须确保你能重新登录该客户端。"), colorLogError),
		newHint(i18n.T("· 若该命名空间已存在登录数据，将被覆盖。")),
		newHint(i18n.T("· 登录成功后，请在【当前命名空间】中填写该命名空间，其他页面的任务才会使用该账户。")),
	)

	blockB := widget.NewCard(i18n.T("登录管理"), i18n.T("多账户通过命名空间（-n）实现，每个命名空间即一个独立账户"), container.NewVBox(
		formB,
		container.NewVBox(container.NewHBox(p.loginBtn), p.loginHint),
		loginHint,
	))

	// 区块 0：界面（语言）——语言选择重启后生效（v1.2.0 方法 A）
	block0 := container.NewVBox(
		widget.NewLabelWithStyle(i18n.T("界面"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		newForm(widget.NewFormItem(i18n.T("界面语言"), p.langSelect)),
		newHint(i18n.T("更改后需重启应用生效。")),
		widget.NewSeparator(),
	)

	content := container.NewVScroll(container.NewVBox(block0, blockA, blockB, widget.NewLabel("")))

	p.view = content
	p.refresh()
	return p
}

// toPage 组装为导航页。
func (p *settingsPage) toPage() *Page {
	return &Page{
		Name: i18n.T("设置"),
		View: p.view,
		Controls: []fyne.Disableable{
			p.langSelect,
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
	p.testBtn.SetText(i18n.T("测试"))
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
	p.ctx.log.AppendBanner(i18n.T("已为新终端窗口启动 tdl login：请在该窗口中用方向键选择账号并回答提示（本软件将在其结束后自动解锁）"))
	p.ctx.log.AppendBanner(i18n.T("提示：登录成功后，请在【当前命名空间】中填写该命名空间，其他页面的任务才会使用该账户"))
	tp := p.ctx.tdlPath()
	p.ctx.ctrl.LaunchConsole(tp, args, cmdgen.EchoString(tp, args), p.loginBtn, p.resetLoginBtn)
}

func (p *settingsPage) resetLoginBtn() {
	p.loginBtn.SetText(i18n.T("登录新账户"))
	p.loginBtn.Importance = widget.MediumImportance
	p.loginBtn.OnTapped = func() { p.onLogin() }
	p.loginBtn.Refresh()
}
