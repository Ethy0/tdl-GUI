package store

import (
	"reflect"
	"testing"
)

// memPrefs 是内存版 Preferences，用于单测（不依赖 fyne）。
type memPrefs struct {
	m map[string]string
}

func newMemPrefs() *memPrefs { return &memPrefs{m: map[string]string{}} }

func (m *memPrefs) String(key string) string        { return m.m[key] }
func (m *memPrefs) SetString(key, value string)     { m.m[key] = value }
func (m *memPrefs) RemoveValue(key string)          { delete(m.m, key) }

func TestStrDefaultAndRoundTrip(t *testing.T) {
	s := NewStore(newMemPrefs())

	if got := s.Str(KeySettingsNS, "default"); got != "default" {
		t.Fatalf("未写入时应返回默认值 default，实际 %q", got)
	}
	s.SetStr(KeySettingsNS, "work")
	if got := s.Str(KeySettingsNS, "default"); got != "work" {
		t.Fatalf("应读回 work，实际 %q", got)
	}
	// 写入空串后回落为默认值（Preferences 无法区分未写入与空串）
	s.SetStr(KeySettingsNS, "")
	if got := s.Str(KeySettingsNS, "default"); got != "default" {
		t.Fatalf("写入空串后应回落默认值，实际 %q", got)
	}
}

func TestBoolDefaultAndRoundTrip(t *testing.T) {
	s := NewStore(newMemPrefs())

	if s.Bool(KeySettingsDebug, false) {
		t.Fatal("未写入时默认应为 false")
	}
	if !s.Bool(KeySettingsDebug, true) {
		t.Fatal("未写入时默认应为 def=true")
	}
	s.SetBool(KeySettingsDebug, true)
	if !s.Bool(KeySettingsDebug, false) {
		t.Fatal("写入 true 后应读回 true")
	}
	s.SetBool(KeySettingsDebug, false)
	if s.Bool(KeySettingsDebug, true) {
		t.Fatal("写入 false 后应读回 false（而非默认值）")
	}
}

func TestListRoundTrip(t *testing.T) {
	s := NewStore(newMemPrefs())

	if got := s.List(KeyUploadPaths); got != nil {
		t.Fatalf("空值应返回 nil，实际 %#v", got)
	}
	want := []string{`C:\a b\1.zip`, `D:\2.mp4`}
	s.SetList(KeyUploadPaths, want)
	if got := s.List(KeyUploadPaths); !reflect.DeepEqual(got, want) {
		t.Fatalf("List 往返不一致：%#v != %#v", got, want)
	}
	// 空行应被过滤
	s.SetStr(KeyUploadPaths, "a\n\n  \nb")
	if got := s.List(KeyUploadPaths); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("空行应被过滤，实际 %#v", got)
	}
	s.SetList(KeyUploadPaths, nil)
	if got := s.List(KeyUploadPaths); got != nil {
		t.Fatalf("写入 nil 应读回 nil，实际 %#v", got)
	}
}

// ClearTaskScope 必须删除全部任务目标范围键，且不触碰调优类/设置类键。
func TestClearTaskScope(t *testing.T) {
	p := newMemPrefs()
	s := NewStore(p)

	s.SetStr(KeyDownloadLinks, "https://t.me/a/1")
	s.SetStr(KeyDownloadTxtPath, `C:\a.txt`)
	s.SetList(KeyDownloadJSONFiles, []string{"a.json"})
	s.SetList(KeyUploadPaths, []string{`D:\a.zip`})
	s.SetStr(KeyUploadChat, "@iyear")
	s.SetStr(KeyExportOutput, "out.json")
	// 调优类与设置类：不应被清理
	s.SetStr(KeyDownloadDir, `E:\dl`)
	s.SetBool(KeyDownloadDesc, true)
	s.SetStr(KeySettingsTdlPath, `C:\tdl.exe`)

	s.ClearTaskScope()

	for _, k := range TaskScopeKeys {
		if v := p.String(k); v != "" {
			t.Fatalf("键 %s 应被清理，实际 %q", k, v)
		}
	}
	if got := s.Str(KeyDownloadDir, ""); got != `E:\dl` {
		t.Fatalf("下载目录属调优类，不应被清理，实际 %q", got)
	}
	if !s.Bool(KeyDownloadDesc, false) {
		t.Fatal("降序开关属调优类，不应被清理")
	}
	if got := s.Str(KeySettingsTdlPath, ""); got != `C:\tdl.exe` {
		t.Fatalf("设置类键不应被清理，实际 %q", got)
	}
}

func TestSetListMarksEmpty(t *testing.T) {
	p := newMemPrefs()
	s := NewStore(p)
	s.SetList(KeyDownloadJSONFiles, []string{"a.json", "b.json"})
	if p.m[KeyDownloadJSONFiles] != "a.json\nb.json" {
		t.Fatalf("序列化应为换行拼接，实际 %q", p.m[KeyDownloadJSONFiles])
	}
}
