// tdl-GUI 是命令行工具 tdl（Telegram Downloader）的 Windows 桌面前端：
// 不重新实现 tdl 任何功能，仅作为命令构造器与运行器，实时回显 tdl.exe 输出。
package main

import (
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/lang"

	"tdl-gui/internal/i18n"
	"tdl-gui/internal/store"
	"tdl-gui/internal/ui"
)

// version 由构建时注入：-ldflags "-X main.version=x.y.z"
var version = "1.2.0"

func main() {
	a := app.NewWithID("tdl-gui")
	// 语言初始化：必须在 ui.Build（首次调用 i18n.T）之前完成。
	// 语言是启动期一次性决策（v1.2.0 方法 A：设置页改语言后重启生效），
	// 故 Init 放在 main 而非 ui.Build 内——Build 会被大量白盒测试直接调用，
	// 若在其中探测系统语言，测试机为英文时数百条中文断言会失败。
	i18n.Init(a.Preferences().String(store.KeySettingsLang), lang.SystemLocale().LanguageString())
	w := ui.Build(a, version)
	w.ShowAndRun()
}
