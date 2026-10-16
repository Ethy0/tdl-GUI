// Package i18n 提供 tdl-GUI 的轻量多语言解析：中文原文即键，英文等语言由
// locales/*.json 提供译文；查不到译文时原样返回原文，因此源语言（简体中文）
// 永远正确显示，未翻译的新增文案自动降级为中文而非空白。
//
// 为什么不直接用 fyne.io/fyne/v2/lang：
// Fyne v2.8.1 的 lang 包在包初始化时按系统 locale 一次性确定当前语言，
// 且未导出任何 setter（SetPreferredLocale 尚在上游 PR #5026 未合入），
// 无法实现"用户在设置页选择语言、重启后生效"（方法 A）。本项目因此自建解析器；
// fyne lang 仅保留两个用途：探测系统语言（lang.SystemLocale）与框架内置控件
// （对话框按钮、Entry 右键菜单）的自动翻译——后者仍随系统语言，见方案限制 L-1。
//
// 并发说明：Init 只在进程启动（main.go）与测试中调用，二者不会并发；
// 其余函数（T/Tf/Lang/Resolve）只读，可被任意 goroutine 安全调用。
// 相关测试请勿使用 t.Parallel()。
package i18n

import (
	"embed"
	"encoding/json"
	"strings"
	"sync"
	"text/template"
)

// 语言常量。LangZh 是源语言（键的语言），无翻译文件，T 直接返回键。
const (
	LangZh  = "zh-Hans"
	LangEn  = "en"
	locales = "locales"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	mu sync.RWMutex

	// lang 当前生效语言，默认源语言（未调用 Init 时的行为 = 现状，保证既有测试与
	// 不经 main.go 的场景（白盒测试直接构造页面）都得到中文界面）。
	lang = LangZh

	// sysLocale Init 时保存的系统语言，供 Resolve 解析"跟随系统"用。
	sysLocale = LangZh

	// tables 语言 → (键 → 译文)。
	tables = map[string]map[string]string{}
)

// Init 初始化语言，由 main.go 在 ui.Build 之前调用一次。
//
//	pref         设置页保存的语言偏好："" = 跟随系统；LangZh / LangEn = 显式指定
//	systemLocale 系统语言，调用方传 lang.SystemLocale().LanguageString()
//
// 非并发安全：仅在进程启动与测试中调用。
func Init(pref, systemLocale string) {
	mu.Lock()
	defer mu.Unlock()

	sysLocale = normalize(systemLocale)
	if pref == "" {
		lang = sysLocale
	} else {
		lang = normalize(pref)
	}
}

// Lang 返回当前生效语言。
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// Resolve 返回某语言偏好值对应的生效语言（设置页判断是否需要提示重启用）。
// pref 语义与 Init 相同（"" = 跟随系统）。
func Resolve(pref string) string {
	mu.RLock()
	sys := sysLocale
	mu.RUnlock()

	if pref == "" {
		return sys
	}
	return normalize(pref)
}

// T 翻译一条文案。key 即中文原文（gettext 风格）；当前语言无对应条目时原样返回 key。
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	return lookup(lang, key)
}

// Tf 翻译一条带参文案。模板语法与 Fyne lang 一致（text/template）：
//
//	i18n.Tf("已解析 {{.n}} 条链接", map[string]any{"n": 3})
//
// 参数以 map 提供，模板中写作 {{.n}}。模板解析或执行失败时回退为原文（不 panic）。
func Tf(key string, data any) string {
	mu.RLock()
	s := lookup(lang, key)
	mu.RUnlock()

	t, err := template.New("t").Parse(s)
	if err != nil {
		return s
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return s
	}
	return b.String()
}

// lookup 在指定语言的译表中查键（不加锁，调用方持锁）。
func lookup(l string, key string) string {
	if l == LangZh {
		return key // 源语言：原文即译文
	}
	if m, ok := tables[l]; ok {
		if v, ok := m[key]; ok && v != "" {
			return v
		}
	}
	return key
}

// normalize 把任意 locale 字符串归一化为受支持的语言。
// 目前支持 zh（→ zh-Hans，源语言）与 en；其余一律回退源语言。
func normalize(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	if l == "" {
		return LangZh
	}
	l = strings.ReplaceAll(l, "_", "-")
	switch {
	case strings.HasPrefix(l, "en"):
		return LangEn
	case strings.HasPrefix(l, "zh"):
		return LangZh
	default:
		return LangZh
	}
}

// parseTable 解析一个语言的翻译表（语言码由调用方从文件名得出，故无需传入）。
func parseTable(data []byte) (map[string]string, error) {
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for k, v := range m {
		if strings.TrimSpace(v) == "" {
			delete(m, k) // 空译文视为未翻译，回退原文
		}
	}
	return m, nil
}

// init 载入内嵌翻译文件（失败时保持空表 → 全部回退源语言，不阻塞启动）。
func init() {
	entries, err := localeFS.ReadDir(locales)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := localeFS.ReadFile(locales + "/" + e.Name())
		if err != nil {
			continue
		}
		code := strings.TrimSuffix(e.Name(), ".json")
		m, err := parseTable(data)
		if err != nil {
			continue
		}
		tables[normalize(code)] = m
	}
}

// Keys 返回某语言译表的键集合（测试与一致性检查用）。
func Keys(l string) []string {
	mu.RLock()
	defer mu.RUnlock()
	m := tables[normalize(l)]
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
