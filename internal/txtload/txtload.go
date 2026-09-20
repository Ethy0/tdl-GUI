// Package txtload 解析"每行一个链接"的 txt 文件（GUI 适配层，tdl 原生 -f 只接受 JSON）。
//
// 规则：每行一个链接；跳过空行与以 # 开头的注释行；去除行首尾空白与 UTF-8 BOM。
package txtload

import (
	"bufio"
	"io"
	"os"
	"strings"
)

const maxLineSize = 1024 * 1024

// Parse 从 r 中解析链接列表。
func Parse(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var out []string
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseFile 读取文件并解析链接列表。
func ParseFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}
