package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"tdl-gui/internal/cmdgen"
	"tdl-gui/internal/store"
	"tdl-gui/internal/txtload"
)

// txtCountHintDefault 是 txt 批量导入区未选择文件时的说明文案（清空列表后恢复为它）。
const txtCountHintDefault = "txt 文件中每行一个链接，空行与 # 开头的注释行会被跳过"

// 断点策略单选项（映射 cmdgen.Resume*）。
var resumeLabels = []string{"默认（无附加参数）", "续传（--continue）", "重新开始（--restart）"}

func resumeValue(label string) string {
	switch label {
	case resumeLabels[1]:
		return cmdgen.ResumeContinue
	case resumeLabels[2]:
		return cmdgen.ResumeRestart
	}
	return cmdgen.ResumeDefault
}

func resumeLabel(v string) string {
	switch v {
	case cmdgen.ResumeContinue:
		return resumeLabels[1]
	case cmdgen.ResumeRestart:
		return resumeLabels[2]
	}
	return resumeLabels[0]
}

type downloadPage struct {
	ctx  *pageCtx
	view fyne.CanvasObject

	linksEntry *widget.Entry

	txtBtn      *widget.Button
	txtPathLbl  *widget.Label
	txtCountLbl *widget.Label
	txtLinks    []string

	jsonList  *widget.List
	jsonAdd   *widget.Button
	jsonDel   *widget.Button
	jsonFiles []string
	jsonSel   int

	dirEntry *widget.Entry
	dirBtn   *widget.Button

	threadsEntry *widget.Entry
	limitEntry   *widget.Entry

	descCheck    *widget.Check
	rewriteCheck *widget.Check
	groupCheck   *widget.Check
	skipCheck    *widget.Check
	takeoutCheck *widget.Check

	includeEntry *widget.Entry
	excludeEntry *widget.Entry
	conflictHint *canvas.Text

	templateEntry *widget.Entry
	resumeRadio   *widget.RadioGroup
	serveCheck    *widget.Check
	portEntry     *widget.Entry

	startBtn  *widget.Button
	clearBtn  *widget.Button
	startHint *canvas.Text
}

func newDownloadPage(ctx *pageCtx) *downloadPage {
	p := &downloadPage{ctx: ctx, jsonSel: -1}
	st := ctx.st

	// ---- 创建控件 ----
	p.linksEntry = widget.NewMultiLineEntry()
	p.linksEntry.SetPlaceHolder("每行一个链接，例如：\nhttps://t.me/telegram/193\nhttps://t.me/c/1697797156/151\nhttps://t.me/iFreeKnow/45662/55005")
	p.linksEntry.SetMinRowsVisible(4)
	// 消息链接属"任务目标范围"：不持久化，启动时始终为空（见 store.TaskScopeKeys）
	p.linksEntry.SetText("")

	p.txtBtn = widget.NewButton("选择 txt 文件…", nil)
	p.txtPathLbl = widget.NewLabel("未选择文件")
	p.txtCountLbl = newHint(txtCountHintDefault)

	p.jsonList, p.jsonDel = newPathListView(
		func() []string { return p.jsonFiles },
		func() int { return p.jsonSel },
		func(i int) { p.jsonSel = i },
		func() { p.removeJSON() },
		func() bool { return ctx.ctrl.Running() }, // 执行期间不接受选中（widget.List 不可 Disable）
	)
	p.jsonAdd = widget.NewButton("添加 JSON…", nil)
	// JSON 文件列表属"任务目标范围"：不持久化，启动时始终为空
	p.jsonFiles = nil

	p.dirEntry = newReadOnlyEntry()
	p.dirEntry.SetText(st.Str(store.KeyDownloadDir, ""))
	p.dirBtn = widget.NewButton("浏览…", nil)

	p.threadsEntry = widget.NewEntry()
	p.threadsEntry.SetPlaceHolder("留空 = tdl 默认（4）")
	positiveIntEntry(p.threadsEntry)
	p.threadsEntry.SetText(st.Str(store.KeyDownloadThreads, ""))

	p.limitEntry = widget.NewEntry()
	p.limitEntry.SetPlaceHolder("留空 = tdl 默认（2）")
	positiveIntEntry(p.limitEntry)
	p.limitEntry.SetText(st.Str(store.KeyDownloadLimit, ""))

	p.descCheck = widget.NewCheck("降序下载（最新 → 最旧）", nil)
	p.descCheck.SetChecked(st.Bool(store.KeyDownloadDesc, false))
	p.rewriteCheck = widget.NewCheck("按文件头 MIME 重写扩展名（--rewrite-ext）", nil)
	p.rewriteCheck.SetChecked(st.Bool(store.KeyDownloadRewriteExt, false))
	p.groupCheck = widget.NewCheck("自动下载相册/分组消息（--group）", nil)
	p.groupCheck.SetChecked(st.Bool(store.KeyDownloadGroup, false))
	p.skipCheck = widget.NewCheck("跳过同名且同大小的文件（--skip-same）", nil)
	p.skipCheck.SetChecked(st.Bool(store.KeyDownloadSkipSame, false))
	p.takeoutCheck = widget.NewCheck("使用 Takeout 会话（--takeout）", nil)
	p.takeoutCheck.SetChecked(st.Bool(store.KeyDownloadTakeout, false))

	p.includeEntry = widget.NewEntry()
	p.includeEntry.SetPlaceHolder("如 jpg,png（留空 = 不限制）")
	p.includeEntry.SetText(st.Str(store.KeyDownloadInclude, ""))
	p.excludeEntry = widget.NewEntry()
	p.excludeEntry.SetPlaceHolder("如 mp4,flv（留空 = 不排除）")
	p.excludeEntry.SetText(st.Str(store.KeyDownloadExclude, ""))
	p.conflictHint = newRedHint()

	p.templateEntry = widget.NewEntry()
	p.templateEntry.SetPlaceHolder(`如 {{ .DialogID }}_{{ .MessageID }}_{{ .FileName }}（留空 = tdl 默认）`)
	p.templateEntry.SetText(st.Str(store.KeyDownloadTemplate, ""))

	p.resumeRadio = widget.NewRadioGroup(resumeLabels, nil)
	p.resumeRadio.Horizontal = true
	p.resumeRadio.SetSelected(resumeLabel(st.Str(store.KeyDownloadResume, cmdgen.ResumeDefault)))

	p.serveCheck = widget.NewCheck("serve 模式（以 HTTP 服务方式提供文件而非下载）", nil)
	p.serveCheck.SetChecked(st.Bool(store.KeyDownloadServe, false))
	p.portEntry = widget.NewEntry()
	p.portEntry.SetPlaceHolder(cmdgen.DefaultPort)
	positiveIntEntry(p.portEntry)
	p.portEntry.SetText(st.Str(store.KeyDownloadPort, cmdgen.DefaultPort))

	p.startBtn = widget.NewButton("开始", nil)
	p.startBtn.Importance = widget.HighImportance
	p.clearBtn = widget.NewButton("清空列表", nil)
	p.startHint = newRedHint()

	// ---- 事件绑定（在恢复初值之后，避免无谓写入）----
	p.linksEntry.OnChanged = func(s string) {
		p.refresh() // 消息链接不持久化（任务目标范围）
	}
	p.txtBtn.OnTapped = func() { p.pickTxt() }
	p.jsonAdd.OnTapped = func() { p.addJSON() }
	p.dirBtn.OnTapped = func() {
		pickFolder(ctx.win, func(path string) {
			p.dirEntry.SetText(path)
			st.SetStr(store.KeyDownloadDir, path)
		})
	}
	p.threadsEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyDownloadThreads, s)
		p.refresh() // 数字非法需即时禁用 [开始]
	}
	p.limitEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyDownloadLimit, s)
		p.refresh()
	}
	p.descCheck.OnChanged = func(v bool) { st.SetBool(store.KeyDownloadDesc, v) }
	p.rewriteCheck.OnChanged = func(v bool) { st.SetBool(store.KeyDownloadRewriteExt, v) }
	p.groupCheck.OnChanged = func(v bool) { st.SetBool(store.KeyDownloadGroup, v) }
	p.skipCheck.OnChanged = func(v bool) { st.SetBool(store.KeyDownloadSkipSame, v) }
	p.takeoutCheck.OnChanged = func(v bool) { st.SetBool(store.KeyDownloadTakeout, v) }
	p.includeEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyDownloadInclude, s)
		p.refresh()
	}
	p.excludeEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyDownloadExclude, s)
		p.refresh()
	}
	p.templateEntry.OnChanged = func(s string) { st.SetStr(store.KeyDownloadTemplate, s) }
	p.resumeRadio.OnChanged = func(label string) { st.SetStr(store.KeyDownloadResume, resumeValue(label)) }
	p.serveCheck.OnChanged = func(v bool) {
		st.SetBool(store.KeyDownloadServe, v)
		p.refresh()
	}
	p.portEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyDownloadPort, s)
		p.refresh()
	}
	p.startBtn.OnTapped = func() { p.onStart() }
	p.clearBtn.OnTapped = func() { p.clearTaskInputs() }

	// 说明：txt 导入路径属"任务目标范围"，不持久化，故启动时不再重新解析上次的 txt
	// （上次未完成任务的链接不应自动恢复，见 store.TaskScopeKeys）。

	// ---- 布局 ----
	jsonBox := container.NewVBox(
		container.NewGridWrap(fyne.NewSize(300, 110), p.jsonList),
		container.NewHBox(p.jsonAdd, p.jsonDel),
	)
	txtBox := container.NewVBox(
		container.NewBorder(nil, nil, nil, p.txtBtn, p.txtPathLbl),
		p.txtCountLbl,
	)
	dirBox := container.NewBorder(nil, nil, nil, p.dirBtn, p.dirEntry)

	form := newForm(
		widget.NewFormItem("消息链接", p.linksEntry),
		widget.NewFormItem("txt 批量导入", txtBox),
		widget.NewFormItem("导出 JSON 文件", jsonBox),
		widget.NewFormItem("下载目录", dirBox),
		widget.NewFormItem("每任务线程数", p.threadsEntry),
		widget.NewFormItem("并发任务数", p.limitEntry),
		widget.NewFormItem("附加选项", container.NewVBox(
			p.descCheck,
			p.rewriteCheck,
			p.groupCheck,
			p.skipCheck,
			p.takeoutCheck,
			newHint("提示：--rewrite-ext 会按文件头 MIME 重写扩展名（如 .apk 可能被改名为 .zip）；--takeout 可降低大量下载时的 flood wait。"),
		)),
		widget.NewFormItem("扩展名白名单", p.includeEntry),
		widget.NewFormItem("扩展名黑名单", container.NewVBox(p.excludeEntry, p.conflictHint)),
		widget.NewFormItem("文件名模板", p.templateEntry),
		widget.NewFormItem("断点策略", container.NewVBox(p.resumeRadio, newHint("提示：tdl 在默认模式下检测到未完成任务会交互询问，而 GUI 子进程无法输入，请显式选择续传或重新开始。"))),
		widget.NewFormItem("serve 模式", p.serveCheck),
		widget.NewFormItem("serve 端口", p.portEntry),
	)

	content := container.NewBorder(
		nil,
		newTaskRow(p.startBtn, p.startHint, p.clearBtn),
		nil, nil,
		container.NewVScroll(form),
	)

	p.view = content
	p.refresh()
	return p
}

// toPage 组装为导航页。
func (p *downloadPage) toPage() *Page {
	return &Page{
		Name: "下载",
		View: p.view,
		Controls: []fyne.Disableable{
			p.linksEntry, p.txtBtn,
			p.jsonAdd, p.jsonDel,
			p.dirEntry, p.dirBtn,
			p.threadsEntry, p.limitEntry,
			p.descCheck, p.rewriteCheck, p.groupCheck, p.skipCheck, p.takeoutCheck,
			p.includeEntry, p.excludeEntry,
			p.templateEntry,
			p.resumeRadio,
			p.serveCheck, p.portEntry,
			p.startBtn, p.clearBtn,
		},
		Refresh: p.refresh,
	}
}

// clearTaskInputs 清空全部"任务目标范围"输入：消息链接、txt 导入、JSON 文件列表。
// 三者均不持久化，故仅需清理内存与界面状态。
func (p *downloadPage) clearTaskInputs() {
	p.linksEntry.SetText("")
	p.txtLinks = nil
	p.txtPathLbl.SetText("未选择文件")
	p.txtCountLbl.SetText(txtCountHintDefault)
	p.jsonFiles = nil
	p.jsonSel = -1
	p.jsonList.UnselectAll()
	p.jsonList.Refresh()
	p.ctx.log.AppendBanner("已清空：消息链接、txt 导入、JSON 文件列表")
	p.refresh()
}

// refresh 重算页面可交互状态（校验驱动的 [开始] 启用/禁用、联动禁灰）。
func (p *downloadPage) refresh() {
	if p.startBtn == nil || p.serveCheck == nil {
		return
	}
	conflict := p.includeEntry.Text != "" && p.excludeEntry.Text != ""
	if conflict {
		setRedHint(p.conflictHint, "扩展名白名单与黑名单不能同时填写")
	} else {
		setRedHint(p.conflictHint, "")
	}

	if p.serveCheck.Checked {
		p.portEntry.Enable()
	} else {
		p.portEntry.Disable()
	}
	p.dirEntry.Disable() // 只读：仅可通过 [浏览] 修改

	if p.jsonSel >= 0 {
		p.jsonDel.Enable()
	} else {
		p.jsonDel.Disable()
	}

	// 校验与实际执行同一来源：任何会使 BuildDownload 失败的输入都禁用 [开始] 并给出原因
	reason := ""
	if _, _, err := p.buildArgs(); err != nil {
		p.startBtn.Disable()
		if !conflict { // 字段级红字已说明的冲突不重复提示
			reason = firstLine(err)
		}
	} else {
		p.startBtn.Enable()
	}
	setRedHint(p.startHint, reason)
}

// logTxt 在日志中列出 txt 链接清单。
func (p *downloadPage) logTxt(prefix, path string, links []string) {
	p.ctx.log.AppendBanner(fmt.Sprintf("%s：%s（已解析 %d 条链接）", prefix, path, len(links)))
	for _, l := range links {
		p.ctx.log.AppendLine("    " + l)
	}
}

func (p *downloadPage) onStart() {
	args, links, err := p.buildArgs()
	if err != nil {
		dialog.ShowError(err, p.ctx.win)
		return
	}
	for _, l := range cmdgen.SuspiciousLinks(links) {
		p.ctx.log.AppendBanner("警告：链接格式可能不受支持：" + l)
	}
	tp := p.ctx.tdlPath()
	p.ctx.ctrl.Launch(tp, args, cmdgen.EchoString(tp, args), p.startBtn, p.resetStart)
}

func (p *downloadPage) resetStart() {
	p.startBtn.SetText("开始")
	p.startBtn.Importance = widget.HighImportance
	p.startBtn.OnTapped = func() { p.onStart() }
	p.startBtn.Refresh()
}

func (p *downloadPage) buildArgs() ([]string, []string, error) {
	d := cmdgen.DownloadArgs{
		Links:      strings.Split(p.linksEntry.Text, "\n"),
		TxtLinks:   p.txtLinks,
		JSONFiles:  p.jsonFiles,
		Dir:        p.dirEntry.Text,
		Threads:    p.threadsEntry.Text,
		Limit:      p.limitEntry.Text,
		Desc:       p.descCheck.Checked,
		RewriteExt: p.rewriteCheck.Checked,
		Group:      p.groupCheck.Checked,
		SkipSame:   p.skipCheck.Checked,
		Takeout:    p.takeoutCheck.Checked,
		Include:    strings.TrimSpace(p.includeEntry.Text),
		Exclude:    strings.TrimSpace(p.excludeEntry.Text),
		Template:   p.templateEntry.Text,
		Resume:     resumeValue(p.resumeRadio.Selected),
		Serve:      p.serveCheck.Checked,
		Port:       p.portEntry.Text,
	}
	args, err := cmdgen.BuildDownload(p.ctx.global(), d)
	if err != nil {
		return nil, nil, err
	}
	links := append(append([]string{}, d.Links...), d.TxtLinks...)
	return args, links, nil
}

func (p *downloadPage) pickTxt() {
	pickFile(p.ctx.win, []string{".txt"}, func(path string) {
		links, err := txtload.ParseFile(path)
		p.txtPathLbl.SetText(path)
		if err != nil {
			p.txtLinks = nil
			p.txtCountLbl.SetText("txt 解析失败，请重新选择文件")
			p.ctx.log.AppendError("txt 解析失败：" + err.Error())
			p.refresh()
			return
		}
		p.txtLinks = links
		p.txtCountLbl.SetText(fmt.Sprintf("已解析 %d 条链接", len(links)))
		p.logTxt("txt 批量导入", path, links)
		p.refresh()
	})
}

func (p *downloadPage) addJSON() {
	pickFile(p.ctx.win, []string{".json"}, func(path string) {
		p.addJSONPath(path)
	})
}

// addJSONPath 追加 JSON 文件路径（按路径去重）；对话框回调与单测共用。
func (p *downloadPage) addJSONPath(path string) {
	for _, f := range p.jsonFiles {
		if f == path {
			return
		}
	}
	p.jsonFiles = append(p.jsonFiles, path)
	p.jsonList.Refresh()
	p.refresh()
}

func (p *downloadPage) removeJSON() {
	if p.jsonSel < 0 || p.jsonSel >= len(p.jsonFiles) {
		return
	}
	p.jsonFiles = append(p.jsonFiles[:p.jsonSel], p.jsonFiles[p.jsonSel+1:]...)
	p.jsonSel = -1
	p.jsonList.UnselectAll()
	p.jsonList.Refresh()
	p.refresh()
}
