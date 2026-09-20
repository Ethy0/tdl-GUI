package ui

import (
	"image/color"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mattn/go-runewidth"
)

const (
	// progressInterval 进度行渲染节流（每 100ms 最多真正渲染一次，只保留最新值）。
	progressInterval = 100 * time.Millisecond
	// gridRefreshInterval 日志重绘节流：合并高频追加，避免大日志整表重绘拖垮 UI 线程。
	gridRefreshInterval = 50 * time.Millisecond
	// maxLines 日志行数上限；超过时丢弃最旧 trimLines 行并整表重建。
	maxLines  = 5000
	trimLines = 2000
	// logTabWidth 制表符展开宽度（与 TextGrid 默认制表位一致）。
	logTabWidth = 8
	// maxLogCols 单行最大显示列数：超出部分仅截断渲染（不影响 lines 原文与复制）。
	// TextGrid 无换行能力，最小宽度由最宽行决定；不封顶会把主窗口撑宽且无法缩小（D-13）。
	maxLogCols = 200
)

var (
	colorLogBG     = color.NRGBA{R: 0x1E, G: 0x1E, B: 0x1E, A: 0xFF}
	colorLogText   = color.NRGBA{R: 0xD4, G: 0xD4, B: 0xD4, A: 0xFF}
	colorLogBanner = color.NRGBA{R: 0xFF, G: 0xB8, B: 0x6C, A: 0xFF}
	colorLogError  = color.NRGBA{R: 0xFF, G: 0x55, B: 0x55, A: 0xFF}
)

type logStyle int

const (
	logStyleNormal logStyle = iota
	logStyleBanner
	logStyleError
)

func gridStyleFor(st logStyle) *widget.CustomTextGridStyle {
	fg := colorLogText
	switch st {
	case logStyleBanner:
		fg = colorLogBanner
	case logStyleError:
		fg = colorLogError
	}
	return &widget.CustomTextGridStyle{
		FGColor: fg,
		// 不设 BGColor：单元格背景矩形必须保持透明，否则右侧单元格的不透明底色
		// 会盖住宽字符（CJK/全角）字形向右溢出的半边（见 doc/fix-plan 本方案 §2.2）。
		// 整块日志区底色由 NewLogView 中垫底的 canvas.Rectangle(colorLogBG) 提供。
		TextStyle: fyne.TextStyle{Monospace: true},
	}
}

type logLine struct {
	text  string
	style logStyle
}

// LogView 是运行日志黑框：实时流式输出、\r 行内刷新、5000 行上限、复制/清空/自动滚动。
//
// 所有方法都必须在 Fyne 主线程语义下调用（TaskController 内部已用 fyne.Do 派发）。
// mu 串行化全部状态访问：生产环境所有调用都在主线程（无争用）；节流重绘的
// 定时器回调经 uiDo 派发，在测试环境下（fyne.Do 直接执行）也不会与调用方并发。
type LogView struct {
	mu sync.Mutex

	app    fyne.App
	grid   *widget.TextGrid
	scroll *container.Scroll
	auto   *widget.Check
	view   fyne.CanvasObject

	lines       []logLine
	progress    string
	hasProgress bool
	renderedLen int       // 上一次进度行长度（rune 计），用于整行覆盖旧字符
	lastRender  time.Time // 进度节流

	// 重绘节流字段跨 goroutine 访问（AfterFunc 回调在独立 goroutine 启动），
	// 用原子类型保证无数据竞争；lastGridRender 为 UnixNano。
	lastGridRender atomic.Int64
	refreshPending atomic.Bool
}

// NewLogView 创建日志组件（顶部工具行 + 黑框）。
func NewLogView(a fyne.App) *LogView {
	l := &LogView{app: a}

	l.grid = widget.NewTextGrid()

	bg := canvas.NewRectangle(colorLogBG)
	bg.SetMinSize(fyne.NewSize(400, 200)) // 日志区最小高度保护
	// 双向滚动：水平最小尺寸不随内容增长，日志内容永不影响主窗口宽度（D-13 兜底）。
	l.scroll = container.NewScroll(container.NewStack(bg, l.grid))

	l.auto = widget.NewCheck("自动滚动", func(bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.autoScroll()
	})
	l.auto.SetChecked(true)

	copyBtn := widget.NewButton("复制全部", func() { l.CopyAll() })
	clearBtn := widget.NewButton("清空", func() { l.Clear() })
	toolbar := container.NewHBox(l.auto, copyBtn, clearBtn)

	l.view = container.NewBorder(toolbar, nil, nil, nil, l.scroll)
	return l
}

// CanvasObject 返回日志组件的可放置对象（运行期间保持可用，不参与全局锁定）。
func (l *LogView) CanvasObject() fyne.CanvasObject { return l.view }

// AppendLine 追加一行普通日志。
func (l *LogView) AppendLine(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.appendStyled(line, logStyleNormal)
}

// AppendBanner 追加强调行（命令回显、正常退出码等，橙色）。
func (l *LogView) AppendBanner(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.appendStyled(text, logStyleBanner)
}

// AppendError 追加错误/中止行（红色）。
func (l *LogView) AppendError(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.appendStyled(text, logStyleError)
}

// UpdateProgress 行内刷新进度行（\r 触发的整行替换，带节流）。
func (l *LogView) UpdateProgress(line string) {
	if line == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// 同一帧会先后命中多行：文件进度行（含 %）与 overall 聚合行（无 %）。
	// overall 行不得覆盖已存的文件进度行，否则最终固化的不是真实下载进度（D-16）。
	if l.hasProgress && strings.Contains(l.progress, "%") && !strings.Contains(line, "%") {
		return
	}
	l.progress = line
	l.hasProgress = true
	if now := time.Now(); now.Sub(l.lastRender) >= progressInterval {
		l.lastRender = now
		l.renderProgress()
	}
}

// CommitProgress 固化残留进度行（任务结束时调用）。
func (l *LogView) CommitProgress() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.hasProgress {
		return
	}
	l.commitProgress()
	l.requestRefresh()
}

// CopyAll 复制全部日志到剪贴板。
func (l *LogView) CopyAll() {
	if l.app == nil {
		return
	}
	l.app.Clipboard().SetContent(strings.Join(l.Lines(), "\n"))
}

// Clear 清空日志缓冲（仅 UI 缓冲，不影响运行中的任务）。
func (l *LogView) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = nil
	l.progress = ""
	l.hasProgress = false
	l.renderedLen = 0
	l.grid.SetText("")
	l.grid.Refresh()
}

// Lines 返回已提交日志的快照（测试用）。
func (l *LogView) Lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.lines))
	for _, ln := range l.lines {
		out = append(out, ln.text)
	}
	return out
}

func (l *LogView) appendStyled(text string, st logStyle) {
	if l.hasProgress {
		l.commitProgress()
	}
	l.lines = append(l.lines, logLine{text: text, style: st})

	if len(l.lines) > maxLines {
		drop := trimLines
		if drop >= len(l.lines) {
			drop = len(l.lines) - 1
		}
		l.lines = append([]logLine(nil), l.lines[drop:]...)
		l.rebuild()
	} else {
		l.setRow(len(l.lines)-1, text, st)
	}
	l.requestRefresh()
}

// requestRefresh 节流重绘日志表：高频追加时合并为每 gridRefreshInterval 一次。
func (l *LogView) requestRefresh() {
	now := time.Now().UnixNano()
	if now-l.lastGridRender.Load() >= int64(gridRefreshInterval) {
		l.lastGridRender.Store(now)
		l.grid.Refresh()
		l.autoScroll()
		return
	}
	if !l.refreshPending.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(gridRefreshInterval, func() {
		uiDo(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.refreshPending.Store(false)
			l.lastGridRender.Store(time.Now().UnixNano())
			l.grid.Refresh()
			l.autoScroll()
		})
	})
}

func (l *LogView) commitProgress() {
	s := strings.TrimRight(l.progress, " ")
	l.progress = ""
	l.renderedLen = 0
	l.hasProgress = false
	if s == "" {
		// 清掉临时进度行
		l.grid.SetRow(len(l.lines), widget.TextGridRow{Style: gridStyleFor(logStyleNormal)})
		return
	}
	l.lines = append(l.lines, logLine{text: s, style: logStyleNormal})
	l.setRow(len(l.lines)-1, s, logStyleNormal)
}

func (l *LogView) renderProgress() {
	text := truncateToCols(l.progress)
	// 整行覆盖：按显示列数补齐空格，避免旧字符残留；补格上限 maxLogCols，
	// 否则超宽帧会让进度行被永久锁在历史最大宽度（D-13）。
	prev := l.renderedLen
	if prev > maxLogCols {
		prev = maxLogCols
	}
	if n := cellWidth(text); n < prev {
		text += strings.Repeat(" ", prev-n)
	}
	l.renderedLen = cellWidth(l.progress)
	l.setRow(len(l.lines), text, logStyleNormal)
	l.requestRefresh()
}

func (l *LogView) setRow(idx int, text string, st logStyle) {
	l.grid.SetRow(idx, makeRow(truncateToCols(text), st))
	l.grid.SetRowStyle(idx, gridStyleFor(st))
}

// truncateToCols 按显示列数截断文本（仅渲染层使用，不修改 lines 缓冲原文）。
// 超宽行只截断显示并以省略号提示；复制（CopyAll）仍导出完整原文。
func truncateToCols(s string) string {
	if cellWidth(s) <= maxLogCols {
		return s
	}
	col := 0
	var b strings.Builder
	for _, r := range s {
		w := runeWidth(r)
		if r == '\t' {
			w = logTabWidth - col%logTabWidth
		}
		if col+w > maxLogCols-1 { // 预留 1 列放置省略号
			break
		}
		b.WriteRune(r)
		col += w
	}
	b.WriteRune('…')
	return b.String()
}

func (l *LogView) rebuild() {
	l.grid.SetText("")
	for i, ln := range l.lines {
		l.setRow(i, ln.text, ln.style)
	}
}

func (l *LogView) autoScroll() {
	if l.auto != nil && l.auto.Checked && l.scroll != nil {
		l.scroll.ScrollToBottom()
	}
}

func makeRow(s string, st logStyle) widget.TextGridRow {
	return widget.TextGridRow{Cells: makeCells(s), Style: gridStyleFor(st)}
}

// makeCells 把字符串转换为"一格一列"的单元格序列。
//
// TextGrid 约定一个单元格即一列，宽度为 2 的字符（汉字、全角符号、Emoji 等）
// 必须再补一个空格单元格，否则字形会被相邻字符覆盖。
func makeCells(s string) []widget.TextGridCell {
	cells := make([]widget.TextGridCell, 0, len(s))
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := logTabWidth - col%logTabWidth
			for i := 0; i < n; i++ {
				cells = append(cells, widget.TextGridCell{Rune: ' '})
			}
			col += n
			continue
		}
		cells = append(cells, widget.TextGridCell{Rune: r})
		w := runeWidth(r)
		for i := 1; i < w; i++ {
			cells = append(cells, widget.TextGridCell{Rune: ' '})
		}
		col += w
	}
	return cells
}

// cellWidth 返回字符串占用的显示列数（与 makeCells 保持一致）。
func cellWidth(s string) int {
	w := 0
	for _, r := range s {
		if r == '\t' {
			w += logTabWidth - w%logTabWidth
			continue
		}
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	return 1 // 组合符等零宽字符按 1 列处理，保证行宽可预测
}
