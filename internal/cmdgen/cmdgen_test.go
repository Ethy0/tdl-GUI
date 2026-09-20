package cmdgen

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBuildGlobal(t *testing.T) {
	tests := []struct {
		name   string
		g      GlobalFlags
		withNS bool
		want   []string
	}{
		{"all defaults omitted", GlobalFlags{NS: "default", ReconTimeout: "5m", Pool: "8", Delay: "0s"}, true, nil},
		{"recon 5m0s omitted", GlobalFlags{ReconTimeout: "5m0s"}, true, nil},
		{"empty all", GlobalFlags{}, true, nil},
		{"ns non-default", GlobalFlags{NS: "work"}, true, []string{"-n", "work"}},
		{"ns ignored when withNS=false", GlobalFlags{NS: "work"}, false, nil},
		{"ns default omitted", GlobalFlags{NS: "default"}, true, nil},
		{"recon custom", GlobalFlags{ReconTimeout: "1m30s"}, true, []string{"--reconnect-timeout", "1m30s"}},
		{"pool custom", GlobalFlags{Pool: "4"}, true, []string{"--pool", "4"}},
		{"delay custom", GlobalFlags{Delay: "5s"}, true, []string{"--delay", "5s"}},
		{"full order", GlobalFlags{
			NS: "work", Proxy: "socks5://localhost:1080", NTP: "ntp.aliyun.com",
			ReconTimeout: "1m30s", Pool: "4", Delay: "5s",
			Debug: true, NoProgPS: true, Storage: "type=bolt,path=D:/d",
		}, true, []string{
			"-n", "work",
			"--proxy", "socks5://localhost:1080",
			"--ntp", "ntp.aliyun.com",
			"--reconnect-timeout", "1m30s",
			"--pool", "4",
			"--delay", "5s",
			"--debug",
			"--disable-progress-ps",
			"--storage", "type=bolt,path=D:/d",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildGlobal(tt.g, tt.withNS)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestBuildDownloadMinimal(t *testing.T) {
	got, err := BuildDownload(GlobalFlags{TdlPath: "tdl.exe"}, DownloadArgs{Links: []string{"https://t.me/a/1"}})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{"dl", "-u", "https://t.me/a/1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildDownloadFull(t *testing.T) {
	g := GlobalFlags{
		TdlPath: "tdl.exe", NS: "work", Proxy: "socks5://localhost:1080",
		ReconTimeout: "1m30s", Pool: "4", Delay: "5s", Debug: true,
	}
	d := DownloadArgs{
		Links:     []string{"https://t.me/a/1"},
		TxtLinks:  []string{"https://t.me/b/2", " https://t.me/c/3 "},
		JSONFiles: []string{"C:/x/e.json"},
		Dir:       "D:/dl", Threads: "4", Limit: "2",
		Desc: true, RewriteExt: true, Group: true, SkipSame: true, Takeout: true,
		Include: "jpg,png", Template: "{{ .FileName }}",
		Resume: ResumeContinue, Serve: true, Port: "9090",
	}
	got, err := BuildDownload(g, d)
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{
		"-n", "work", "--proxy", "socks5://localhost:1080",
		"--reconnect-timeout", "1m30s", "--pool", "4", "--delay", "5s", "--debug",
		"dl",
		"-u", "https://t.me/a/1",
		"-u", "https://t.me/b/2",
		"-u", "https://t.me/c/3",
		"-f", "C:/x/e.json",
		"-d", "D:/dl", "-t", "4", "-l", "2",
		"--desc", "--rewrite-ext", "--group", "--skip-same", "--takeout",
		"-i", "jpg,png",
		"--template", "{{ .FileName }}",
		"--continue",
		"--serve", "--port", "9090",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestBuildDownloadRestartAndExclude(t *testing.T) {
	got, err := BuildDownload(GlobalFlags{TdlPath: "tdl.exe"}, DownloadArgs{
		JSONFiles: []string{"e.json"}, Exclude: "mp4,flv", Resume: ResumeRestart, Port: "8080",
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{"dl", "-f", "e.json", "-e", "mp4,flv", "--restart"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildDownloadErrors(t *testing.T) {
	tests := []struct {
		name string
		g    GlobalFlags
		d    DownloadArgs
		want string
	}{
		{"no tdl path", GlobalFlags{}, DownloadArgs{Links: []string{"https://t.me/a/1"}}, "请先在设置页选择 tdl.exe 路径"},
		{"no input", GlobalFlags{TdlPath: "t.exe"}, DownloadArgs{}, "请至少提供一种输入源"},
		{"include exclude conflict", GlobalFlags{TdlPath: "t.exe"},
			DownloadArgs{Links: []string{"l"}, Include: "jpg", Exclude: "mp4"}, "白名单与黑名单不能同时填写"},
		{"bad threads", GlobalFlags{TdlPath: "t.exe"},
			DownloadArgs{Links: []string{"l"}, Threads: "abc"}, "每任务线程数 必须为正整数"},
		{"zero limit", GlobalFlags{TdlPath: "t.exe"},
			DownloadArgs{Links: []string{"l"}, Limit: "0"}, "并发任务数 必须为正整数"},
		{"bad serve port", GlobalFlags{TdlPath: "t.exe"},
			DownloadArgs{Links: []string{"l"}, Serve: true, Port: "-1"}, "serve 端口 必须为正整数"},
		{"bad pool", GlobalFlags{TdlPath: "t.exe", Pool: "x"},
			DownloadArgs{Links: []string{"l"}}, "DC 连接池大小 必须为非负整数"},
		{"negative pool", GlobalFlags{TdlPath: "t.exe", Pool: "-1"},
			DownloadArgs{Links: []string{"l"}}, "DC 连接池大小 必须为非负整数"},
		{"pool zero allowed", GlobalFlags{TdlPath: "t.exe", Pool: "0"},
			DownloadArgs{Links: []string{"l"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildDownload(tt.g, tt.d)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("不应报错：%v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("应返回错误")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("错误文案应包含 %q，实际 %q", tt.want, err.Error())
			}
		})
	}
}

func TestBuildGlobalPoolZero(t *testing.T) {
	got := BuildGlobal(GlobalFlags{TdlPath: "t.exe", Pool: "0"}, true)
	if !reflect.DeepEqual(got, []string{"--pool", "0"}) {
		t.Fatalf("--pool 0（不限制）应被拼入，实际 %#v", got)
	}
}

func TestBuildDownloadTooManyLinks(t *testing.T) {
	links := make([]string, MaxLinkCount+1)
	for i := range links {
		links[i] = fmt.Sprintf("https://t.me/a/%d", i)
	}
	_, err := BuildDownload(GlobalFlags{TdlPath: "t.exe"}, DownloadArgs{Links: links})
	if err == nil || !strings.Contains(err.Error(), "链接数量过多") {
		t.Fatalf("超过 %d 条链接应报错，实际 %v", MaxLinkCount, err)
	}

	// 恰好 800 条：不报错
	links = links[:MaxLinkCount]
	if _, err := BuildDownload(GlobalFlags{TdlPath: "t.exe"}, DownloadArgs{Links: links}); err != nil {
		t.Fatalf("%d 条链接不应报错：%v", MaxLinkCount, err)
	}
}

func TestBuildUpload(t *testing.T) {
	got, err := BuildUpload(GlobalFlags{TdlPath: "t.exe", NS: "work"}, UploadArgs{
		Paths:   []string{"D:/a.zip", " D:/b "},
		Chat:    "@iyear",
		Threads: "4", Limit: "2",
		Caption: "hello",
		Include: "zip", RM: true, Photo: true,
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{
		"-n", "work", "up",
		"-p", "D:/a.zip", "-p", "D:/b",
		"-c", "@iyear",
		"-t", "4", "-l", "2",
		"--caption", "hello",
		"-i", "zip",
		"--rm", "--photo",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestBuildUploadTopicAndTo(t *testing.T) {
	got, err := BuildUpload(GlobalFlags{TdlPath: "t.exe"}, UploadArgs{
		Paths: []string{"a.zip"}, Chat: "123456789", Topic: "42",
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{"up", "-p", "a.zip", "-c", "123456789", "--topic", "42"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}

	got, err = BuildUpload(GlobalFlags{TdlPath: "t.exe"}, UploadArgs{
		Paths: []string{"a.zip"}, To: "expr://x",
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want = []string{"up", "-p", "a.zip", "--to", "expr://x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildUploadErrors(t *testing.T) {
	tests := []struct {
		name string
		u    UploadArgs
		want string
	}{
		{"no path", UploadArgs{Chat: "x"}, "请至少添加一个待上传文件或目录"},
		{"to conflicts chat", UploadArgs{Paths: []string{"a"}, Chat: "x", To: "y"}, "消息路由与目标聊天/话题互斥"},
		{"to conflicts topic", UploadArgs{Paths: []string{"a"}, Topic: "1", To: "y"}, "消息路由与目标聊天/话题互斥"},
		{"include exclude", UploadArgs{Paths: []string{"a"}, Include: "x", Exclude: "y"}, "白名单与黑名单不能同时填写"},
		{"bad topic", UploadArgs{Paths: []string{"a"}, Chat: "c", Topic: "x"}, "论坛话题 ID 必须为正整数"},
		{"topic without chat", UploadArgs{Paths: []string{"a"}, Topic: "5"}, "论坛话题 ID 必须与目标聊天同时填写"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildUpload(GlobalFlags{TdlPath: "t.exe"}, tt.u)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("错误文案应包含 %q，实际 %v", tt.want, err)
			}
		})
	}
}

func TestBuildExport(t *testing.T) {
	got, err := BuildExport(GlobalFlags{TdlPath: "t.exe"}, ExportArgs{
		Chat: "iyear", Output: "out.json", Type: ExportTypeTime, Range: "1000,2000",
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{"chat", "export", "-c", "iyear", "-o", "out.json", "-i", "1000,2000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildExportFull(t *testing.T) {
	got, err := BuildExport(GlobalFlags{TdlPath: "t.exe"}, ExportArgs{
		Chat: "iyear", Topic: "3", Reply: "55", Output: "out.json",
		Type: ExportTypeLast, Range: "100", Filter: "Views>200",
		WithContent: true, All: true, Raw: true,
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{
		"chat", "export",
		"-c", "iyear", "--topic", "3", "--reply", "55",
		"-o", "out.json", "-T", "last", "-i", "100", "-f", "Views>200",
		"--with-content", "--all", "--raw",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestBuildExportNoOutput(t *testing.T) {
	got, err := BuildExport(GlobalFlags{TdlPath: "t.exe"}, ExportArgs{Type: ExportTypeID})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{"chat", "export", "-T", "id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildExportErrors(t *testing.T) {
	tests := []struct {
		name string
		e    ExportArgs
		want string
	}{
		{"bad range", ExportArgs{Range: "1 2"}, "范围格式不正确"},
		{"bad range comma", ExportArgs{Range: "1,"}, "范围格式不正确"},
		{"bad topic", ExportArgs{Topic: "x"}, "话题 ID 必须为正整数"},
		{"bad reply", ExportArgs{Reply: "-3"}, "评论区帖子 ID 必须为正整数"},
		{"bad type", ExportArgs{Type: "wrong"}, "导出类型非法"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildExport(GlobalFlags{TdlPath: "t.exe"}, tt.e)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("错误文案应包含 %q，实际 %v", tt.want, err)
			}
		})
	}
}

func TestBuildLogin(t *testing.T) {
	got, err := BuildLogin(GlobalFlags{TdlPath: "t.exe"}, LoginArgs{NS: "work"})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want := []string{"-n", "work", "login"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}

	// -n 在 login 之前；-p / -d 在 login 之后
	got, err = BuildLogin(GlobalFlags{TdlPath: "t.exe"}, LoginArgs{
		NS: "work", Passcode: "1234", TdDir: `C:\Users\a\AppData\Roaming\Telegram Desktop`,
	})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	want = []string{"-n", "work", "login", "-p", "1234", "-d", `C:\Users\a\AppData\Roaming\Telegram Desktop`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}

	// default 命名空间省略 -n
	got, err = BuildLogin(GlobalFlags{TdlPath: "t.exe"}, LoginArgs{NS: "default"})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if !reflect.DeepEqual(got, []string{"login"}) {
		t.Fatalf("got %#v", got)
	}
}

func TestBuildLoginErrors(t *testing.T) {
	if _, err := BuildLogin(GlobalFlags{TdlPath: "t.exe"}, LoginArgs{}); err == nil ||
		!strings.Contains(err.Error(), "请填写命名空间") {
		t.Fatalf("空命名空间应报错，实际 %v", err)
	}
	if _, err := BuildLogin(GlobalFlags{}, LoginArgs{NS: "a b"}); err == nil ||
		!strings.Contains(err.Error(), "命名空间不能包含空白字符") {
		t.Fatalf("命名空间含空白应报错，实际 %v", err)
	}
	if _, err := BuildLogin(GlobalFlags{}, LoginArgs{NS: "work"}); err == nil ||
		!strings.Contains(err.Error(), "请先在设置页选择 tdl.exe 路径") {
		t.Fatalf("缺 tdl 路径应报错，实际 %v", err)
	}
}

func TestBuildTest(t *testing.T) {
	got, err := BuildTest(GlobalFlags{TdlPath: "t.exe"})
	if err != nil {
		t.Fatalf("不应报错：%v", err)
	}
	if !reflect.DeepEqual(got, []string{"version"}) {
		t.Fatalf("got %#v", got)
	}
	if _, err := BuildTest(GlobalFlags{}); err == nil {
		t.Fatal("缺 tdl 路径应报错")
	}
}

func TestEchoString(t *testing.T) {
	got := EchoString(`C:\tdl\tdl.exe`, []string{"-n", "work", "dl", "-u", "https://t.me/a/1"})
	want := `$ "C:\tdl\tdl.exe" "-n" "work" "dl" "-u" "https://t.me/a/1"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// login 密码脱敏
	got = EchoString("tdl.exe", []string{"-n", "work", "login", "-p", "secret", "-d", "dir"})
	if strings.Contains(got, "secret") {
		t.Fatalf("密码未脱敏：%q", got)
	}
	want = `$ "tdl.exe" "-n" "work" "login" "-p" "***" "-d" "dir"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// upload 的 -p 是路径，不脱敏
	got = EchoString("tdl.exe", []string{"up", "-p", "D:/a b.zip"})
	want = `$ "tdl.exe" "up" "-p" "D:/a b.zip"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSuspiciousLinks(t *testing.T) {
	got := SuspiciousLinks([]string{
		"https://t.me/a/1",
		"tg://resolve?domain=x",
		"http://t.me/a/2",
		"not-a-link",
		"",
	})
	want := []string{"http://t.me/a/2", "not-a-link"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if got := SuspiciousLinks([]string{"https://t.me/a/1"}); got != nil {
		t.Fatalf("全部合法时应返回 nil，实际 %#v", got)
	}
}
