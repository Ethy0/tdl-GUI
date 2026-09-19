# tdl-GUI 设计书（design）

> 版本：v1.0（对应 proposal v0.3）
> 日期：2026-09-19
> 上游文档：`doc/proposal.md`（需求书，条款编号以 §P.x 引用）；tdl 官方文档 https://docs.iyear.me/tdl/

---

## 1. 系统架构

### 1.1 分层与模块

```
┌─────────────────────────────────────────────────────────┐
│  UI 层（internal/ui）                                     │
│  app.go 主窗口/导航/全局锁定  logview.go 运行日志黑框        │
│  download.go / upload.go / export.go / settings.go 四页   │
├─────────────────────────────────────────────────────────┤
│  控制层                                                   │
│  task.go TaskController 任务状态机（开始↔中止、锁定/解锁）    │
├─────────────────────────────────────────────────────────┤
│  服务层（无 UI 依赖，可独立单测）                            │
│  cmdgen   命令拼装（表单结构体 → []string args，纯函数）      │
│  runner   子进程生命周期（启动/流解析/中止/退出回调）          │
│  store    Preferences 封装（键名、默认值、读写、序列化）      │
│  txtload  txt 批量链接解析（空行/注释跳过）                  │
├─────────────────────────────────────────────────────────┤
│  平台层：os/exec + taskkill（Windows）｜Fyne v2（≥2.5）    │
└─────────────────────────────────────────────────────────┘
```

依赖方向自上而下单向：`ui → task → (cmdgen|runner|store|txtload)`；`cmdgen/runner/store/txtload` 之间互不依赖。除 `ui` 外的包**不 import fyne**（保证可表驱动单测）。

### 1.2 Go 包结构

```
tdl-gui/
├── main.go                     // 入口：app.NewWithID、主窗口组装
├── go.mod                      // module tdl-gui；fyne.io/fyne/v2 ≥ v2.5.0
├── internal/
│   ├── cmdgen/
│   │   ├── cmdgen.go           // GlobalFlags + 4 个 Builder + 校验
│   │   └── cmdgen_test.go      // 表驱动测试
│   ├── runner/
│   │   ├── runner.go           // Runner：Start/Abort/Running
│   │   ├── parser.go           // \r/\n 行解析状态机
│   │   ├── parser_test.go
│   │   ├── ansi.go             // ANSI 转义剥离
│   │   └── ansi_test.go
│   ├── store/
│   │   └── store.go            // Store：Preferences 读写封装
│   ├── txtload/
│   │   ├── txtload.go          // 解析 txt → []string 链接
│   │   └── txtload_test.go
│   └── ui/
│       ├── app.go              // App：窗口、导航、页面切换、全局锁定
│       ├── task.go             // TaskController：状态机
│       ├── logview.go          // LogView：日志黑框组件
│       ├── page_download.go
│       ├── page_upload.go
│       ├── page_export.go
│       └── page_settings.go    // 全局参数 + 登录管理两区块
└── doc/
    ├── proposal.md
    └── design.md               // 本文件
```

### 1.3 一次任务的端到端数据流

```
用户点击[开始]
→ Page 收集表单 → struct（DownloadArgs 等）
→ cmdgen.BuildXxx(args) → ([]string cliArgs, error)     // 校验失败则弹错，不启动
→ LogView.AppendLine("$ tdl.exe <args>")                // 回显（脱敏）
→ TaskController.Start()                                // 按钮变红[中止] + 全局锁定
→ runner.Start(tdlPath, cliArgs, onEvent)
→ tdl 子进程 ──stdout/stderr──→ 行解析器 ──→ Event{Line|Progress}
       │                                    │（fyne.Do）
       │                              LogView 渲染
→ 进程退出 → Event{Exit}
→ TaskController.Finish()                               // 标注退出码、解锁、按钮复原
```

---

## 2. 关键技术决策

| # | 决策 | 理由 |
|---|---|---|
| D1 | Fyne **≥ v2.5.0**，UI 更新一律经 `fyne.Do(func(){...})` | v2.5 起统一主线程调度 API，消除 goroutine 直接改 UI 的数据竞争（§P.7-4） |
| D2 | 日志黑框用 `widget.TextGrid` + `container.NewScroll` | TextGrid 支持按行写入/替换，等宽字体，可设行级前景/背景色 |
| D3 | 持久化用 `app.NewWithID("tdl-gui")` 的 Preferences | AppID 隔离存储；Windows 下由 Fyne 框架持久化于用户系统配置目录（框架内部管理），不出现在软件目录、不随 exe 分发、用户无感知（§P.1、§P.8） |
| D4 | 子进程 `SysProcAttr{CreationFlags: 0x08000000 /*CREATE_NO_WINDOW*/, HideWindow: true}` | GUI 程序无控制台，不加此 flag 每次执行会闪现黑色控制台窗口 |
| D5 | 中止用 `taskkill /T /F /PID <pid>`（兜底 `cmd.Process.Kill()`） | /T 杀整棵进程树、/F 立即强杀，满足"立刻杀死"硬性要求（§P.4.1）；taskkill 自身也加 D4 隐藏窗口 |
| D6 | stdout/stderr 各自 pipe，两个 goroutine 读，写入同一把互斥锁保护的解析器 | Windows 无法把两个 pipe 合一；锁保证行序不交错错乱 |
| D7 | `taskkill`/路径等含空格参数由 `exec.Command` 自动加引号 | 不自行拼命令行字符串，规避转义 bug |
| D8 | 登录密码（passcode）仅存于 Preferences、回显命令行时脱敏为 `***` | 防止肩窥/日志截图泄露 |
| D9 | cmdgen 为纯函数包，全部映射集中在该包 | tdl flag 变化时只改一处（§P.11 风险对策） |

---

## 3. 数据模型与持久化（store）

### 3.1 表单结构体（cmdgen 与 ui 共用）

```go
type GlobalFlags struct {          // 设置页 5.4.1
    TdlPath  string                // tdl.exe 完整路径（必填校验）
    NS       string                // 当前命名空间，默认 "default"
    Proxy    string                // --proxy
    NTP      string                // --ntp
    ReconTimeout string            // --reconnect-timeout，默认 "2m"
    Pool     string                // --pool，默认 "8"（字符串存储，数字校验）
    Delay    string                // --delay，默认 "0s"
    Debug    bool                  // --debug
    NoProgPS bool                  // --disable-progress-ps
    Storage  string                // --storage（空=不拼入）
}

type DownloadArgs struct {
    Links    []string              // 手动输入框逐行
    TxtLinks []string              // txtload 解析产物（运行时派生，不持久化）
    JSONFiles []string             // -f 列表
    Dir      string                // -d
    Threads, Limit string          // -t / -l（数字校验，空=不拼）
    Desc, RewriteExt, Group, SkipSame, Takeout bool
    Include, Exclude, Template string   // -i / -e / --template
    Resume   string                // "default" | "continue" | "restart"
    Serve    bool
    Port     string                // 数字校验；仅 Serve=true 时拼入，默认 "8080"
}

type UploadArgs struct {
    Paths    []string              // -p 列表
    Chat     string                // -c
    Topic    string                // --topic（数字校验）
    To       string                // --to（与 Chat/Topic 互斥）
    Threads, Limit string
    Caption  string                // --caption
    Include, Exclude string
    RM, Photo bool
}

type ExportArgs struct {
    Chat   string                  // -c
    Topic  string                  // --topic（数字）
    Reply  string                  // --reply（数字）
    Output string                  // -o，默认 "tdl-export.json"
    Type   string                  // "time" | "id" | "last"
    Range  string                  // -i
    Filter string                  // -f
    WithContent, All, Raw bool
}

type LoginOp int                   // 设置页 5.4.2 登录区
const ( LoginNew LoginOp = iota; LoginList; LoginUse; LoginLogout )

type LoginArgs struct {
    Op       LoginOp
    NS       string                // 各操作的目标命名空间
    Passcode string                // 仅 LoginNew
    TdDir    string                // 仅 LoginNew（客户端目录）
}
```

### 3.2 Preferences 键名总表

键名规则 `页面.字段`（§P.8）；多值字段以 `\n` join 存单字符串（Preferences 无切片类型）；**所有写入均在字段 onChanged 回调中即时执行**。

| 键 | 类型 | 默认 | 对应字段 |
|---|---|---|---|
| `settings.tdlPath` | String | "" | GlobalFlags.TdlPath |
| `settings.ns` | String | "default" | GlobalFlags.NS |
| `settings.proxy` | String | "" | Proxy |
| `settings.ntp` | String | "" | NTP |
| `settings.reconnectTimeout` | String | "2m" | ReconTimeout |
| `settings.pool` | String | "8" | Pool |
| `settings.delay` | String | "0s" | Delay |
| `settings.debug` | Bool | false | Debug |
| `settings.disableProgressPS` | Bool | false | NoProgPS |
| `settings.storage` | String | "" | Storage |
| `download.links` | String(\n拼接) | "" | DownloadArgs.Links |
| `download.txtPath` | String | "" | txt 文件路径（启动时重新解析派生 TxtLinks） |
| `download.jsonFiles` | String(\n拼接) | "" | JSONFiles |
| `download.dir` | String | "" | Dir |
| `download.threads` / `download.limit` | String | "" | Threads/Limit |
| `download.desc/rewriteExt/group/skipSame/takeout` | Bool | false | 对应勾选 |
| `download.include` / `download.exclude` / `download.template` | String | "" | 对应字段 |
| `download.resume` | String | "default" | Resume |
| `download.serve` | Bool | false | Serve |
| `download.port` | String | "8080" | Port |
| `upload.paths` | String(\n拼接) | "" | UploadArgs.Paths |
| `upload.chat/topic/to/threads/limit/caption/include/exclude` | String | "" | 对应字段 |
| `upload.rm` / `upload.photo` | Bool | false | RM/Photo |
| `export.chat/topic/reply/output/range/filter` | String | ""（output 默认 "tdl-export.json"） | 对应字段 |
| `export.type` | String | "time" | Type |
| `export.withContent/all/raw` | Bool | false | 对应勾选 |
| `login.ns` | String | "default" | 登录区命名空间框 |
| `login.passcode` | String | "" | Passcode（D8） |
| `login.tdDir` | String | "" | TdDir |

### 3.3 Store 接口

```go
type Store struct{ p fyne.Preferences }

func NewStore(p fyne.Preferences) *Store
func (s *Store) Str(key, def string) string      // 读
func (s *Store) SetStr(key, v string)            // 写（onChanged 中调用）
func (s *Store) Bool(key string, def bool) bool
func (s *Store) SetBool(key string, v bool)
func (s *Store) List(key string) []string        // 读 \n 拼接字段（空串→nil）
func (s *Store) SetList(key string, v []string)  // strings.Join(v,"\n")
```

---

## 4. 命令拼装器（cmdgen）

### 4.1 接口

```go
func BuildGlobal(g GlobalFlags, withNS bool) []string          // -n/--proxy/... 顺序固定
func BuildDownload(g GlobalFlags, d DownloadArgs) ([]string, error)
func BuildUpload(g GlobalFlags, u UploadArgs) ([]string, error)
func BuildExport(g GlobalFlags, e ExportArgs) ([]string, error)
func BuildLogin(g GlobalFlags, l LoginArgs) ([]string, error)
func EchoString(tdlPath string, args []string) string          // 回显用（含脱敏）
```

返回值均**不含 tdl.exe 路径本身**（路径由 runner 作为 exec.Command 第一参数）。通用规则（§P.6）：布尔勾选才拼入；字符串/数字非空才拼入；顺序为「全局 flags → 子命令 → 子命令 flags」。

### 4.2 全局 flags 拼装（BuildGlobal）

| 条件 | 拼入 |
|---|---|
| withNS=true 且 NS≠"" 且 ≠"default" | `-n <ns>`（等于 "default" 时省略） |
| Proxy≠"" | `--proxy <v>` |
| NTP≠"" | `--ntp <v>` |
| ReconTimeout≠"" 且 ≠"2m" | `--reconnect-timeout <v>` |
| Pool≠"" 且 ≠"8" | `--pool <v>` |
| Delay≠"" 且 ≠"0s" | `--delay <v>` |
| Debug | `--debug` |
| NoProgPS | `--disable-progress-ps` |
| Storage≠"" | `--storage <v>` |

等于默认值时省略可使命令行更短；语义不变（tdl 默认值与之一致）。

### 4.3 各 Builder 映射表

**BuildDownload → `dl` 子命令**（§P.5.1，顺序即拼入顺序）：

| 来源 | 拼入 |
|---|---|
| Links[i] | `-u <link>`（先手动） |
| TxtLinks[i] | `-u <link>`（后 txt，不去重） |
| JSONFiles[i] | `-f <file>` |
| Dir≠"" | `-d <dir>` |
| Threads | `-t <n>`；Limit → `-l <n>` |
| Desc→`--desc`；RewriteExt→`--rewrite-ext`；Group→`--group`；SkipSame→`--skip-same`；Takeout→`--takeout` |
| Include≠"" | `-i <exts>`（与 Exclude 互斥） |
| Exclude≠"" | `-e <exts>` |
| Template≠"" | `--template <v>` |
| Resume=="continue"→`--continue`；=="restart"→`--restart` |
| Serve | `--serve`；且 Port≠""→`--port <n>` |

**BuildUpload → `up`**（§P.5.2）：Paths[i]→`-p`；Chat→`-c`；Topic→`--topic`；To→`--to`；Threads/Limit→`-t/-l`；Caption→`--caption`；Include/Exclude→`-i/-e`；RM→`--rm`；Photo→`--photo`。

**BuildExport → `chat export`**（§P.5.3）：Chat→`-c`；Topic→`--topic`；Reply→`--reply`；Output≠""→`-o`；Type≠"time"→`-T <type>`；Range→`-i`；Filter→`-f`；WithContent→`--with-content`；All→`--all`；Raw→`--raw`。

**BuildLogin**（§P.5.4.2，全局 flags 位置各不相同）：

| Op | 拼装结果 |
|---|---|
| LoginNew | `[全局(含-n)] login [-p <pass>] [-d <dir>]`（`-p/-d` 是 login 子命令 flags，置于子命令后） |
| LoginList | `login -l`（不带全局，`-n` 无意义） |
| LoginUse | `use -n <ns>`（`-n` 是 use 子命令 flag，置于子命令后） |
| LoginLogout | `[全局(仅-n)] logout` |

### 4.4 校验规则（Builder 返回 error，UI 据此禁用/弹错）

| 规则 | 错误文案 |
|---|---|
| TdlPath 为空 | "请先在设置页选择 tdl.exe 路径" |
| 下载：Include 与 Exclude 同时非空 | "扩展名白名单与黑名单不能同时填写" |
| 下载：Links+TxtLinks+JSONFiles 全空 | "请至少提供一种输入源（链接 / txt / JSON）" |
| 下载：链接总数（Links+TxtLinks）>800 | "链接数量过多（N 条），命令行可能超长，请分批执行"（警告性错误，见 §10-R3） |
| 上传：Paths 为空 | "请至少添加一个待上传文件或目录" |
| 上传：To 与 (Chat 或 Topic) 同时非空 | "消息路由与目标聊天/话题互斥，只能二选一" |
| 登录：LoginNew/LoginUse/LoginLogout 时 NS 为空 | "请填写命名空间" |
| 数字字段（Threads/Limit/Port/Topic/Reply/Pool）非空且非正整数 | "字段 <名称> 必须为正整数" |
| 链接行不以 `https://t.me/` 或 `tg://` 开头 | 警告日志（不阻断）："链接格式可能不受支持：<link>" |

错误聚合：Builder 遍历全部字段后一次性返回 `errors.Join` 风格的聚合错误，UI 用 `dialog.NewError` 展示；同时字段级校验（数字、互斥）在输入时即时反馈（Entry.Validator / 校验状态红字），二者并存。

### 4.5 回显与脱敏（EchoString）

- 输出格式：`$ "<tdlPath>" "<arg1>" "<arg2>" ...`（路径与含空格参数带引号，便于用户复制到终端复现）。
- 脱敏：`login` 命令中 `-p` 的后继参数替换为 `***`；其余原样。

---

## 5. 进程管理器（runner）

### 5.1 接口与事件

```go
type Event interface{}
type LineEvent struct{ Text string }        // \n 终止的完整行
type ProgressEvent struct{ Text string }    // \r 触发的行刷新
type ExitEvent struct{ Code int; Err error; Aborted bool }

type Runner struct{ /* cmd, pid, events, aborted, mu */ }
func NewRunner() *Runner
func (r *Runner) Start(path string, args []string, onEvent func(Event)) error
func (r *Runner) Abort()                    // 幂等
func (r *Runner) Running() bool
```

`onEvent` 由**调用方保证**在 UI 线程语义下执行——实现上 Runner 内部对每个事件包一层 `fyne.Do`？**否**（D 决策：runner 不 import fyne）。约定：`ui.TaskController` 把自己的回调传给 Runner，回调内部自行用 `fyne.Do` 派发。

### 5.2 启动（Start）

1. `exec.Command(path, args...)`；`SysProcAttr` 按第 2 节 D4（隐藏控制台窗口）。
2. `StdoutPipe()` + `StderrPipe()`；两个 goroutine 各自 `io.Copy` 进 `syncWriter`（内部 `sync.Mutex` + 逐字节喂给 `lineParser`）。
3. `cmd.Start()`；成功后记录 pid，另起 goroutine `cmd.Wait()` → 发送 `ExitEvent{Code, Err, Aborted}`。
4. 环境变量：默认继承（不显式设置 Environ）。

### 5.3 行解析器（parser.go）

逐 rune 状态机（ansi 剥离在喂入前完成）：

```
状态：inLine=false
遇 '\r'：
    若 currentBuf 非空 → 发 ProgressEvent(strip(currentBuf))   // 同一行重绘
    inLine=true; currentBuf 清空                                // 后续内容覆盖本行
遇 '\n'：
    发 LineEvent(strip(currentBuf)); currentBuf 清空; inLine=false
EOF/管道关闭：
    若 currentBuf 非空 → 发 LineEvent(strip(currentBuf))       // 最后一行无换行
其余 rune → currentBuf 追加
```

tdl 的进度输出形态为 `\r<重绘整行>`（spinner/百分比/速度），本算法"遇 \r 先发上段、清缓冲"正好实现"同一行内容整体替换"。`CommitProgress` 语义由 LogView 内部处理（见 §6）。

### 5.4 ANSI 剥离（ansi.go）

```go
var csiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var oscRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
func Strip(s string) string   // 依次去 OSC、CSI，再滤掉裸 \x1b 及其他 C0 控制符（保留 \r\n\t）
```

### 5.5 中止（Abort，硬性要求 §P.4.1）

```
1. 原子置 aborted=true（幂等保护，防连点）
2. exec.Command("taskkill", "/T", "/F", "/PID", pid)（加 D4 隐藏窗口）
3. 不等待 taskkill 返回即视为已提交；cmd.Wait() 自然返回 → ExitEvent{Aborted:true}
4. 兜底：若 taskkill 调用出错（如系统无 taskkill），退回 cmd.Process.Kill()
5. UI 侧收到 Abort 调用后立即在日志写入醒目标注行（不等 ExitEvent）
   格式："========= 任务已被用户中止（强制终止进程树）========="
```

### 5.6 退出处理

`ExitEvent` 中：Code 为 tdl 退出码；`Err` 为 Wait 错误（含被杀后的 "TerminateProcess" 类错误，此时 Code<=0）。TaskController 收到后：日志写入 `退出码：<code>（正常/失败/已中止）`，执行解锁流程（§7）。

---

## 6. 运行日志组件（LogView）

### 6.1 API

```go
type LogView struct{ /* grid, scroll, lines, mu, dirty... */ }
func NewLogView() *LogView
func (l *LogView) CanvasObject() fyne.CanvasObject   // 顶部工具行 + 黑框 的容器
func (l *LogView) AppendLine(line string)            // 线程安全（内部 fyne.Do）
func (l *LogView) UpdateProgress(line string)        // \r 行内刷新
func (l *LogView) CommitProgress()                   // 任务结束时固化残留进度行
func (l *LogView) AppendBanner(text string)          // 醒目标注行（命令回显/中止/退出码）
func (l *LogView) CopyAll()                          // 工具栏[复制全部]
func (l *LogView) Clear()                            // 工具栏[清空]（仅 UI 缓冲，不影响运行中任务）
```

### 6.2 结构与样式

- 外层：`container.NewBorder(工具栏, nil, nil, nil, scroll)`；工具栏 = [自动滚动✓] [复制全部] [清空] 三个小控件（**运行期间保持可用**，§P.4.1 例外条款）。
- `scroll := container.NewVScroll(grid)`；`grid := widget.NewTextGrid()`。
- 单元格样式（`widget.TextGridStyleCustom`）：背景 `#1E1E1E`，普通文本 `#D4D4D4`；Banner 行文本 `#FFB86C`；"已中止" Banner 用 `#FF5555`。
- 自动滚动：勾选状态下每次渲染后 `scroll.ScrollToBottom()`；用户手动向上滚动不自动强制拉回（只有勾选态生效，简化实现，不监听滚动位置）。

### 6.3 渲染算法与性能

- 内部维护 `lines []string`（权威缓冲，CopyAll/Clear 操作它）。
- `AppendLine`：`lines = append(lines, s)`；`grid.SetRow(len(lines)-1, row(s))`。
- `UpdateProgress`：进度行**不进入 lines**，单独渲染为 TextGrid 末尾"临时行"；每次调用用 `SetRow(progressRowIdx, ...)` 整行替换（行宽变化时旧字符残留 → 替换前按较长长度补空格，保证整行覆盖）。
- `CommitProgress`：把当前临时行落入 lines 转为正式行，撤掉临时行。
- **节流**：`UpdateProgress` 每 100ms 最多真正渲染一次（time.Ticker + pending 缓冲），进度事件丢弃式合并（只保留最新），普通行不节流。
- **上限**：lines 超过 5000 行时丢弃最旧 2000 行并整表重建（`grid.SetText(strings.Join(...))`）；重建频率极低，成本可接受。

---

## 7. 任务状态机（TaskController，两项硬性要求的实现核心）

### 7.1 状态

```
                Start 成功                        
  ┌─────────┐ ────────────▶ ┌─────────┐  Wait 返回  ┌─────────┐
  │  Idle   │               │ Running │ ──────────▶ │ Finishing│ ─▶ Idle
  └─────────┘ ◀──────────── └─────────┘             └─────────┘
     ▲  ▲        按钮复原/解锁       │ Abort()
     │  └─── Start 失败：弹错，保持 Idle（不锁定）
     │
     └── Finishing：日志写退出码 → 解锁 → 恢复按钮 → Idle
```

```go
type TaskController struct {
    runner *runner.Runner
    log    *LogView
    // 运行前登记的锁集与触发按钮
    lockables []fyne.Disableable       // 导航按钮 + 四页全部表单控件（禁用后随页面切换不影响，Disable 状态跨 Show/Hide 保持）
    trigger   *widget.Button           // 发起本次任务的按钮
    triggerReset func()                // 恢复该按钮（文案、Importance、回调）
    busy  atomic.Bool
}
func (t *TaskController) Launch(tdlPath string, args []string, echo string,
        trigger *widget.Button, reset func())  // 校验通过后调用
func (t *TaskController) Abort()
```

### 7.2 Launch 流程（对应 §P.4.1 两条硬性要求）

1. `busy.CompareAndSwap(false,true)`，失败直接 return（单任务模型）。
2. 回显：`log.AppendBanner(echo)`。
3. **触发按钮立即变形**：`trigger.SetText("中止")`、`trigger.Importance = widget.HighImportance`（红）、`trigger.OnTapped = t.Abort`。
4. **全局锁定**：遍历 `lockables` 逐个 `Disable()`（导航 4 按钮 + 各页全部 Entry/Check/RadioGroup/Select/文件选择按钮/其余任务按钮；日志工具栏不在集合内）。
5. `runner.Start(...)`；失败（exe 不存在等）→ 弹 `dialog.NewError`，执行第 7 步恢复，busy=false。
6. 事件回调（内部 `fyne.Do`）：Line→`log.AppendLine`；Progress→`log.UpdateProgress`；Exit→`log.CommitProgress()` + `log.AppendBanner("退出码 N（状态）")` → 第 7 步。
7. **解锁恢复**：全部 `Enable()`（TdlPath 未配置时开始按钮保持禁用——恢复逻辑按"当前合法性"重算而非简单全 Enable）；`triggerReset()` 恢复原文案/样式/回调；busy=false。

### 7.3 页面与控件的登记机制

- 每个页面构建函数签名统一：`func NewXxxPage(ctx *PageCtx) (*Page)`，`Page` 含 `View fyne.CanvasObject` 与 `Controls []fyne.Disableable`（构建时自动收集本页所有可禁用控件）。
- `App` 汇总四页 Controls + 导航按钮 → TaskController.lockables。
- 校验驱动的按钮初始状态（如 TdlPath 为空时 [开始] 禁用）由各页 `Refresh()` 计算；解锁恢复时调用全部页面 `Refresh()`。

---

## 8. UI 详细设计

### 8.1 主窗口（app.go / main.go）

```go
a := app.NewWithID("tdl-gui")            // D3：Preferences 隔离键
w := a.NewWindow("tdl-GUI")
w.Resize(fyne.NewSize(1000, 640))        // §P.9
```

```
主布局 = container.NewBorder(nil,nil, nav, nil, right)
nav    = VBox[导航按钮×4]（宽度固定 ~120；当前页高亮 Importance=High）
right  = container.NewVSplit(logView, pageStack)；Split.SetOffset(0.4)   // 日志占 40%（§P.9）
pageStack = container.NewStack(p1.View, p2.View, p3.View, p4.View)      // Hide/Show 切换
```

- 页面切换：导航按钮 OnTapped → 隐其余显目标页（内容常驻，切换不丢状态；表单状态本就实时持久化）。
- **关窗拦截**：`w.SetCloseIntercept(...)`：Idle → 直接 Close；Running → `dialog.NewConfirm("任务运行中","关闭窗口将强制终止任务,确认?", func(ok){ if ok {runner.Abort(); w.Close()} })`。
- 首次启动（§P.5.4.1）：启动时读 `settings.tdlPath`，为空则默认切到设置页并 `log.AppendBanner("首次使用：请在设置页选择 tdl.exe 路径")`。

### 8.2 下载页（page_download.go）

自上而下顺序（§P.5.1；全部控件入本页 Controls）：

| # | 控件 | 类型 | 绑定/行为 |
|---|---|---|---|
| 1 | 消息链接 | MultiLineEntry | ↔ `download.links`（\n 即列表） |
| 2 | txt 批量导入 | Button[选择txt] + Label(路径) + Label("已解析 N 条链接") | dialog.NewFileOpen(.txt)；选择后 txtload 解析 → 缓存 TxtLinks + 更新 N；路径↔`download.txtPath`（启动时重解析） |
| 3 | 导出 JSON 文件 | Button[添加]×1 + List(widget.List) + Button[移除选中] | ↔ `download.jsonFiles`；文件多选对话框 |
| 4 | 下载目录 | Entry(只读) + Button[浏览] | dialog.NewFolderOpen ↔ `download.dir` |
| 5 | 线程数 / 并发数 | Entry×2 | Validator=正整数或空 |
| 6 | 五个勾选 | Check×5 | desc / rewriteExt / group / skipSame / takeout（rewriteExt、takeout 带说明小字） |
| 7 | 白/黑名单 | Entry×2 | 互斥即时校验（冲突时两框红字提示 + [开始] 禁用） |
| 8 | 文件名模板 | Entry | ↔ template |
| 9 | 断点策略 | RadioGroup(默认/续传/重新开始) | ↔ download.resume |
| 10 | serve | Check + Entry(端口,默认8080) | Check=false 时端口框 Disable |
| 11 | [开始] | Button（触发前 HighImportance 样式） | 点击→collect→BuildDownload→Launch |

### 8.3 上传页（page_upload.go）

| # | 控件 | 类型 | 绑定 |
|---|---|---|---|
| 1 | 待上传路径 | widget.List + [添加文件][添加目录][移除选中] | ↔ `upload.paths`（文件 dialog.NewFileOpen 多选 / 目录 dialog.NewFolderOpen） |
| 2 | 目标聊天 / 话题ID | Entry + Entry(数字) | chat / topic；to 非空时二者 Disable |
| 3 | 消息路由 | Entry | ↔ to（与 2 互斥即时反馈） |
| 4 | 线程/并发 | Entry×2 | 数字校验 |
| 5 | 标题 caption | MultiLineEntry | ↔ caption |
| 6 | 白/黑名单 | Entry×2 | 互斥同下载页 |
| 7 | 删除本地文件 / 照片形式 | Check×2 | rm（红色警示小字）/ photo |
| 8 | [开始] | Button | 点击时若 RM=true 先 `dialog.NewConfirm` "上传成功后将删除本地文件，确认？"（§P.10-4）确认后继续 Launch |

### 8.4 导出消息页（page_export.go）

| # | 控件 | 类型 | 绑定 |
|---|---|---|---|
| 1 | 目标聊天 | Entry | ↔ export.chat（placeholder 注明"留空=收藏夹"） |
| 2 | 话题ID / 评论区帖子ID | Entry×2(数字) | topic / reply |
| 3 | 输出文件 | Entry(只读) + Button[另存为] | dialog.NewFileSave（缺省名 tdl-export.json）↔ export.output |
| 4 | 导出类型 | RadioGroup(按时间/按消息ID/最近N条) | ↔ export.type（time/id/last） |
| 5 | 范围 | Entry | ↔ export.range；placeholder 随类型切换（`起,止时间戳` / `起,止消息ID` / `数量N`） |
| 6 | 过滤表达式 | Entry | ↔ export.filter |
| 7 | 三个勾选 | Check×3 | withContent / all / raw（raw 标注"调试用"） |
| 8 | [开始] | Button | BuildExport→Launch |

### 8.5 设置页（page_settings.go，两区块 §P.5.4）

**区块 A：全局参数（widget.Form 布局）**

| 控件 | 类型 | 绑定 |
|---|---|---|
| tdl.exe 路径 | Entry(只读)+[浏览] | dialog.NewFileOpen(过滤.exe) ↔ settings.tdlPath；变更后触发全页 Refresh（开始按钮解锁） |
| 当前命名空间 | Entry | ↔ settings.ns |
| 代理 | Entry | ↔ settings.proxy（placeholder 示例格式） |
| NTP / 重连超时 / 延迟 | Entry×3 | ↔ ntp / reconnectTimeout / delay |
| DC 连接池 | Entry(数字) | ↔ pool |
| 调试 / 禁用资源统计 | Check×2 | ↔ debug / disableProgressPS |
| 存储 | Entry | ↔ storage（标注"高级，留空使用默认"） |
| [测试] | Button | BuildGlobal 无子命令 → args=`["--version"]`；作为任务 Launch（含按钮变红/锁定全流程） |

**区块 B：登录管理（Card 容器包裹，顶部说明小字：仅支持桌面客户端登录，客户端须从官网下载）**

| 控件 | 类型 | 行为 |
|---|---|---|
| 命名空间 | Entry（登录/登出共用） | ↔ login.ns |
| 本地密码 | Entry(Password) | ↔ login.passcode |
| 客户端目录 | Entry(只读)+[浏览] | dialog.NewFolderOpen ↔ login.tdDir |
| [登录新账户] | Button | BuildLogin(LoginNew)→Launch（该按钮变红[中止]） |
| [列出已登录账户] | Button | BuildLogin(LoginList)→Launch |
| [设为默认账户] | Button | BuildLogin(LoginUse, ns=login.ns)→Launch；成功后建议手动同步 settings.ns |
| [登出] | Button | BuildLogin(LoginLogout)→Launch |
| "设为默认"辅助 | Select | 可选列表 = 手填 + 上次 `tdl login -l` 输出中解析的 ns（简单实现：仅手填也满足验收 §P.10-5） |

### 8.6 对话框汇总

| 场景 | 类型 |
|---|---|
| cmdgen 校验失败 | dialog.NewError（聚合文案） |
| `--rm` 二次确认 | dialog.NewConfirm（红色警示文案） |
| runner.Start 失败（路径无效等） | dialog.NewError |
| 关窗时有任务运行 | dialog.NewConfirm |
| 非法数字/互斥冲突 | 控件旁红字（即时），不弹窗 |

---

## 9. 并发模型

| goroutine | 职责 | 退出条件 |
|---|---|---|
| 主（Fyne） | UI 事件、渲染 | 窗口关闭 |
| runner-stdout | io.Copy → syncWriter → parser | 管道 EOF |
| runner-stderr | 同上 | 管道 EOF |
| runner-wait | cmd.Wait → ExitEvent | 进程结束 |
| taskkill（Abort 时临时） | 执行 taskkill | 命令返回 |
| LogView 节流 ticker | 100ms flush 进度 | CommitProgress/新任务 |

数据竞争规避：
1. **一切 UI 变更只在主线程**：所有回调经 `fyne.Do`（D1）。
2. syncWriter 内 mutex 序列化 stdout/stderr 字节流 → parser 单线程语义。
3. LogView.lines 只在主线程读写（AppendLine 等入口即 fyne.Do 内部）；CopyAll 读取同样经 fyne.DoAndWait 或在锁内快照。
4. TaskController.busy 用 atomic.Bool；Abort 幂等。
5. Preferences 写入均在 Fyne 控件回调（主线程）中执行，无并发写。

---

## 10. 错误处理与边界场景

| # | 场景 | 处理 |
|---|---|---|
| E1 | tdlPath 未设置/文件不存在 | 各页 [开始] 禁用（Refresh 计算）；[测试] 点击时 Start 报错弹窗 |
| E2 | tdl 退出码非 0 | 日志 Banner："退出码 N（失败）"；不弹窗（tdl 自身错误已在日志输出） |
| E3 | tdl 等待 stdin（断点确认等） | 用户可用 [中止] 结束；下载页断点策略单选已规避主要场景（§P.5.1） |
| E4 | 连点 [开始]/[中止] | busy CAS + Abort 幂等 |
| E5 | 进程退出但管道残留输出 | Wait 前 parser 持续工作；ExitEvent 发送前先 flush（EOF 由 io.Copy 返回保证） |
| E6 | 日志暴涨 | §6.3 5000 行上限 |
| E7 | R3 命令行超长（链接极多） | cmdgen 阈值 800 链接报错提示分批（Windows CreateProcess 32K 字符上限防护） |
| E8 | txt 文件被移动/删除 | 启动重解析失败 → 日志警告 + TxtLinks 置空 + UI 显示"txt 解析失败"；不阻塞手动链接 |
| E9 | 中止后进程仍存活（taskkill 极端失败） | 兜底 Process.Kill()；ExitEvent 仍会触发解锁，界面不死锁 |
| E10 | 窗口缩放极小 | 日志 Split 最低高度保护（MinSize）；参数区 Scroll 包裹 |

---

## 11. 构建、打包与交付

- `go.mod`：`module tdl-gui`；`require fyne.io/fyne/v2 v2.5.x`；Go ≥ 1.21。
- 构建环境（开发者侧）：Windows + MinGW-w64（CGO，gcc in PATH）。
- 产物：`go build -trimpath -ldflags "-s -w -H=windowsgui" -o tdl-gui.exe .`
  - `-H=windowsgui` 去掉控制台宿主（配合 D4 双保险防黑窗）；
  - 亦可用 `fyne package -os windows`（等价产物，附带默认图标嵌入 exe 内部资源，不产生外部文件）。
- 交付物清单：**仅 `tdl-gui.exe`**（§P.10-1）。用户自备 `tdl.exe`（设置页指定路径）。
- 版本信息：`-ldflags "-X main.version=x.y.z"`，显示在窗口标题。

---

## 12. 测试计划

| 层 | 用例类型 | 内容 |
|---|---|---|
| cmdgen | 表驱动单测 | 每个 Builder：空值最小集、全字段、互斥冲突、数字非法、链接计数阈值、全局 flags 默认值省略、`-n` 位置（login/use 两种位置）、EchoString 脱敏 |
| runner.parser | 单测 | `\r` 刷新序列、`\r\n` 混合、无尾换行 EOF flush、中英文/宽字符行、空行 |
| runner.ansi | 单测 | CSI 颜色码、OSC 标题序列、裸 ESC、无转义原文 |
| txtload | 单测 | 空行/`#`注释/前后空白/Windows CRLF/空文件 |
| store | 单测（内存 Preferences mock） | 键读写、List 序列化往返、默认值 |
| LogView / UI | 手工测试 | 进度刷新不闪烁、5000 行截断、复制/清空、锁定变灰覆盖全部控件（逐页点验）、中止按钮立即变红、恢复后按钮状态正确 |
| 集成验收 | 手工 | proposal §10 的 8 条逐一映射执行（含真实 tdl.exe 冒烟：--version、下载单链接、export→download 回流、双命名空间登录） |

进度行渲染无自动化 UI 测试（Fyne TextGrid 断言成本高），以手工检查单（checklist 形式附在验收记录）覆盖。

---

## 13. 开发任务分解（映射 proposal §12 里程碑）

**M1 骨架**
1. go.mod + 目录骨架 + app.NewWithID + 主窗口/导航/Split 布局/页面切换
2. store 包 + 键名常量 + 序列化
3. runner 包（Start/parser/ansi/Abort/taskkill）+ 单测
4. LogView（Append/Progress/节流/上限/工具栏）+ TaskController（变形/锁定/恢复/关窗拦截）
5. 设置页区块 A（全局参数 + [测试] 按钮）+ 首启引导

**M2 下载页**
1. 表单 11 项 + 绑定持久化 + 校验
2. txtload 包 + 单测 + txt 导入交互（N 条提示/失败警告）
3. cmdgen.BuildDownload + 单测 + 联调

**M3 上传页 + 导出页**
1. 上传页（路径列表/互斥/rm 确认）+ cmdgen.BuildUpload + 单测
2. 导出页（类型联动 placeholder/另存为）+ cmdgen.BuildExport + 单测

**M4 登录区 + 收尾**
1. 设置页区块 B 登录管理 + cmdgen.BuildLogin（四种 Op）+ 单测
2. 导出→下载工作流联调、全部手工测试单、验收 §P.10
3. 打包单 exe、-H=windowsgui 验证无黑窗、交付

---

## 附录 A：proposal 条款 → design 章节对照

| proposal | design |
|---|---|
| §4.1 布局/日志/任务模型 | §6、§7、§8.1 |
| §5.1 下载 | §8.2、§4.3 |
| §5.2 上传 | §8.3、§4.3 |
| §5.3 导出消息 | §8.4、§4.3 |
| §5.4 设置+登录 | §8.5、§4.2/4.3 |
| §6 命令拼装 | §4 全节 |
| §7 进程与日志 | §5、§6、§9 |
| §8 持久化 | §3 |
| §11 风险 | §2-D5、§10（E1~E10 覆盖并扩展 R3 命令行超长） |
| §12 里程碑 | §13 |
