package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 前端把 RuntimeError 交给 LogError 后，必须能在「查看日志」里看到——
// 这条链路（log.Printf → 环形缓冲 → GetLogs）此前被用户反馈过「日志为空」。
func TestLogErrorIsVisibleInGetLogs(t *testing.T) {
	t.Setenv("UNBOX_LOG_FILE", filepath.Join(t.TempDir(), "unbox.log"))
	InitLogging()
	svc := &ShellService{}
	msg := `RuntimeError: 请求失败: Get "https://haiwaikan.com/api.php/provide/vod?ac=videolist&pg=1": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
	svc.LogError(msg)
	if got := svc.GetLogs(); !strings.Contains(got, msg) {
		t.Fatalf("GetLogs 未包含 LogError 写入的内容，got=%q", got)
	}
}

// 日志同时落盘：用户重启后再打开「查看日志」，仍能看到上一会话末尾的报错。
func TestInitLoggingRestoresPreviousSessionTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unbox.log")
	t.Setenv("UNBOX_LOG_FILE", path)
	if err := os.WriteFile(path, []byte("[old] RuntimeError: 上一会话的报错\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	InitLogging()
	if got := (&ShellService{}).GetLogs(); !strings.Contains(got, "上一会话的报错") {
		t.Fatalf("重启后未恢复上一会话日志，got=%q", got)
	}
}

// LogError 必须同步落盘，不能只留在内存里。
func TestLogErrorWritesToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unbox.log")
	t.Setenv("UNBOX_LOG_FILE", path)
	InitLogging()
	(&ShellService{}).LogError("disk-marker")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取日志文件: %v", err)
	}
	if !strings.Contains(string(raw), "disk-marker") {
		t.Fatalf("日志文件未写入内容，got=%q", string(raw))
	}
}
