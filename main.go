// tdl-GUI 是命令行工具 tdl（Telegram Downloader）的 Windows 桌面前端：
// 不重新实现 tdl 任何功能，仅作为命令构造器与运行器，实时回显 tdl.exe 输出。
package main

import (
	"fyne.io/fyne/v2/app"

	"tdl-gui/internal/ui"
)

// version 由构建时注入：-ldflags "-X main.version=x.y.z"
var version = "1.0.0"

func main() {
	a := app.NewWithID("tdl-gui")
	w := ui.Build(a, version)
	w.ShowAndRun()
}
