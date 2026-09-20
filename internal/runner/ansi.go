package runner

import (
	"regexp"
	"strings"
)

var (
	// CSI：ESC [ 参数 中间字节 终止字节（颜色码、光标移动等）
	csiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	// OSC：ESC ] ... BEL 或 ST
	oscRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
)

// Strip 剥离 ANSI 转义序列（OSC、CSI、其余两字符 ESC 序列），
// 并过滤除 \r \n \t 以外的 C0 控制符（保留 \r \n 供行解析使用）。
func Strip(s string) string {
	if s == "" {
		return s
	}
	s = oscRe.ReplaceAllString(s, "")
	s = csiRe.ReplaceAllString(s, "")

	var b strings.Builder
	b.Grow(len(s))
	inEscape := false
	for _, r := range s {
		if inEscape {
			// ESC 之后：中间字节（0x20-0x2F）继续等待，终止字节（0x30-0x7E）结束序列；
			// 二者均不输出（覆盖 ESC ( B、ESC = 等两/三字符序列）。
			if r >= 0x20 && r <= 0x2f {
				continue
			}
			inEscape = false
			if r >= 0x30 && r <= 0x7e {
				continue
			}
			// 非终止字节（如换行）按普通控制符规则继续处理
		}
		switch {
		case r == 0x1b:
			inEscape = true
		case r == '\r' || r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			// 丢弃其余控制符与 DEL
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
