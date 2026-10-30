package mpvproc

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// 命令应答超时是最常见的 mpv 失败形态，报错必须带上 mpv 自己的 stderr，
// 否则只剩一句干巴巴的「命令应答超时」，无从定位是网络还是解码问题。
func TestSendTimeoutCarriesMpvStderr(t *testing.T) {
	p := &mpvProc{
		responses: make(chan response, 16),
		conn:      unresponsiveConn{},
		session:   1,
	}
	p.stderr = &stderrTail{max: 1024}
	_, _ = p.stderr.Write([]byte("Failed to resolve 'example.com'"))

	oldTimeout := cmdResponseTimeout
	cmdResponseTimeout = 30 * time.Millisecond
	defer func() { cmdResponseTimeout = oldTimeout }()

	err := p.send("loadfile", "http://x/a.m3u8", "replace")
	if err == nil || !strings.Contains(err.Error(), "Failed to resolve") {
		t.Fatalf("应答超时应带上 mpv stderr，got %v", err)
	}
}

// unresponsiveConn 是「写得进、永远不应答」的假连接，用来触发命令应答超时。
type unresponsiveConn struct{}

func (unresponsiveConn) Write(p []byte) (int, error) { return len(p), nil }
func (unresponsiveConn) Read(p []byte) (int, error)  { return 0, io.ErrClosedPipe }
func (unresponsiveConn) Close() error                { return nil }

func TestLoadPlayClose(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("本机无 mpv，跳过集成测试")
	}
	p, err := New(mpv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 用本地生成的静音短视频？M1 不引入 ffmpeg，这里用一个不会立即 EOF 的
	// 短循环流不好造，退而求其次：验证 Load 能启动 mpv 并建立 IPC 即可，
	// 播放本身由 Task 5 的冒烟在真实环境人工确认。
	if err := p.Load(ctx, player.Stream{URL: "https://example.com/none.m3u8"}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st := p.State(); st.Playing != player.StatePlaying {
		t.Fatalf("Load 后 Playing = %v, want playing", st.Playing)
	}
	if err := p.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
}

// TestConcurrentReload 并发交错 Load/Play/Pause/Seek/Close，令旧会话 send 与
// 新会话 Load 重叠，验证：会话代际丢弃跨会话串味的迟到应答、指针同一性判断
// 不误杀后继会话，且全程无 race/panic/死锁。末尾再做一次 Load+Play+Pause 确认
// 实例仍可用。
func TestConcurrentReload(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("本机无 mpv，跳过集成测试")
	}
	p, err := New(mpv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const url = "https://example.com/none.m3u8"

	// 初始会话。
	if err := p.Load(ctx, player.Stream{URL: url}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				_ = p.Load(ctx, player.Stream{URL: url})
				_ = p.Play()
				_ = p.Pause()
				_ = p.Seek(1)
				_ = p.Close()
			}
		}()
	}
	wg.Wait()

	// 结束后仍可用：串味被代际丢弃，无死锁。
	if err := p.Load(ctx, player.Stream{URL: url}); err != nil {
		t.Fatalf("末尾 Load: %v", err)
	}
	if err := p.Play(); err != nil {
		t.Fatalf("末尾 Play: %v", err)
	}
	if err := p.Pause(); err != nil {
		t.Fatalf("末尾 Pause: %v", err)
	}
}

func TestSendEventTerminalBlocksUntilRead(t *testing.T) {
	ch := make(chan player.Event) // 无缓冲：无人读取时必然阻塞
	done := make(chan struct{})
	go func() {
		sendEvent(ch, player.Event{Kind: player.EventEOF})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("终端事件在无人读取时被丢弃了（不应发生）")
	case <-time.After(20 * time.Millisecond):
		// 预期：阻塞在发送上，done 未关闭
	}
	<-ch // 读取后放行
	<-done
}

func TestSendEventPositionDropsWhenFull(t *testing.T) {
	ch := make(chan player.Event, 1)
	ch <- player.Event{Kind: player.EventPosition}          // 占满
	sendEvent(ch, player.Event{Kind: player.EventPosition}) // 应丢弃而非阻塞
}
