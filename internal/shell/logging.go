package shell

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
)

// 日志缓冲与日志文件的上限。
// 缓冲只影响「查看日志」能翻多久；文件保证重启后仍能看到上一会话的尾巴。
const (
	logBufferSize = 256 << 10 // 内存环形缓冲
	logFileMax    = 1 << 20   // 文件超过 1MB 时只保留最后一段
	logFileKeep   = 256 << 10
)

// logBuffer 是有界环形日志缓冲，供设置页「查看日志」使用。
type logBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newLogBuffer(max int) *logBuffer { return &logBuffer{max: max} }

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// appLogs 是全局日志缓冲（最近 256KB）。
var appLogs = newLogBuffer(logBufferSize)

var (
	logFileMu sync.Mutex
	logFile   *os.File
)

// logFilePath 返回日志文件位置；测试可用 UNBOX_LOG_FILE 指向临时目录。
func logFilePath() string {
	if p := os.Getenv("UNBOX_LOG_FILE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "unbox", "unbox.log")
}

// internalVersion 返回内部构建版本（debug.ReadBuildInfo 的 Main.Version）。
// 构建时开启 -buildvcs（默认 auto）且处于 git 仓库内，会原生产出伪版本：
//
//	无 tag → v0.0.0-<时间>-<commit>
//	有 tag → vX.Y.(Z+1)-0.<时间>-<commit>
//
// 未开 -buildvcs 或不在 git 仓库内构建时为 "(devel)"。
func internalVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

// InitLogging 把标准 log 重定向到 stderr + 内存环形缓冲 + 日志文件，并在每行前加上内部版本。
// 须在 main 启动最早处调用，保证后续所有 log 输出都被捕获。
//
// 启动时会把文件末尾一段回填进内存缓冲：进程重启后，「查看日志」仍能看到
// 上一会话最后一次报错，而不是一片空白。
func InitLogging() {
	path := logFilePath()
	writers := []io.Writer{os.Stderr, appLogs}
	if path != "" {
		if tail, f, err := openLogFile(path); err == nil {
			if len(tail) > 0 {
				_, _ = appLogs.Write(tail)
			}
			logFileMu.Lock()
			if logFile != nil {
				_ = logFile.Close()
			}
			logFile = f
			logFileMu.Unlock()
			writers = append(writers, f)
		}
	}
	log.SetOutput(io.MultiWriter(writers...))
	log.SetPrefix("[" + internalVersion() + "] ")
}

// openLogFile 打开（或创建）日志文件，返回需要回填到内存的上一会话末尾内容。
// 文件超过上限时截断成最后一段，避免长期运行无限增长。
func openLogFile(path string) ([]byte, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	var tail []byte
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			keep := max(0, len(raw)-logBufferSize)
			tail = raw[keep:]
			if info.Size() > logFileMax {
				trimmed := tail
				if len(trimmed) > logFileKeep {
					trimmed = trimmed[len(trimmed)-logFileKeep:]
				}
				if writeErr := os.WriteFile(path, trimmed, 0o644); writeErr == nil {
					tail = trimmed
				}
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return tail, f, nil
}

// GetLogs 返回最近捕获的日志（供设置页展示）。
func (s *ShellService) GetLogs() string {
	return appLogs.String()
}
