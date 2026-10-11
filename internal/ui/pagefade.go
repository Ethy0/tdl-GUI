package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// 页面切换动画常量（v1.1.0 已确认）。
const (
	// pageFadeInDuration 新页淡入时长。
	pageFadeInDuration = 180 * time.Millisecond
	// pageSlideOffset 上下滑动的位移量（px）。
	pageSlideOffset = float32(16)
)

// pageFade 页面过渡容器：包裹各功能页，支持「淡入 + 上下方向滑入」动画。
//
// 实现要点（踩坑记录）：
//  1. Fyne 2.8 的 canvas.Rectangle 没有 Translucency 字段，淡入淡出改为动画遮罩
//     矩形的 FillColor alpha；
//  2. 遮罩空闲时必须 Hide()，否则本 fader 若在 Stack 中层级更高（如"设置"页），
//     会把下面的页面整块盖住（曾表现为"点击下载标签后右侧全空白"）；
//  3. 切页的"淡出"由 show() 立即隐藏旧页完成，仅新页播放淡入+滑入动画——
//     这样页面可见性保持同步语义（测试与逻辑都只看 Show/Hide）。
type pageFade struct {
	widget.BaseWidget
	content fyne.CanvasObject
	r       *pageFadeRenderer // CreateRenderer 时填充
}

// PageFade 是 pageFade 的导出别名（供测试定位）。
type PageFade = pageFade

// Renderer 返回渲染器（未创建时为 nil）。供黑盒测试下钻结构。
func (p *pageFade) Renderer() fyne.WidgetRenderer { return p.r }

// PageView 返回内层真实页面视图（黑盒测试定位用）。
func (p *pageFade) PageView() fyne.CanvasObject { return p.content }

// newPageFade 包裹一个页面视图。
func newPageFade(content fyne.CanvasObject) *pageFade {
	p := &pageFade{content: content}
	p.ExtendBaseWidget(p)
	return p
}

// ShowInstant 立即显示（无动画）：内容可见、遮罩完全收起。
func (p *pageFade) ShowInstant() {
	p.content.Show()
	p.setMask(0)
}

// HideInstant 立即隐藏：内容不可见、遮罩完全收起。
func (p *pageFade) HideInstant() {
	p.content.Hide()
	p.setMask(0)
}

// AnimateIn 新页入场：从 offsetY（+下方 / -上方）滑到 0，同时遮罩由全不透明减淡到透明。
func (p *pageFade) AnimateIn(offsetY float32, dur time.Duration) {
	p.content.Show()
	p.content.Move(fyne.NewPos(0, offsetY))
	p.setMask(1)
	r := p.r
	if r == nil {
		// 尚未布局（无渲染器）：直接落位，不播动画
		p.content.Move(fyne.NewPos(0, 0))
		return
	}
	r.stopAnim()
	r.anim = fyne.NewAnimation(dur, func(f float32) {
		if f >= 1 {
			// 动画结束：Fyne 驱动只会回调一次 Tick(1.0) 并将动画移出 run-loop，
			// 必须自行清除引用，否则 Refresh 会误判“仍在动画中”而不重置遮罩
			p.content.Move(fyne.NewPos(0, 0))
			r.anim = nil
			p.setMask(0)
			return
		}
		p.content.Move(fyne.NewPos(0, offsetY*(1-f)))
		p.setMask(1 - f)
	})
	r.anim.Curve = fyne.AnimationEaseOut
	r.anim.Start()
}

// setMask 设置遮罩透明度：0=完全透明且收起（不遮挡下层页面），1=完全不透明。
func (p *pageFade) setMask(alpha float32) {
	r := p.r
	if r == nil {
		return
	}
	if alpha < 0 {
		alpha = 0
	} else if alpha > 1 {
		alpha = 1
	}
	if alpha <= 0.001 {
		r.mask.FillColor = colorWithAlpha(theme.Color(theme.ColorNameBackground), 0)
		r.mask.Hide()
	} else {
		r.mask.FillColor = colorWithAlpha(theme.Color(theme.ColorNameBackground), alpha)
		r.mask.Show()
	}
	canvas.Refresh(r.mask)
	canvas.Refresh(p)
}

func (p *pageFade) CreateRenderer() fyne.WidgetRenderer {
	mask := canvas.NewRectangle(colorWithAlpha(theme.Color(theme.ColorNameBackground), 0))
	mask.Hide()
	r := &pageFadeRenderer{p: p, mask: mask}
	p.r = r
	return r
}

type pageFadeRenderer struct {
	p    *pageFade
	mask *canvas.Rectangle
	anim *fyne.Animation
}

// stopAnim 停止进行中的过渡动画。
func (r *pageFadeRenderer) stopAnim() {
	if r.anim != nil {
		r.anim.Stop()
		r.anim = nil
	}
}

func (r *pageFadeRenderer) Layout(size fyne.Size) {
	r.p.content.Resize(size)
	r.mask.Resize(size)
	r.mask.Move(fyne.NewPos(0, 0))
}

func (r *pageFadeRenderer) MinSize() fyne.Size { return r.p.content.MinSize() }

func (r *pageFadeRenderer) Refresh() {
	// 空闲状态遮罩必须透明且收起；动画中由 tick 控制，不重置
	if r.anim == nil {
		r.mask.FillColor = colorWithAlpha(theme.Color(theme.ColorNameBackground), 0)
		r.mask.Hide()
	}
	canvas.Refresh(r.p.content)
	canvas.Refresh(r.mask)
}

func (r *pageFadeRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.content, r.mask}
}

func (r *pageFadeRenderer) Destroy() { r.stopAnim() }

// colorWithAlpha 返回 base 按 alpha（0~1）降低透明度后的颜色。
func colorWithAlpha(base color.Color, alpha float32) color.Color {
	cr, cg, cb, _ := base.RGBA()
	return color.NRGBA{
		R: uint8(cr >> 8),
		G: uint8(cg >> 8),
		B: uint8(cb >> 8),
		A: uint8(float32(0xff) * alpha),
	}
}
