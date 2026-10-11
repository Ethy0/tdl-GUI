package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// 导航组件常量（v1.1.0 nav-switch-animation 方案）。
const (
	// navItemHeight 单个导航项高度（与旧版 GridWrap 一致）。
	navItemHeight = 42
	// navPillInset 胶囊相对导航项的内缩边距。
	navPillInset = 5
	// pillAnimDuration 胶囊滑动动画时长（已确认 200ms）。
	pillAnimDuration = 200 * time.Millisecond
)

// NavItem 是导航项的测试句柄（不参与绘制，仅用于黑盒测试定位/点击/查禁用态）。
//
// 注意：导航文字必须是裸 *canvas.Text 放进渲染器，因为 Fyne 绘制器用
// `case *canvas.Text:` 类型断言分派，任何包装结构体都不会被绘制。
type NavItem struct {
	Name  string
	index int
	nav   *navSwitcher
}

// Disabled 返回所属导航的禁用态（任务运行中为 true）。
func (t *NavItem) Disabled() bool { return t.nav != nil && t.nav.Disabled() }

// Tapped 实现 fyne.Tappable：模拟点击该导航项（供 test.Tap 使用）。
func (t *NavItem) Tapped(*fyne.PointEvent) {
	if t.nav != nil {
		t.nav.Select(t.index)
	}
}

// navSwitcher 左侧竖排导航：圆角胶囊指示器随选中项平滑滑动（200ms，EaseOut）。
// 实现 fyne.Disableable：任务运行中整体变灰、点击与键盘均不可交互；
// 实现 fyne.Tappable：按点击纵坐标取导航项；
// 实现 fyne.Focusable：聚焦后可用上下键切换。
type navSwitcher struct {
	widget.BaseWidget
	items    []string
	handles  []*NavItem
	onSelect func(int)
	current  int
	disabled bool
	instant  bool                 // 下一次 Refresh 时胶囊直接落位（首次启动用）
	r        *navSwitcherRenderer // CreateRenderer 时填充
}

// NavSwitcher 是 navSwitcher 的导出别名（供测试定位）；业务代码请用 newNavSwitcher。
type NavSwitcher = navSwitcher

// PillY 返回胶囊当前 y 坐标（-1 表示渲染器尚未创建）。供测试断言滑动结果。
func (s *navSwitcher) PillY() float32 {
	if s.r == nil {
		return -1
	}
	return s.r.pillY
}

// PillYFor 返回第 i 项对应的胶囊目标 y 坐标（测试用）。
func (s *navSwitcher) PillYFor(i int) float32 {
	return float32(i)*navItemHeight + navPillInset
}

// Renderer 返回渲染器（未创建时为 nil）。供黑盒测试下钻结构。
func (s *navSwitcher) Renderer() fyne.WidgetRenderer { return s.r }

// Items 返回全部导航项句柄（测试用，顺序与页面一致）。
func (s *navSwitcher) Items() []*NavItem { return s.handles }

// newNavSwitcher 创建导航组件；onSelect 在用户选择某项时回调（点击与键盘统一入口）。
func newNavSwitcher(items []string, onSelect func(int)) *navSwitcher {
	s := &navSwitcher{items: items, onSelect: onSelect}
	s.ExtendBaseWidget(s)
	for i, name := range items {
		s.handles = append(s.handles, &NavItem{Name: name, index: i, nav: s})
	}
	return s
}

// SetCurrent 更新选中项（不触发 onSelect 回调），胶囊平滑滑动到目标位置。
func (s *navSwitcher) SetCurrent(i int) {
	if i < 0 || i >= len(s.items) || i == s.current {
		return
	}
	s.current = i
	s.Refresh()
}

// SetCurrentInstant 立即切换选中项（胶囊不播动画），用于首次启动。
func (s *navSwitcher) SetCurrentInstant(i int) {
	if i < 0 || i >= len(s.items) || i == s.current {
		return
	}
	s.current = i
	s.instant = true
	s.Refresh()
}

// Select 选中某项（用户点击/键盘统一入口）。
func (s *navSwitcher) Select(i int) {
	if i < 0 || i >= len(s.items) || i == s.current || s.disabled {
		return
	}
	s.current = i
	s.Refresh()
	if s.onSelect != nil {
		s.onSelect(i)
	}
}

// ---------- fyne.Disableable ----------

func (s *navSwitcher) Disable() {
	if s.disabled {
		return
	}
	s.disabled = true
	s.Refresh()
}

func (s *navSwitcher) Enable() {
	if !s.disabled {
		return
	}
	s.disabled = false
	s.Refresh()
}

func (s *navSwitcher) Disabled() bool { return s.disabled }

// ---------- fyne.Tappable ----------

// Tapped 按点击的纵向位置换算导航项索引（组件铺满左侧整列）。
func (s *navSwitcher) Tapped(ev *fyne.PointEvent) {
	if s.disabled || ev == nil {
		return
	}
	s.Select(int(ev.Position.Y / navItemHeight))
}

// ---------- fyne.Focusable ----------

func (s *navSwitcher) TypedKey(e *fyne.KeyEvent) {
	if s.disabled || s.onSelect == nil {
		return
	}
	switch e.Name {
	case fyne.KeyUp:
		if s.current > 0 {
			s.Select(s.current - 1)
		}
	case fyne.KeyDown:
		if s.current < len(s.items)-1 {
			s.Select(s.current + 1)
		}
	}
}

func (s *navSwitcher) TypedRune(rune) {}

func (s *navSwitcher) FocusGained() { s.Refresh() }

func (s *navSwitcher) FocusLost() { s.Refresh() }

// ---------- widget.Widget ----------

func (s *navSwitcher) CreateRenderer() fyne.WidgetRenderer {
	r := &navSwitcherRenderer{sw: s}
	s.r = r
	r.pill = canvas.NewRectangle(theme.PrimaryColor())
	r.pill.CornerRadius = (navItemHeight - 2*navPillInset) / 2
	for _, name := range s.items {
		// 必须是裸 *canvas.Text：包装类型会导致 Fyne 绘制器类型断言失败（不绘制）
		t := canvas.NewText(name, theme.Color(theme.ColorNameForeground))
		t.TextSize = theme.TextSize() + 1
		t.TextStyle = fyne.TextStyle{Bold: true}
		t.Alignment = fyne.TextAlignCenter
		r.labels = append(r.labels, t)
	}
	return r
}

type navSwitcherRenderer struct {
	sw         *navSwitcher
	pill       *canvas.Rectangle
	labels     []*canvas.Text
	pillY      float32 // 胶囊当前 y（动画插值基础，坐标相对组件左上角）
	animTarget float32 // 当前动画的目标 y（用于动画中目标变更时续走）
	anim       *fyne.Animation
}

// pillTargetY 目标胶囊 y 坐标。
func (r *navSwitcherRenderer) pillTargetY() float32 {
	return float32(r.sw.current)*navItemHeight + navPillInset
}

// layoutPill 按当前 pillY 摆放胶囊。
func (r *navSwitcherRenderer) layoutPill() {
	w := r.sw.Size().Width
	if w <= 0 {
		return
	}
	r.pill.Resize(fyne.NewSize(w-2*navPillInset, navItemHeight-2*navPillInset))
	r.pill.Move(fyne.NewPos(navPillInset, r.pillY))
}

// stopAnim 停止进行中的胶囊动画（若有）。
func (r *navSwitcherRenderer) stopAnim() {
	if r.anim != nil {
		r.anim.Stop()
		r.anim = nil
	}
}

// animatePill 启动从当前位置到目标位置的滑动动画。
// 动画中目标变更时从当前插值位置续走（不排队、不跳变）。
//
// 注意：Fyne 驱动在动画结束时只回调一次 Tick(1.0) 并将动画移出 run-loop，
// 不会通知调用方；因此必须在 f>=1 时自行把 r.anim 置 nil，
// 否则“动画中”状态永远为真，后续 Refresh 不再更新胶囊（曾表现为“标签卡死在第一次点击”）。
func (r *navSwitcherRenderer) animatePill(target float32) {
	from := r.pillY
	r.stopAnim()
	r.animTarget = target
	if from == target {
		r.layoutPill()
		return
	}
	a := fyne.NewAnimation(pillAnimDuration, func(f float32) {
		r.pillY = from + (target-from)*f
		if f >= 1 {
			r.pillY = target
			r.anim = nil
		}
		r.layoutPill()
		canvas.Refresh(r.pill)
	})
	a.Curve = fyne.AnimationEaseOut
	r.anim = a
	a.Start()
}

func (r *navSwitcherRenderer) Layout(size fyne.Size) {
	for i, l := range r.labels {
		l.Resize(fyne.NewSize(size.Width, navItemHeight))
		l.Move(fyne.NewPos(0, float32(i)*navItemHeight))
	}
	if r.pill.Size().IsZero() {
		// 首次布局：直接落位，不播动画
		r.pillY = r.pillTargetY()
	}
	r.layoutPill()
}

func (r *navSwitcherRenderer) MinSize() fyne.Size {
	if len(r.labels) == 0 {
		return fyne.NewSize(0, 0)
	}
	return fyne.NewSize(navWidth, float32(len(r.labels))*navItemHeight)
}

func (r *navSwitcherRenderer) Refresh() {
	fg := theme.Color(theme.ColorNameForeground)
	if r.sw.disabled {
		fg = theme.Color(theme.ColorNameDisabled)
	}
	for _, l := range r.labels {
		l.Color = fg
		canvas.Refresh(l)
	}
	r.pill.FillColor = pillColor(r.sw.disabled)
	target := r.pillTargetY()
	switch {
	case r.sw.instant || r.pill.Size().IsZero():
		// 首次落位 / 明确要求不做动画
		r.sw.instant = false
		r.stopAnim()
		r.pillY = target
		r.animTarget = target
		r.layoutPill()
	case r.anim == nil || r.animTarget != target:
		// 无动画，或动画中目标已变更 → 从当前位置滑向新目标
		r.animatePill(target)
	}
	canvas.Refresh(r.pill)
}

func (r *navSwitcherRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.labels)+1)
	objs = append(objs, r.pill) // 胶囊在底层，文字在上层
	for _, l := range r.labels {
		objs = append(objs, l)
	}
	return objs
}

func (r *navSwitcherRenderer) Destroy() { r.stopAnim() }

// pillColor 胶囊颜色（禁用态降透明度变灰）。
func pillColor(disabled bool) color.Color {
	c := theme.PrimaryColor()
	if !disabled {
		return c
	}
	n := color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x40}
	if rgba, ok := c.(color.NRGBA); ok {
		n = rgba
		n.A = uint8(float32(rgba.A) * 0.35)
	}
	return n
}
