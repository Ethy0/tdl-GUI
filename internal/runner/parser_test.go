package runner

import (
	"reflect"
	"testing"
)

type ev struct {
	text     string
	progress bool
}

func collect() (*Parser, *[]ev) {
	var got []ev
	p := NewParser(func(text string, progress bool) {
		got = append(got, ev{text, progress})
	})
	return p, &got
}

func TestParserLines(t *testing.T) {
	p, got := collect()

	if _, err := p.Write([]byte("a\nb\n")); err != nil {
		t.Fatalf("Write 出错：%v", err)
	}
	p.Flush()

	want := []ev{{"a", false}, {"b", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %#v want %#v", *got, want)
	}
}

func TestParserCRLF(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("a\r\nb\r\n"))
	p.Flush()

	want := []ev{{"a", false}, {"b", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("CRLF 不应产生空行：got %#v want %#v", *got, want)
	}
}

func TestParserProgress(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("10%\r20%\r30%\n"))
	p.Flush()

	want := []ev{{"10%", true}, {"20%", true}, {"30%", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %#v want %#v", *got, want)
	}
}

func TestParserProgressThenFlush(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("50%"))
	p.Flush()

	want := []ev{{"50%", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("无尾换行的残留应作为完整行发出：got %#v want %#v", *got, want)
	}
}

func TestParserEmptyLines(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("a\n\nb\n"))
	p.Flush()

	want := []ev{{"a", false}, {"", false}, {"b", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %#v want %#v", *got, want)
	}
}

func TestParserSplitWrites(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("ab"))
	_, _ = p.Write([]byte("c\nd"))
	p.Flush()

	want := []ev{{"abc", false}, {"d", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %#v want %#v", *got, want)
	}
}

func TestParserSplitMultibyte(t *testing.T) {
	p, got := collect()
	// "中" 的 UTF-8 编码被拆到两次 Write
	_, _ = p.Write([]byte{0xe4})
	_, _ = p.Write([]byte{0xb8, 0xad, '\n'})
	p.Flush()

	want := []ev{{"中", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("多字节字符被切断：got %#v want %#v", *got, want)
	}
}

func TestParserWideLines(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("下载进度：50% 速度 1.2MB/s\n"))
	p.Flush()

	want := []ev{{"下载进度：50% 速度 1.2MB/s", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %#v want %#v", *got, want)
	}
}

func TestParserANSIStripped(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("\x1b[32mgreen line\x1b[0m\n"))
	p.Flush()

	want := []ev{{"green line", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %#v want %#v", *got, want)
	}
}

func TestParserEmptyCRIgnored(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("\r\r\n"))
	p.Flush()

	if len(*got) != 1 || (*got)[0] != (ev{"", false}) {
		t.Fatalf("空 \r 不应派发进度事件：got %#v", *got)
	}
}

func TestParserFlushIdempotent(t *testing.T) {
	p, got := collect()
	_, _ = p.Write([]byte("x"))
	p.Flush()
	p.Flush()

	want := []ev{{"x", false}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("重复 Flush 不应重复派发：got %#v", *got)
	}
}
