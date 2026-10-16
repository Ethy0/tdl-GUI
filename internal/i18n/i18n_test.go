package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 本文件全部用例均不得使用 t.Parallel()：i18n 是进程级全局状态（语言随 Init 改变），
// 并行执行会让用例互相污染。

// resetLang 用例结束后恢复源语言，保证同包内其它测试与后续用例看到中文界面。
func resetLang(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { Init(LangZh, LangZh) })
}

func TestInitResolve(t *testing.T) {
	resetLang(t)
	cases := []struct {
		name     string
		pref     string
		sys      string
		wantLang string
		wantRes  string // Resolve(pref)
	}{
		{"跟随系统-中文", "", "zh-CN", LangZh, LangZh},
		{"跟随系统-中文繁体也归源语言", "", "zh-Hant-TW", LangZh, LangZh},
		{"跟随系统-英文", "", "en-US", LangEn, LangEn},
		{"跟随系统-未知语言回退源语言", "", "de-DE", LangZh, LangZh},
		{"显式英文覆盖中文系统", LangEn, "zh-CN", LangEn, LangEn},
		{"显式中文覆盖英文系统", LangZh, "en-US", LangZh, LangZh},
		{"偏好值未知回退源语言", "fr", "en-US", LangZh, LangZh},
		{"偏好值大小写与下划线归一化", "EN_us", "zh-CN", LangEn, LangEn},
		{"系统语言为空回退源语言", "", "", LangZh, LangZh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Init(tc.pref, tc.sys)
			if got := Lang(); got != tc.wantLang {
				t.Fatalf("Lang() = %q，期望 %q", got, tc.wantLang)
			}
			if got := Resolve(tc.pref); got != tc.wantRes {
				t.Fatalf("Resolve(%q) = %q，期望 %q", tc.pref, got, tc.wantRes)
			}
		})
	}
}

// TestResolveUsesSystemLocaleFromInit 「跟随系统」必须基于 Init 时保存的系统语言，
// 供设置页判断"改语言是否需要重启"。
func TestResolveUsesSystemLocaleFromInit(t *testing.T) {
	resetLang(t)
	Init("", "en-GB")
	if got := Resolve(""); got != LangEn {
		t.Fatalf("中文系统之外 Resolve(\"\") 应等于系统语言，实际 %q", got)
	}
	// 当前生效语言 == 跟随系统解析值 → 设置页不应提示重启
	if Lang() != Resolve("") {
		t.Fatalf("生效语言 %q 与 Resolve(\"\") %q 应一致", Lang(), Resolve(""))
	}
}

func TestT_Fallback(t *testing.T) {
	resetLang(t)
	const key = "下载"

	Init(LangZh, LangZh)
	if got := T(key); got != key {
		t.Fatalf("源语言下 T 应原样返回原文，实际 %q", got)
	}

	Init(LangEn, LangZh)
	if got := T(key); got != "Download" {
		t.Fatalf("英文下 T(%q) 应为译文，实际 %q", key, got)
	}

	// 未收录的键在英文下回退中文原文（新增文案不会显示为空）
	const missing = "这条文案尚未翻译"
	if got := T(missing); got != missing {
		t.Fatalf("未收录键应回退原文，实际 %q", got)
	}
}

func TestTf_Template(t *testing.T) {
	resetLang(t)
	Init(LangZh, LangZh)

	// 源语言下键自身即模板，需正常渲染参数
	if got := Tf("已解析 {{.n}} 条链接", map[string]any{"n": 3}); got != "已解析 3 条链接" {
		t.Fatalf("源语言模板渲染错误：%q", got)
	}
	// 退出码行是既有测试的硬断言对象（ui_test.go / 集成测试）
	if got := Tf("退出码：{{.code}}（{{.status}}）", map[string]any{"code": 0, "status": T("正常")}); got != "退出码：0（正常）" {
		t.Fatalf("退出码行渲染错误：%q", got)
	}

	Init(LangEn, LangZh)
	if got := Tf("已解析 {{.n}} 条链接", map[string]any{"n": 3}); got != "Parsed 3 links" {
		t.Fatalf("英文模板渲染错误：%q", got)
	}

	// 无参数键走 Tf 也应正常（不得输出 "<no value>"）
	if got := Tf("下载", nil); got != "Download" {
		t.Fatalf("无参键 Tf 结果错误：%q", got)
	}

	// 模板非法时回退原文，不 panic
	if got := Tf("坏模板 {{.n", map[string]any{"n": 1}); got != "坏模板 {{.n" {
		t.Fatalf("非法模板应回退原文，实际 %q", got)
	}

	// 参数缺失时按 text/template 语义输出 <no value>（不 panic、不丢文案）
	Init(LangZh, LangZh)
	if got := Tf("已解析 {{.n}} 条链接", nil); !strings.Contains(got, "条链接") {
		t.Fatalf("缺参时不应丢文案：%q", got)
	}
}

// TestEnTableLoaded 英文译表必须可用且无空值（空值会在 parseTable 中被剔除）。
func TestEnTableLoaded(t *testing.T) {
	resetLang(t)
	keys := Keys(LangEn)
	if len(keys) < 100 {
		t.Fatalf("英文译表条目过少：%d", len(keys))
	}
	Init(LangEn, LangZh)
	for _, k := range keys {
		if v := T(k); v == "" {
			t.Fatalf("键 %q 的译文为空", k)
		}
	}
}

var tmplParamRe = regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)

func tmplParams(s string) []string {
	var out []string
	for _, m := range tmplParamRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// TestTfParamsConsistent 译文的模板参数必须与键（中文原文）完全一致，
// 防止"键改了参数名而译文漏改"导致英文界面出现 <no value>。
func TestTfParamsConsistent(t *testing.T) {
	resetLang(t)
	Init(LangEn, LangZh)
	for _, k := range Keys(LangEn) {
		want := tmplParams(k)
		got := tmplParams(T(k))
		if strings.Join(want, ",") != strings.Join(got, ",") {
			t.Errorf("模板参数不一致：键 %q 有 %v，译文 %q 有 %v", k, want, T(k), got)
		}
	}
}

// TestNormalizeUnsupportedFallsBackToSource 未支持语言一律回退源语言（新增语言需显式扩展）。
func TestNormalizeUnsupportedFallsBackToSource(t *testing.T) {
	resetLang(t)
	for _, in := range []string{"ja", "ru-RU", "fr", "  ", "xx"} {
		if got := normalize(in); got != LangZh {
			t.Errorf("normalize(%q) = %q，期望 %q", in, got, LangZh)
		}
	}
	if got := normalize("en-GB"); got != LangEn {
		t.Errorf("normalize(\"en-GB\") = %q，期望 %q", got, LangEn)
	}
}

// TestEnNoLiteralEscapes 译文不得含字面反斜杠。
//
// 回归背景（v1.2.0 实测缺陷）：生成器 temp/i18njson 早期版本把译文当纯文本行读取，
// 而键清单经 strconv.Unquote 解码 → 键里是真实换行、译文里是字面 "\n"（反斜杠+n），
// 表现为英文界面消息链接占位符显示 `One link per line, e.g.:\nhttps://…`。
// 现约定：译文行中的 \n / \t 会被还原为真实字符，故正常译文中不应出现任何反斜杠。
func TestEnNoLiteralEscapes(t *testing.T) {
	resetLang(t)
	Init(LangEn, LangZh)
	for _, k := range Keys(LangEn) {
		v := T(k)
		if strings.Contains(v, `\`) {
			t.Errorf("译文含字面反斜杠（转义未还原）：\n  键 %q\n  值 %q", k, v)
		}
	}
}

// TestMultiLineStructureKept 多行文案的换行结构必须与原文一致。
//
// 换行数不等通常意味着译文漏写（或误写为字面 \n）导致版面塌成一行。
func TestMultiLineStructureKept(t *testing.T) {
	resetLang(t)
	Init(LangEn, LangZh)
	checked := 0
	for _, k := range Keys(LangEn) {
		if !strings.Contains(k, "\n") {
			continue
		}
		checked++
		if w, g := strings.Count(k, "\n"), strings.Count(T(k), "\n"); w != g {
			t.Errorf("多行文案换行数不一致：键 %q 有 %d 个换行，译文 %q 有 %d 个", k, w, T(k), g)
		}
	}
	if checked == 0 {
		t.Fatal("未找到多行文案，本用例失去意义（检查 en.json）")
	}
	t.Logf("已校验 %d 条多行文案", checked)
}
