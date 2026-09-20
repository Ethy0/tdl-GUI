// Package cmdgen 把界面表单结构体拼装为 tdl 命令行参数（纯函数，无 UI / 无 fyne 依赖）。
//
// 拼装顺序：全局 flags → 子命令 → 子命令 flags。
// 通用规则：布尔勾选才拼入；字符串/数字非空才拼入；等于默认值的全局参数省略（语义不变）。
package cmdgen

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 默认值（与 tdl 0.20.4 实际默认一致：等于默认值时不拼入，语义不变）。
const (
	DefaultNS              = "default"
	DefaultReconTimeout    = "5m"
	DefaultReconTimeoutAlt = "5m0s"
	DefaultPool            = "8"
	DefaultDelay           = "0s"
	DefaultPort            = "8080"
	DefaultExportOutput    = "tdl-export.json"
	DefaultExportType      = "time"

	// MaxLinkCount 链接数量上限（Windows CreateProcess 命令行 32K 字符上限防护）。
	MaxLinkCount = 800
)

// 断点策略取值。
const (
	ResumeDefault  = "default"
	ResumeContinue = "continue"
	ResumeRestart  = "restart"
)

// 导出类型取值。
const (
	ExportTypeTime = "time"
	ExportTypeID   = "id"
	ExportTypeLast = "last"
)

// GlobalFlags 对应设置页全局参数（不持久化于 tdl，每次执行自动附加）。
type GlobalFlags struct {
	TdlPath      string // tdl.exe 完整路径（必填）
	NS           string // 当前命名空间，默认 "default"
	Proxy        string // --proxy
	NTP          string // --ntp
	ReconTimeout string // --reconnect-timeout，默认 "5m"
	Pool         string // --pool，默认 "8"
	Delay        string // --delay，默认 "0s"
	Debug        bool   // --debug
	NoProgPS     bool   // --disable-progress-ps
	Storage      string // --storage（空 = 不拼入）
}

// DownloadArgs 对应下载页表单。
type DownloadArgs struct {
	Links     []string // 手动输入链接
	TxtLinks  []string // txt 解析产物（运行时派生）
	JSONFiles []string // -f 列表
	Dir       string   // -d
	Threads   string   // -t
	Limit     string   // -l

	Desc       bool
	RewriteExt bool
	Group      bool
	SkipSame   bool
	Takeout    bool

	Include  string // -i
	Exclude  string // -e
	Template string // --template

	Resume string // "default" | "continue" | "restart"

	Serve bool
	Port  string
}

// UploadArgs 对应上传页表单。
type UploadArgs struct {
	Paths   []string // -p 列表
	Chat    string   // -c
	Topic   string   // --topic
	To      string   // --to（与 Chat/Topic 互斥）
	Threads string
	Limit   string
	Caption string // --caption
	Include string
	Exclude string
	RM      bool
	Photo   bool
}

// ExportArgs 对应导出消息页表单。
type ExportArgs struct {
	Chat        string // -c
	Topic       string // --topic
	Reply       string // --reply
	Output      string // -o
	Type        string // "time" | "id" | "last"
	Range       string // -i
	Filter      string // -f
	WithContent bool
	All         bool
	Raw         bool
}

// LoginArgs 对应设置页登录区（最小化版：仅桌面客户端登录）。
type LoginArgs struct {
	NS       string // 命名空间（必填）
	Passcode string // 本地密码（对应桌面客户端 passcode）
	TdDir    string // 桌面客户端目录
}

var rangeRe = regexp.MustCompile(`^\d+(,\d+)*$`)

var errTdlPath = errors.New("请先在设置页选择 tdl.exe 路径")

// BuildGlobal 拼装全局 flags（顺序固定）。withNS=false 时不拼入 -n（如 login -l 场景）。
func BuildGlobal(g GlobalFlags, withNS bool) []string {
	args := make([]string, 0, 16)
	if withNS && g.NS != "" && g.NS != DefaultNS {
		args = append(args, "-n", g.NS)
	}
	if g.Proxy != "" {
		args = append(args, "--proxy", g.Proxy)
	}
	if g.NTP != "" {
		args = append(args, "--ntp", g.NTP)
	}
	if v := g.ReconTimeout; v != "" && v != DefaultReconTimeout && v != DefaultReconTimeoutAlt {
		args = append(args, "--reconnect-timeout", v)
	}
	if g.Pool != "" && g.Pool != DefaultPool {
		args = append(args, "--pool", g.Pool)
	}
	if g.Delay != "" && g.Delay != DefaultDelay {
		args = append(args, "--delay", g.Delay)
	}
	if g.Debug {
		args = append(args, "--debug")
	}
	if g.NoProgPS {
		args = append(args, "--disable-progress-ps")
	}
	if g.Storage != "" {
		args = append(args, "--storage", g.Storage)
	}
	return args
}

// ValidateGlobal 校验全局参数（tdl.exe 路径必填、连接池为非负整数）。
func ValidateGlobal(g GlobalFlags) error {
	var errs []error
	if g.TdlPath == "" {
		errs = append(errs, errTdlPath)
	}
	if err := CheckNonNegativeInt("DC 连接池大小", g.Pool); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// ValidateRange 校验导出范围格式：留空、单个正整数、或英文逗号分隔的两个正整数。
func ValidateRange(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if !rangeRe.MatchString(s) {
		return errors.New("范围格式不正确：应为正整数，或用英文逗号分隔的两个正整数（如 1000,2000）")
	}
	return nil
}

// CheckNonNegativeInt 校验"留空或非负整数"字段
// （用于 0 有特定语义的字段：tdl 的 --pool 0 表示不限制）。
func CheckNonNegativeInt(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return fmt.Errorf("字段 %s 必须为非负整数（0 表示不限制）", name)
	}
	return nil
}

// CheckPositiveInt 校验"留空或正整数"字段。
func CheckPositiveInt(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return fmt.Errorf("字段 %s 必须为正整数", name)
	}
	return nil
}

// CleanList 去除每项首尾空白并丢弃空项。
func CleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildDownload 拼装 `dl` 子命令。返回值不含 tdl.exe 路径本身。
func BuildDownload(g GlobalFlags, d DownloadArgs) ([]string, error) {
	var errs []error
	if err := ValidateGlobal(g); err != nil {
		errs = append(errs, err)
	}

	links := CleanList(d.Links)
	txtLinks := CleanList(d.TxtLinks)
	files := CleanList(d.JSONFiles)

	if d.Include != "" && d.Exclude != "" {
		errs = append(errs, errors.New("扩展名白名单与黑名单不能同时填写"))
	}
	if len(links)+len(txtLinks)+len(files) == 0 {
		errs = append(errs, errors.New("请至少提供一种输入源（链接 / txt / JSON）"))
	}
	if n := len(links) + len(txtLinks); n > MaxLinkCount {
		errs = append(errs, fmt.Errorf("链接数量过多（%d 条），命令行可能超长，请分批执行", n))
	}
	if err := CheckPositiveInt("每任务线程数", d.Threads); err != nil {
		errs = append(errs, err)
	}
	if err := CheckPositiveInt("并发任务数", d.Limit); err != nil {
		errs = append(errs, err)
	}
	if d.Serve {
		if err := CheckPositiveInt("serve 端口", d.Port); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	args := BuildGlobal(g, true)
	args = append(args, "dl")
	for _, l := range links {
		args = append(args, "-u", l)
	}
	for _, l := range txtLinks {
		args = append(args, "-u", l)
	}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	if d.Dir != "" {
		args = append(args, "-d", d.Dir)
	}
	if t := strings.TrimSpace(d.Threads); t != "" {
		args = append(args, "-t", t)
	}
	if l := strings.TrimSpace(d.Limit); l != "" {
		args = append(args, "-l", l)
	}
	if d.Desc {
		args = append(args, "--desc")
	}
	if d.RewriteExt {
		args = append(args, "--rewrite-ext")
	}
	if d.Group {
		args = append(args, "--group")
	}
	if d.SkipSame {
		args = append(args, "--skip-same")
	}
	if d.Takeout {
		args = append(args, "--takeout")
	}
	if d.Include != "" {
		args = append(args, "-i", d.Include)
	}
	if d.Exclude != "" {
		args = append(args, "-e", d.Exclude)
	}
	if d.Template != "" {
		args = append(args, "--template", d.Template)
	}
	switch d.Resume {
	case ResumeContinue:
		args = append(args, "--continue")
	case ResumeRestart:
		args = append(args, "--restart")
	}
	if d.Serve {
		args = append(args, "--serve")
		if p := strings.TrimSpace(d.Port); p != "" {
			args = append(args, "--port", p)
		}
	}
	return args, nil
}

// BuildUpload 拼装 `up` 子命令。
func BuildUpload(g GlobalFlags, u UploadArgs) ([]string, error) {
	var errs []error
	if err := ValidateGlobal(g); err != nil {
		errs = append(errs, err)
	}

	paths := CleanList(u.Paths)
	if len(paths) == 0 {
		errs = append(errs, errors.New("请至少添加一个待上传文件或目录"))
	}
	if u.To != "" && (u.Chat != "" || u.Topic != "") {
		errs = append(errs, errors.New("消息路由与目标聊天/话题互斥，只能二选一"))
	}
	if u.Include != "" && u.Exclude != "" {
		errs = append(errs, errors.New("扩展名白名单与黑名单不能同时填写"))
	}
	if err := CheckPositiveInt("论坛话题 ID", u.Topic); err != nil {
		errs = append(errs, err)
	}
	if err := CheckPositiveInt("每任务线程数", u.Threads); err != nil {
		errs = append(errs, err)
	}
	if err := CheckPositiveInt("并发任务数", u.Limit); err != nil {
		errs = append(errs, err)
	}
	if u.Topic != "" && u.Chat == "" {
		errs = append(errs, errors.New("论坛话题 ID 必须与目标聊天同时填写"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	args := BuildGlobal(g, true)
	args = append(args, "up")
	for _, p := range paths {
		args = append(args, "-p", p)
	}
	if u.Chat != "" {
		args = append(args, "-c", u.Chat)
	}
	if t := strings.TrimSpace(u.Topic); t != "" {
		args = append(args, "--topic", t)
	}
	if u.To != "" {
		args = append(args, "--to", u.To)
	}
	if t := strings.TrimSpace(u.Threads); t != "" {
		args = append(args, "-t", t)
	}
	if l := strings.TrimSpace(u.Limit); l != "" {
		args = append(args, "-l", l)
	}
	if u.Caption != "" {
		args = append(args, "--caption", u.Caption)
	}
	if u.Include != "" {
		args = append(args, "-i", u.Include)
	}
	if u.Exclude != "" {
		args = append(args, "-e", u.Exclude)
	}
	if u.RM {
		args = append(args, "--rm")
	}
	if u.Photo {
		args = append(args, "--photo")
	}
	return args, nil
}

// BuildExport 拼装 `chat export` 子命令。
func BuildExport(g GlobalFlags, e ExportArgs) ([]string, error) {
	var errs []error
	if err := ValidateGlobal(g); err != nil {
		errs = append(errs, err)
	}

	if err := CheckPositiveInt("话题 ID", e.Topic); err != nil {
		errs = append(errs, err)
	}
	if err := CheckPositiveInt("评论区帖子 ID", e.Reply); err != nil {
		errs = append(errs, err)
	}
	switch e.Type {
	case "", ExportTypeTime, ExportTypeID, ExportTypeLast:
	default:
		errs = append(errs, fmt.Errorf("导出类型非法：%s", e.Type))
	}
	if err := ValidateRange(e.Range); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	args := BuildGlobal(g, true)
	args = append(args, "chat", "export")
	if e.Chat != "" {
		args = append(args, "-c", e.Chat)
	}
	if t := strings.TrimSpace(e.Topic); t != "" {
		args = append(args, "--topic", t)
	}
	if r := strings.TrimSpace(e.Reply); r != "" {
		args = append(args, "--reply", r)
	}
	if e.Output != "" {
		args = append(args, "-o", e.Output)
	}
	if e.Type != "" && e.Type != DefaultExportType {
		args = append(args, "-T", e.Type)
	}
	if r := strings.TrimSpace(e.Range); r != "" {
		args = append(args, "-i", r)
	}
	if e.Filter != "" {
		args = append(args, "-f", e.Filter)
	}
	if e.WithContent {
		args = append(args, "--with-content")
	}
	if e.All {
		args = append(args, "--all")
	}
	if e.Raw {
		args = append(args, "--raw")
	}
	return args, nil
}

// BuildLogin 拼装登录命令：`[全局(含 -n)] login [-p <pass>] [-d <dir>]`。
// 仅支持桌面客户端登录（tdl 默认方式，读取 Telegram Desktop 会话）。
func BuildLogin(g GlobalFlags, l LoginArgs) ([]string, error) {
	var errs []error
	if g.TdlPath == "" {
		errs = append(errs, errTdlPath)
	}
	ns := strings.TrimSpace(l.NS)
	if ns == "" {
		errs = append(errs, errors.New("请填写命名空间"))
	} else if strings.ContainsAny(ns, " \t\r\n") {
		errs = append(errs, errors.New("命名空间不能包含空白字符"))
	}
	if err := CheckNonNegativeInt("DC 连接池大小", g.Pool); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	gg := g
	gg.NS = ns
	args := BuildGlobal(gg, true)
	args = append(args, "login")
	if l.Passcode != "" {
		args = append(args, "-p", l.Passcode)
	}
	if l.TdDir != "" {
		args = append(args, "-d", l.TdDir)
	}
	return args, nil
}

// BuildTest 用于设置页 [测试] 按钮：执行等价的轻量命令 `tdl version` 验证 tdl.exe 可用性。
// 与其它任务一致地校验全局参数（tdl.exe 路径必填、连接池非负），非法时同样禁用执行。
func BuildTest(g GlobalFlags) ([]string, error) {
	if err := ValidateGlobal(g); err != nil {
		return nil, err
	}
	return []string{"version"}, nil
}

// SuspiciousLinks 返回可能不受支持的链接（非 https://t.me/ 或 tg:// 开头），仅警告不阻断。
func SuspiciousLinks(links []string) []string {
	var out []string
	for _, l := range CleanList(links) {
		if strings.HasPrefix(l, "https://t.me/") || strings.HasPrefix(l, "tg://") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// EchoString 生成日志回显用的命令行文本：`$ "tdl.exe" "arg1" ...`。
// login 命令中的密码（-p 的后继参数）脱敏为 ***。
func EchoString(tdlPath string, args []string) string {
	isLogin := false
	for _, a := range args {
		if a == "login" {
			isLogin = true
			break
		}
	}

	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quote(tdlPath))
	for i := 0; i < len(args); i++ {
		if isLogin && args[i] == "-p" && i+1 < len(args) {
			parts = append(parts, quote("-p"), quote("***"))
			i++
			continue
		}
		parts = append(parts, quote(args[i]))
	}
	return "$ " + strings.Join(parts, " ")
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
