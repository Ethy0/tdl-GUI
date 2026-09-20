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

type uploadPage struct {
	ctx  *pageCtx
	view fyne.CanvasObject

	pathsList *widget.List
	addFile   *widget.Button
	addDir    *widget.Button
	delPath   *widget.Button
	paths     []string
	pathSel   int

	chatEntry  *widget.Entry
	topicEntry *widget.Entry
	toEntry    *widget.Entry
	toHint     *canvas.Text

	threadsEntry *widget.Entry
	limitEntry   *widget.Entry

	captionEntry *widget.Entry

	includeEntry *widget.Entry
	excludeEntry *widget.Entry
	conflictHint *canvas.Text

	rmCheck    *widget.Check
	photoCheck *widget.Check

	startBtn  *widget.Button
	startHint *canvas.Text
}

func newUploadPage(ctx *pageCtx) *uploadPage {
	p := &uploadPage{ctx: ctx, pathSel: -1}
	st := ctx.st

	p.pathsList, p.delPath = newPathListView(
		func() []string { return p.paths },
		func() int { return p.pathSel },
		func(i int) { p.pathSel = i },
		func() { p.removePath() },
		func() bool { return ctx.ctrl.Running() }, // 执行期间不接受选中（widget.List 不可 Disable）
	)
	// 待上传路径列表属"任务目标范围"：不持久化，启动时始终为空（见 store.TaskScopeKeys）
	p.paths = nil
	p.addFile = widget.NewButton("添加文件…", nil)
	p.addDir = widget.NewButton("添加目录…", nil)

	p.chatEntry = widget.NewEntry()
	p.chatEntry.SetPlaceHolder("@iyear / iyear / 123456789 / https://t.me/iyear（留空 = 收藏夹）")
	p.chatEntry.SetText("")

	p.topicEntry = widget.NewEntry()
	p.topicEntry.SetPlaceHolder("论坛话题 ID（可选，需配合目标聊天）")
	positiveIntEntry(p.topicEntry)
	p.topicEntry.SetText("")

	p.toEntry = widget.NewEntry()
	p.toEntry.SetPlaceHolder("消息路由表达式或路由文件路径（与目标聊天/话题互斥；留空 = 不使用）")
	p.toEntry.SetText("")
	p.toHint = newRedHint()

	p.threadsEntry = widget.NewEntry()
	p.threadsEntry.SetPlaceHolder("留空 = tdl 默认（4）")
	positiveIntEntry(p.threadsEntry)
	p.threadsEntry.SetText(st.Str(store.KeyUploadThreads, ""))

	p.limitEntry = widget.NewEntry()
	p.limitEntry.SetPlaceHolder("留空 = tdl 默认（2）")
	positiveIntEntry(p.limitEntry)
	p.limitEntry.SetText(st.Str(store.KeyUploadLimit, ""))

	p.captionEntry = widget.NewMultiLineEntry()
	p.captionEntry.SetPlaceHolder("标题表达式或文件路径（留空 = tdl 默认）")
	p.captionEntry.SetMinRowsVisible(3)
	p.captionEntry.SetText(st.Str(store.KeyUploadCaption, ""))

	p.includeEntry = widget.NewEntry()
	p.includeEntry.SetPlaceHolder("如 jpg,png（留空 = 不限制）")
	p.includeEntry.SetText(st.Str(store.KeyUploadInclude, ""))
	p.excludeEntry = widget.NewEntry()
	p.excludeEntry.SetPlaceHolder("如 mp4,flv（留空 = 不排除）")
	p.excludeEntry.SetText(st.Str(store.KeyUploadExclude, ""))
	p.conflictHint = newRedHint()

	p.rmCheck = widget.NewCheck("上传成功后删除本地文件（--rm，危险）", nil)
	p.rmCheck.SetChecked(st.Bool(store.KeyUploadRM, false))
	p.photoCheck = widget.NewCheck("图片以照片形式发送（--photo）", nil)
	p.photoCheck.SetChecked(st.Bool(store.KeyUploadPhoto, false))

	p.startBtn = widget.NewButton("开始", nil)
	p.startBtn.Importance = widget.HighImportance
	p.startHint = newRedHint()

	// ---- 事件绑定 ----
	p.addFile.OnTapped = func() {
		pickFile(ctx.win, nil, func(path string) {
			p.addPath(path)
		})
	}
	p.addDir.OnTapped = func() {
		pickFolder(ctx.win, func(path string) {
			p.addPath(path)
		})
	}
	p.chatEntry.OnChanged = func(s string) {
		p.refresh() // 目标聊天属任务目标范围，不持久化
	}
	p.topicEntry.OnChanged = func(s string) {
		p.refresh() // 话题 ID 属任务目标范围，不持久化
	}
	p.toEntry.OnChanged = func(s string) {
		p.refresh() // 消息路由属任务目标范围，不持久化
	}
	p.threadsEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyUploadThreads, s)
		p.refresh() // 数字非法需即时禁用 [开始]
	}
	p.limitEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyUploadLimit, s)
		p.refresh()
	}
	p.captionEntry.OnChanged = func(s string) { st.SetStr(store.KeyUploadCaption, s) }
	p.includeEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyUploadInclude, s)
		p.refresh()
	}
	p.excludeEntry.OnChanged = func(s string) {
		st.SetStr(store.KeyUploadExclude, s)
		p.refresh()
	}
	p.rmCheck.OnChanged = func(v bool) { st.SetBool(store.KeyUploadRM, v) }
	p.photoCheck.OnChanged = func(v bool) { st.SetBool(store.KeyUploadPhoto, v) }
	p.startBtn.OnTapped = func() { p.onStart() }

	// ---- 布局 ----
	pathsBox := container.NewVBox(
		container.NewGridWrap(fyne.NewSize(300, 130), p.pathsList),
		container.NewHBox(p.addFile, p.addDir, p.delPath),
	)

	form := newForm(
		widget.NewFormItem("待上传文件/目录", pathsBox),
		widget.NewFormItem("目标聊天", p.chatEntry),
		widget.NewFormItem("论坛话题 ID", p.topicEntry),
		widget.NewFormItem("消息路由", container.NewVBox(
			p.toEntry,
			newHint("提示：消息路由与目标聊天/话题互斥，只能二选一；填写消息路由后，另两项将被禁用。"),
			p.toHint,
		)),
		widget.NewFormItem("每任务线程数", p.threadsEntry),
		widget.NewFormItem("并发任务数", p.limitEntry),
		widget.NewFormItem("标题（caption）", p.captionEntry),
		widget.NewFormItem("扩展名白名单", p.includeEntry),
		widget.NewFormItem("扩展名黑名单", container.NewVBox(p.excludeEntry, p.conflictHint)),
		widget.NewFormItem("附加选项", container.NewVBox(
			p.rmCheck,
			newHintColored("警告：勾选 --rm 后，上传成功的本地文件将被删除且不可恢复，执行前会再次确认。", colorLogError),
			p.photoCheck,
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
func (p *uploadPage) toPage() *Page {
	return &Page{
		Name: "上传",
		View: p.view,
		Controls: []fyne.Disableable{
			p.addFile, p.addDir, p.delPath,
			p.chatEntry, p.topicEntry, p.toEntry,
			p.threadsEntry, p.limitEntry,
			p.captionEntry,
			p.includeEntry, p.excludeEntry,
			p.rmCheck, p.photoCheck,
			p.startBtn,
		},
		Refresh: p.refresh,
	}
}

func (p *uploadPage) refresh() {
	if p.startBtn == nil {
		return
	}

	// 消息路由与目标聊天/话题互斥：to 非空时禁用二者，二者已有内容时冲突
	toSet := strings.TrimSpace(p.toEntry.Text) != ""
	conflictTo := toSet && (strings.TrimSpace(p.chatEntry.Text) != "" || strings.TrimSpace(p.topicEntry.Text) != "")
	if toSet {
		p.chatEntry.Disable()
		p.topicEntry.Disable()
	} else {
		p.chatEntry.Enable()
		p.topicEntry.Enable()
	}
	if conflictTo {
		setRedHint(p.toHint, "消息路由与目标聊天/话题互斥，只能二选一")
	} else {
		setRedHint(p.toHint, "")
	}

	if p.includeEntry.Text != "" && p.excludeEntry.Text != "" {
		setRedHint(p.conflictHint, "扩展名白名单与黑名单不能同时填写")
	} else {
		setRedHint(p.conflictHint, "")
	}

	if p.pathSel >= 0 {
		p.delPath.Enable()
	} else {
		p.delPath.Disable()
	}

	// 校验与实际执行同一来源：任何会使 BuildUpload 失败的输入都禁用 [开始] 并给出原因
	reason := ""
	if _, _, err := p.collect(); err != nil {
		p.startBtn.Disable()
		switch {
		case conflictTo:
			reason = "消息路由与目标聊天/话题互斥，只能二选一"
		case p.includeEntry.Text != "" && p.excludeEntry.Text != "":
			reason = "扩展名白名单与黑名单不能同时填写"
		default:
			reason = firstLine(err)
		}
	} else {
		p.startBtn.Enable()
	}
	setRedHint(p.startHint, reason)
}

func (p *uploadPage) addPath(path string) {
	for _, existing := range p.paths {
		if existing == path {
			return
		}
	}
	p.paths = append(p.paths, path)
	p.pathsList.Refresh()
	p.refresh()
}

func (p *uploadPage) removePath() {
	if p.pathSel < 0 || p.pathSel >= len(p.paths) {
		return
	}
	p.paths = append(p.paths[:p.pathSel], p.paths[p.pathSel+1:]...)
	p.pathSel = -1
	p.pathsList.UnselectAll()
	p.pathsList.Refresh()
	p.refresh()
}

// collect 收集表单并拼装命令行（refresh 与执行共用同一校验来源）。
func (p *uploadPage) collect() (cmdgen.UploadArgs, []string, error) {
	u := cmdgen.UploadArgs{
		Paths:   p.paths,
		Chat:    strings.TrimSpace(p.chatEntry.Text),
		Topic:   strings.TrimSpace(p.topicEntry.Text),
		To:      strings.TrimSpace(p.toEntry.Text),
		Threads: p.threadsEntry.Text,
		Limit:   p.limitEntry.Text,
		Caption: p.captionEntry.Text,
		Include: strings.TrimSpace(p.includeEntry.Text),
		Exclude: strings.TrimSpace(p.excludeEntry.Text),
		RM:      p.rmCheck.Checked,
		Photo:   p.photoCheck.Checked,
	}
	args, err := cmdgen.BuildUpload(p.ctx.global(), u)
	return u, args, err
}

func (p *uploadPage) onStart() {
	_, args, err := p.collect()
	if err != nil {
		dialog.ShowError(err, p.ctx.win)
		return
	}

	if p.rmCheck.Checked {
		dialog.ShowConfirm("危险操作确认", "上传成功后将删除本地文件（--rm），此操作不可恢复。确认继续？",
			func(ok bool) {
				if ok {
					p.launch(args)
				}
			}, p.ctx.win)
		return
	}
	p.launch(args)
}

func (p *uploadPage) launch(args []string) {
	tp := p.ctx.tdlPath()
	p.ctx.ctrl.Launch(tp, args, cmdgen.EchoString(tp, args), p.startBtn, p.resetStart)
}

func (p *uploadPage) resetStart() {
	p.startBtn.SetText("开始")
	p.startBtn.Importance = widget.HighImportance
	p.startBtn.OnTapped = func() { p.onStart() }
	p.startBtn.Refresh()
}
