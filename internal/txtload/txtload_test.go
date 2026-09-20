package txtload

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"only blank", "\n\n  \n\t\n", nil},
		{"only comments", "# a\n#b\n", nil},
		{"simple", "https://t.me/telegram/193\n", []string{"https://t.me/telegram/193"}},
		{"crlf", "https://t.me/a/1\r\nhttps://t.me/a/2\r\n", []string{"https://t.me/a/1", "https://t.me/a/2"}},
		{"skip blank and comment", "# list\n\nhttps://t.me/a/1\n\n# tail\nhttps://t.me/a/2\n",
			[]string{"https://t.me/a/1", "https://t.me/a/2"}},
		{"trim whitespace", "   https://t.me/a/1   \n\thttps://t.me/a/2\t\n",
			[]string{"https://t.me/a/1", "https://t.me/a/2"}},
		{"comment with leading spaces", "   # comment\nhttps://t.me/a/1\n", []string{"https://t.me/a/1"}},
		{"no trailing newline", "https://t.me/a/1", []string{"https://t.me/a/1"}},
		{"bom", "\ufeffhttps://t.me/a/1\n", []string{"https://t.me/a/1"}},
		{"private and topic links", "https://t.me/c/1697797156/151\nhttps://t.me/iFreeKnow/45662/55005\n",
			[]string{"https://t.me/c/1697797156/151", "https://t.me/iFreeKnow/45662/55005"}},
		{"tg scheme", "tg://resolve?domain=iyear\n", []string{"tg://resolve?domain=iyear"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse 出错：%v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("结果不符：got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "links.txt")
	content := "# 我的链接\nhttps://t.me/a/1\n\nhttps://t.me/a/2\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试文件失败：%v", err)
	}

	got, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile 出错：%v", err)
	}
	want := []string{"https://t.me/a/1", "https://t.me/a/2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("结果不符：got %#v want %#v", got, want)
	}
}

func TestParseFileNotExist(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
}
