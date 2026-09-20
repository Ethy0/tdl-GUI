package runner

import "sync"

// Parser 把子进程输出字节流解析为"完整行（\n）"与"进度刷新（\r）"两类事件。
//
// 逐字节扫描，只在 \r / \n 处切分，因此不会切断 UTF-8 多字节字符；
// ANSI 转义序列在"整行"范围内剥离，避免序列跨读取块被截断。
type Parser struct {
	mu   sync.Mutex
	buf  []byte
	emit func(text string, progress bool)
}

// NewParser 创建解析器；emit 会被串行调用（text 已剥离 ANSI）。
func NewParser(emit func(text string, progress bool)) *Parser {
	return &Parser{emit: emit}
}

// Write 实现 io.Writer。
func (p *Parser) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf = append(p.buf, b...)
	p.scan(false)
	return len(b), nil
}

// Flush 处理缓冲区残留（管道 EOF 后调用）：残留内容作为完整行发出。
func (p *Parser) Flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scan(true)
}

func (p *Parser) scan(final bool) {
	for {
		i := indexAnyByte(p.buf, '\r', '\n')
		if i < 0 {
			break
		}
		text := Strip(string(p.buf[:i]))
		isCR := p.buf[i] == '\r'
		n := i + 1
		if isCR && n < len(p.buf) && p.buf[n] == '\n' {
			// \r\n 视为普通换行，避免产生多余空行
			isCR = false
			n++
		}
		p.buf = append(p.buf[:0], p.buf[n:]...)
		if isCR {
			if text != "" {
				p.emit(text, true)
			}
			continue
		}
		p.emit(text, false)
	}
	if final && len(p.buf) > 0 {
		text := Strip(string(p.buf))
		p.buf = p.buf[:0]
		p.emit(text, false)
	}
}

func indexAnyByte(b []byte, rs ...byte) int {
	for i, c := range b {
		for _, r := range rs {
			if c == r {
				return i
			}
		}
	}
	return -1
}
