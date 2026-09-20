package runner

import "testing"

func TestStrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"empty", "", ""},
		{"csi color", "\x1b[31mred\x1b[0m", "red"},
		{"csi cursor move", "a\x1b[2Kb\x1b[1;1Hc", "abc"},
		{"osc title bell", "\x1b]0;my title\x07text", "text"},
		{"osc title st", "\x1b]0;my title\x1b\\text", "text"},
		{"bare esc with printable", "a\x1bb", "a"},
		{"two char escape", "\x1b(Babc", "abc"},
		{"c0 filtered", "a\x00\x07b", "ab"},
		{"keep crlf tab", "a\r\nb\tc", "a\r\nb\tc"},
		{"cjk kept", "下载完成 100%", "下载完成 100%"},
		{"mixed", "\x1b[1;32mOK\x1b[0m\r\n", "OK\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Strip(tt.in); got != tt.want {
				t.Fatalf("Strip(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
