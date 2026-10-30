package mpvproc

import (
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestBuildArgsForceWindowWhenNoWid(t *testing.T) {
	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 0)
	if !contains(args, "--force-window=yes") {
		t.Fatalf("无 wid 时应有 --force-window=yes: %v", args)
	}
	if contains(args, "--wid=") {
		t.Fatalf("无 wid 时不应有 --wid: %v", args)
	}
	if !contains(args, "--osc=yes") {
		t.Fatalf("独立窗口也应开启 OSC 控件: %v", args)
	}
}

func TestBuildArgsWidWhenSet(t *testing.T) {
	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 42)
	if !contains(args, "--wid=42") {
		t.Fatalf("有 wid 时应有 --wid=42: %v", args)
	}
	if contains(args, "--force-window") {
		t.Fatalf("有 wid 时不应有 --force-window: %v", args)
	}
	if !contains(args, "--osc=yes") {
		t.Fatalf("有 wid 时应开 OSC: %v", args)
	}
}

func TestBuildArgsCarriesHeaders(t *testing.T) {
	s := player.Stream{URL: "http://x/a.m3u8", Headers: map[string]string{"Referer": "http://x/"}}
	args := buildArgs(s, "/tmp/x.sock", 0)
	if !contains(args, "--http-header-fields=Referer: http://x/") {
		t.Fatalf("应携带 http header: %v", args)
	}
}

// 拿不到 DRM render 节点时（WSLg 默认不在 render 组、容器里干脆没有），
// mpv 的 EGL/Zink 初始化会失败，随后 x11 vo 二次初始化直接 assert 崩溃：
//
//	mpv: ../video/out/x11_common.c:679: vo_x11_init: Assertion `!vo->x11' failed.
//
// 这时必须走纯软件 x11 输出，画面照常，只是缩放吃 CPU。
func TestBuildArgsSoftwareVideoWithoutRenderNode(t *testing.T) {
	old := mpvVideoArgs
	mpvVideoArgs = func() []string { return softVideoArgs }
	defer func() { mpvVideoArgs = old }()

	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 0)
	if !contains(args, "--vo=x11") || !contains(args, "--hwdec=no") {
		t.Fatalf("无 GPU 时应退回软件输出: %v", args)
	}
}

func TestBuildArgsKeepsGpuVideoWhenAvailable(t *testing.T) {
	old := mpvVideoArgs
	mpvVideoArgs = func() []string { return nil }
	defer func() { mpvVideoArgs = old }()

	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 0)
	if contains(args, "--vo=x11") || contains(args, "--hwdec=no") {
		t.Fatalf("GPU 可用时不该压成软件输出: %v", args)
	}
}

// 环境判断要能被环境变量覆盖：用户实测有/没有显卡时的兜底通道。
func TestVideoOutputArgsEnvOverride(t *testing.T) {
	t.Setenv(envMpvSoft, "1")
	if got := videoOutputArgs(); len(got) == 0 {
		t.Fatal("UNBOX_MPV_SOFT=1 应强制软件输出")
	}
	t.Setenv(envMpvSoft, "0")
	if got := videoOutputArgs(); len(got) != 0 {
		t.Fatalf("UNBOX_MPV_SOFT=0 应强制硬件路径，got %v", got)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
