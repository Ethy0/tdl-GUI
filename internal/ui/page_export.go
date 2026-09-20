package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"tdl-gui/internal/cmdgen"
	"tdl-gui/internal/store"
)

// 导出类型单选项（映射 cmdgen.ExportType*）。
var exportTypeLabels = []string{"按时间（time）", "按消息 ID（id）", "最近 N 条（last）"}

func exportTypeValue(label string) string {
	switch label {
	case exportTypeLabels[1]:
		return cmdgen.ExportTypeID
	case exportTypeLabels[2]:
		return cmdgen.ExportTypeLast
	}
	return cmdgen.ExportTypeTime
}

func exportTypeLabel(v string) string {
	switch v {
	case cmdgen.ExportTypeID:
		return exportTypeLabels[1]
	case cmdgen.ExportTypeLast:
		return exportTypeLabels[2]
	}
	return exportTypeLabels[0]
}

func exportRangePlaceholder(v string) string {
	switch v {
	case cmdgen.ExportTypeID:
		return "起,止消息 ID，如 100,200（留空 = 全部）"
	case cmdgen.ExportTypeLast:
		return "数量 N，如 100"
	}
	return "起,止 Unix 时间戳，如 1700000000,1700086400（留空 = 全部）"
}

type exportPage struct {
	ctx  *pageCtx
	view fyne.CanvasObject

	chatEntry  *widget.Entry
	topicEntry *widget.Entry
	replyEntry *widget.Entry

	outputEntry *widget.Entry
	outputBtn   *widget.Button

	typeRadio   *widget.RadioGroup
	rangeEntry  *widget.Entry
	filterEntry *widget.Entry

	withContentCheck *widget.Check
	allCheck         *widget.Check
	rawCheck         *widget.Check

	startBtn  *widget.Button
	startHint *canvas.Text
}

func newExportPage(ctx *pageCtx) *exportPage {
	p := &exportPage{ctx: ctx}
	st := ctx.st

	// 目标聊天/话题/回复/输出/类型/范围/过滤属"任务目标范围"：不持久化，启动时为空/默认
	p.chatEntry = widget.NewEntry()
	p.chatEntry.SetPlaceHolder("聊天 ID/域名/链接（留空 = 收藏夹）")
	p.chatEntry.SetText("")

	p.topicEntry = widget.NewEntry()
	p.topicEntry.SetPlaceHolder("话题 ID（可选）")
	positiveIntEntry(p.topicEntry)
	p.topicEntry.SetText("")

	p.replyEntry = widget.NewEntry()
	p.replyEntry.SetPlaceHolder("评论区帖子 ID（可选）")
	positiveIntEntry(p.replyEntry)
	p.replyEntry.SetText("")

	p.outputEntry = newReadOnlyEntry()
	p.outputEntry.SetText(cmdgen.DefaultExportOutput)
	p.outputBtn = widget.NewButton("另存为…", nil)

	p.typeRadio = widget.NewRadioGroup(exportTypeLabels, nil)
	p.typeRadio.Horizontal = true
	p.typeRadio.SetSelected(exportTypeLabel(cmdgen.DefaultExportType))

	p.rangeEntry = widget.NewEntry()
	p.rangeEntry.SetText("")
	p.rangeEntry.SetPlaceHolder(exportRangePlaceholder(cmdgen.DefaultExportType))
	p.rangeEntry.Validator = func(s string) error { return cmdgen.ValidateRange(s) }

	p.filterEntry = widget.NewEntry()
	p.filterEntry.SetPlaceHolder(`如 Views>200 && Media.Name endsWith '.zip'（留空 = 匹配全部）`)
	p.filterEntry.SetText("")

	p.withContentCheck = widget.NewCheck("附带消息内容（--with-content）", nil)
	p.withContentCheck.SetChecked(st.Bool(store.KeyExportWithContent, false))
	p.allCheck = widget.NewCheck("导出全部消息（含非媒体，--all）", nil)
	p.allCheck.SetChecked(st.Bool(store.KeyExportAll, false))
	p.rawCheck = widget.NewCheck("导出原始 MTProto 结构（--raw，调试用）", nil)
	p.rawCheck.SetChecked(st.Bool(store.KeyExportRaw, false))

	p.startBtn = widget.NewButton("开始", nil)
	p.startBtn.Importance = widget.HighImportance
	p.startHint = newRedHint()

	// ---- 事件绑定（目标范围字段不持久化，仅做界面联动）----
	p.chatEntry.OnChanged = func(s string) {}
	p.topicEntry.OnChanged = func(s string) {
		p.refresh()
	}
	p.replyEntry.OnChanged = func(s string) {
		p.refresh()
	}
	p.outputBtn.OnTapped = func() {
		pickSaveFile(ctx.win, cmdgen.DefaultExportOutput, func(path string) {
			p.outputEntry.SetText(path)
		})
	}
	p.typeRadio.OnChanged = func(label string) {
		p.rangeEntry.SetPlaceHolder(exportRangePlaceholder(exportTypeValue(label)))
	}
	p.rangeEntry.OnChanged = func(s string) {
		p.refresh()
	}
	p.filterEntry.OnChanged = func(s string) {}
	p.withContentCheck.OnChanged = func(v bool) { st.SetBool(store.KeyExportWithContent, v) }
	p.allCheck.OnChanged = func(v bool) { st.SetBool(store.KeyExportAll, v) }
	p.rawCheck.OnChanged = func(v bool) { st.SetBool(store.KeyExportRaw, v) }
	p.startBtn.OnTapped = func() { p.onStart() }

	// ---- 布局 ----
	outputBox := container.NewBorder(nil, nil, nil, p.outputBtn, p.outputEntry)

	form := newForm(
		widget.NewFormItem("目标聊天", p.chatEntry),
		widget.NewFormItem("话题 ID", p.topicEntry),
		widget.NewFormItem("评论区帖子 ID", p.replyEntry),
		widget.NewFormItem("输出文件", outputBox),
		widget.NewFormItem("导出类型", p.typeRadio),
		widget.NewFormItem("范围", p.rangeEntry),
		widget.NewFormItem("过滤表达式", p.filterEntry),
		widget.NewFormItem("附加选项", container.NewVBox(
			p.withContentCheck,
			p.allCheck,
			p.rawCheck,
		)),
	)

	content := container.NewBorder(
		nil,
		newTaskRow(p.startBtn, p.startHint),
		nil, nil,
		container.NewVScroll(form),
	)

	p.view = content
	p.refresh()
	return p
}

// toPage 组装为导航页。
func (p *exportPage) toPage() *Page {
	return &Page{
		Name: "导出消息",
		View: p.view,
		Controls: []fyne.Disableable{
			p.chatEntry, p.topicEntry, p.replyEntry,
			p.outputEntry, p.outputBtn,
			p.typeRadio, p.rangeEntry, p.filterEntry,
			p.withContentCheck, p.allCheck, p.rawCheck,
			p.startBtn,
		},
		Refresh: p.refresh,
	}
}

func (p *exportPage) refresh() {
	if p.startBtn == nil {
		return
	}
	p.outputEntry.Disable() // 只读：仅可通过 [另存为] 修改

	// 校验与实际执行同一来源：任何会使 BuildExport 失败的输入都禁用 [开始] 并给出原因
	reason := ""
	if _, err := p.collect(); err != nil {
		p.startBtn.Disable()
		reason = firstLine(err)
	} else {
		p.startBtn.Enable()
	}
	setRedHint(p.startHint, reason)
}

// collect 收集表单并拼装命令行（refresh 与执行共用同一校验来源）。
func (p *exportPage) collect() ([]string, error) {
	e := cmdgen.ExportArgs{
		Chat:        strings.TrimSpace(p.chatEntry.Text),
		Topic:       strings.TrimSpace(p.topicEntry.Text),
		Reply:       strings.TrimSpace(p.replyEntry.Text),
		Output:      strings.TrimSpace(p.outputEntry.Text),
		Type:        exportTypeValue(p.typeRadio.Selected),
		Range:       strings.TrimSpace(p.rangeEntry.Text),
		Filter:      p.filterEntry.Text,
		WithContent: p.withContentCheck.Checked,
		All:         p.allCheck.Checked,
		Raw:         p.rawCheck.Checked,
	}
	return cmdgen.BuildExport(p.ctx.global(), e)
}

func (p *exportPage) onStart() {
	args, err := p.collect()
	if err != nil {
		dialog.ShowError(err, p.ctx.win)
		return
	}
	tp := p.ctx.tdlPath()
	p.ctx.ctrl.Launch(tp, args, cmdgen.EchoString(tp, args), p.startBtn, p.resetStart)
}

func (p *exportPage) resetStart() {
	p.startBtn.SetText("开始")
	p.startBtn.Importance = widget.HighImportance
	p.startBtn.OnTapped = func() { p.onStart() }
	p.startBtn.Refresh()
}
