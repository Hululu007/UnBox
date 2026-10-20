package playback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestParseBiliComposite(t *testing.T) {
	raw := "117461315749788+42845144529+32:16+5riF5pmwIDQ4MFA=:5rWB55WFIDM2MFA="
	aid, cid, qs, ok := parseBiliComposite(raw)
	if !ok {
		t.Fatal("复合地址应被识别为 bilibili 格式")
	}
	if aid != "117461315749788" || cid != "42845144529" {
		t.Fatalf("aid/cid 解析错误: %q %q", aid, cid)
	}
	if len(qs) != 2 || qs[0] != "32" || qs[1] != "16" {
		t.Fatalf("画质列表解析错误: %+v", qs)
	}
}

func TestParseBiliCompositeRejectsNonBili(t *testing.T) {
	for _, raw := range []string{
		"https://example.com/v.mp4",
		"plainstring",
		"117461315749788",
		"",
	} {
		if _, _, _, ok := parseBiliComposite(raw); ok {
			t.Fatalf("非 bilibili 复合地址不应被识别: %q", raw)
		}
	}
}

// Resolver 对 bilibili 复合地址应改为调用 playurl API 返回真实流，而不是
// 因冒号解析失败直接报「播放地址无效」。
func TestResolverResolvesBiliComposite(t *testing.T) {
	var gotAid, gotCid, gotQn, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotAid, gotCid, gotQn = q.Get("avid"), q.Get("cid"), q.Get("qn")
		gotReferer = r.Header.Get("Referer")
		_, _ = w.Write([]byte(`{"code":0,"data":{"durl":[{"url":"https://upos-sz-mirror.bilivideo.com/real/stream.m4s"}]}}`))
	}))
	defer srv.Close()

	orig := biliPlayURLBase
	biliPlayURLBase = srv.URL + "/x/player/playurl"
	defer func() { biliPlayURLBase = orig }()

	r := NewResolver(srv.Client())
	stream, err := r.Resolve(context.Background(), player.Stream{
		URL: "117461315749788+42845144529+32:16+5riF5pmwIDQ4MFA=:5rWB55WFIDM2MFA=",
	})
	if err != nil {
		t.Fatalf("bilibili 复合地址解析失败: %v", err)
	}
	if gotAid != "117461315749788" || gotCid != "42845144529" || gotQn != "32" {
		t.Fatalf("playurl 请求参数错误: avid=%q cid=%q qn=%q", gotAid, gotCid, gotQn)
	}
	if gotReferer != "https://www.bilibili.com" {
		t.Fatalf("Referer 应为 bilibili，实际 %q", gotReferer)
	}
	if stream.URL != "https://upos-sz-mirror.bilivideo.com/real/stream.m4s" {
		t.Fatalf("未返回真实播放地址: %q", stream.URL)
	}
}
