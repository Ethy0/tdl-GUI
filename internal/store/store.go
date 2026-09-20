// Package store 封装 Fyne Preferences 的读写：键名常量、默认值、多值字段序列化。
//
// 该包不依赖 fyne：仅依赖最小 Preferences 接口，fyne.Preferences 天然满足它，
// 测试时可用内存实现替换，保证可表驱动单测。
package store

import "strings"

// Preferences 是 store 需要的存储接口（fyne.Preferences 的子集）。
type Preferences interface {
	String(key string) string
	SetString(key string, value string)
	RemoveValue(key string)
}

// Store 是 Preferences 的封装。
type Store struct {
	p Preferences
}

// NewStore 创建 Store。
func NewStore(p Preferences) *Store { return &Store{p: p} }

// Str 读取字符串；未写入（空串）时返回 def。
func (s *Store) Str(key, def string) string {
	if v := s.p.String(key); v != "" {
		return v
	}
	return def
}

// SetStr 写入字符串。
func (s *Store) SetStr(key, v string) { s.p.SetString(key, v) }

// Bool 读取布尔值。
// Preferences 无法区分"未写入"与"false"，因此内部统一以 "true"/"false" 字符串存储，
// 未写入时返回 def（本应用全部布尔默认值均为 false）。
func (s *Store) Bool(key string, def bool) bool {
	switch s.p.String(key) {
	case "true":
		return true
	case "false":
		return false
	}
	return def
}

// SetBool 写入布尔值。
func (s *Store) SetBool(key string, v bool) {
	if v {
		s.p.SetString(key, "true")
		return
	}
	s.p.SetString(key, "false")
}

// List 读取以 \n 拼接的多值字段；空串或全空行返回 nil。
func (s *Store) List(key string) []string {
	raw := s.p.String(key)
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 8)
	for _, part := range strings.Split(raw, "\n") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SetList 以 \n 拼接写入多值字段。
func (s *Store) SetList(key string, v []string) {
	s.p.SetString(key, strings.Join(v, "\n"))
}

// 键名总表（规则：<页面>.<字段>，见设计书 §3.2）。
const (
	KeySettingsTdlPath           = "settings.tdlPath"
	KeySettingsNS                = "settings.ns"
	KeySettingsProxy             = "settings.proxy"
	KeySettingsNTP               = "settings.ntp"
	KeySettingsReconTimeout      = "settings.reconnectTimeout"
	KeySettingsPool              = "settings.pool"
	KeySettingsDelay             = "settings.delay"
	KeySettingsDebug             = "settings.debug"
	KeySettingsDisableProgressPS = "settings.disableProgressPS"
	KeySettingsStorage           = "settings.storage"

	KeyDownloadLinks      = "download.links"
	KeyDownloadTxtPath    = "download.txtPath"
	KeyDownloadJSONFiles  = "download.jsonFiles"
	KeyDownloadDir        = "download.dir"
	KeyDownloadThreads    = "download.threads"
	KeyDownloadLimit      = "download.limit"
	KeyDownloadDesc       = "download.desc"
	KeyDownloadRewriteExt = "download.rewriteExt"
	KeyDownloadGroup      = "download.group"
	KeyDownloadSkipSame   = "download.skipSame"
	KeyDownloadTakeout    = "download.takeout"
	KeyDownloadInclude    = "download.include"
	KeyDownloadExclude    = "download.exclude"
	KeyDownloadTemplate   = "download.template"
	KeyDownloadResume     = "download.resume"
	KeyDownloadServe      = "download.serve"
	KeyDownloadPort       = "download.port"

	KeyUploadPaths   = "upload.paths"
	KeyUploadChat    = "upload.chat"
	KeyUploadTopic   = "upload.topic"
	KeyUploadTo      = "upload.to"
	KeyUploadThreads = "upload.threads"
	KeyUploadLimit   = "upload.limit"
	KeyUploadCaption = "upload.caption"
	KeyUploadInclude = "upload.include"
	KeyUploadExclude = "upload.exclude"
	KeyUploadRM      = "upload.rm"
	KeyUploadPhoto   = "upload.photo"

	KeyExportChat        = "export.chat"
	KeyExportTopic       = "export.topic"
	KeyExportReply       = "export.reply"
	KeyExportOutput      = "export.output"
	KeyExportType        = "export.type"
	KeyExportRange       = "export.range"
	KeyExportFilter      = "export.filter"
	KeyExportWithContent = "export.withContent"
	KeyExportAll         = "export.all"
	KeyExportRaw         = "export.raw"

	KeyLoginNS       = "login.ns"
	KeyLoginPasscode = "login.passcode"
	KeyLoginTdDir    = "login.tdDir"
)

// TaskScopeKeys 是"任务目标范围"类输入：本次任务的目标/范围由它们决定，
// 不属于可复用的调优配置，因此**不持久化**——不写入、启动不读取，
// 并对外提供 ClearTaskScope 清理旧版本遗留值，确保上次未完成任务的链接
// 不会在下次启动时自动恢复。
var TaskScopeKeys = []string{
	KeyDownloadLinks, KeyDownloadTxtPath, KeyDownloadJSONFiles,
	KeyUploadPaths, KeyUploadChat, KeyUploadTopic, KeyUploadTo,
	KeyExportChat, KeyExportTopic, KeyExportReply,
	KeyExportOutput, KeyExportType, KeyExportRange, KeyExportFilter,
}

// ClearTaskScope 删除全部任务目标范围键（清理旧版本遗留的持久化记录）。
func (s *Store) ClearTaskScope() {
	for _, k := range TaskScopeKeys {
		s.p.RemoveValue(k)
	}
}
