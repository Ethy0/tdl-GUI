package runner

import (
	"strings"
	"syscall"
	"unsafe"
)

// CREATE_NEW_CONSOLE：为新进程分配一个独立的新控制台（可见终端窗口）。
const createNewConsole = 0x00000010

// consoleProc 表示在新建终端窗口中运行的子进程。
type consoleProc struct {
	pid    int
	handle syscall.Handle
}

// startInNewConsole 在新建的终端窗口中启动进程。
//
// 为什么不直接用 os/exec：os/exec 会把未显式设置的标准流接到 NUL，
// 子进程将无法交互式读取键盘输入（如 tdl login 的账号选择提示）。
// 这里直接调用 CreateProcessW 且不设置 STARTF_USESTDHANDLES，
// 使子进程使用新控制台的标准句柄，从而可以正常交互。
func startInNewConsole(path string, args []string) (*consoleProc, error) {
	// 只用命令行传程序名（lpApplicationName 传 nil）：
	// 这样可沿用 Windows 的标准搜索顺序（含 PATH），与 exec.Command 的解析行为一致。
	argv := append([]string{path}, args...)
	cmdLine, err := syscall.UTF16PtrFromString(buildCmdLine(argv))
	if err != nil {
		return nil, err
	}

	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi syscall.ProcessInformation

	// inheritHandles=false：不继承本进程句柄；不设置 si.Flags → 不使用 STARTF_USESTDHANDLES
	if err := syscall.CreateProcess(nil, cmdLine, nil, nil, false,
		createNewConsole, nil, nil, &si, &pi); err != nil {
		return nil, err
	}
	_ = syscall.CloseHandle(pi.Thread)

	return &consoleProc{pid: int(pi.ProcessId), handle: pi.Process}, nil
}

// wait 等待进程退出并返回退出码（随后关闭进程句柄）。
func (c *consoleProc) wait() int {
	_, _ = syscall.WaitForSingleObject(c.handle, syscall.INFINITE)
	var code uint32
	_ = syscall.GetExitCodeProcess(c.handle, &code)
	_ = syscall.CloseHandle(c.handle)
	return int(code)
}

// kill 强制结束该进程（taskkill 失败时的兜底），返回错误供结果上报。
// 注意：进程已退出时 TerminateProcess 会返回错误，由调用方计入诊断信息。
func (c *consoleProc) kill() error {
	return syscall.TerminateProcess(c.handle, 1)
}

// buildCmdLine 按 Windows 规则转义并拼接命令行。
func buildCmdLine(argv []string) string {
	var b strings.Builder
	for i, a := range argv {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(syscall.EscapeArg(a))
	}
	return b.String()
}
