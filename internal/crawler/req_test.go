package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestReqInjectsAndCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wd") != "test" {
			t.Errorf("query wd=%q", r.URL.Query().Get("wd"))
		}
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Errorf("X-Test=%q", got)
		}
		_, _ = w.Write([]byte("resp-body"))
	}))
	defer srv.Close()

	e := New()
	e.installReq(http.DefaultClient)
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q, {headers:{"X-Test":"yes"}}) }`, srv.URL+"?wd=test")); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(e.vm)
	if got := obj.Get("content").String(); got != "resp-body" {
		t.Fatalf("content = %q", got)
	}
	if got := obj.Get("statusCode").ToInteger(); got != 200 {
		t.Fatalf("statusCode = %d", got)
	}
	if got := obj.Get("finalUrl").String(); got == "" {
		t.Fatal("finalUrl 为空")
	}
	if _, ok := obj.Get("headers").(*goja.Object); !ok {
		t.Fatal("headers 应为对象")
	}
}

func TestReqPostData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	e := New()
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q, {method:"POST",data:"a=1"}).content }`, srv.URL)); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil || v.String() != "ok" {
		t.Fatalf("req POST = %v, %v", v, err)
	}
}

// drpy2 规则大量使用 req('/category/data?...') 这种以站点根为基准的相对地址。
// 旧实现直接交给 net/http，报 `unsupported protocol scheme ""`。
// 全站搜索会按站点绑定 ctx 超时；req 必须带它跑，
// 否则连不上的站点会按自己的 15s 客户端超时把搜索线程占死。
func TestReqHonorsEngineContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	e := New()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	e.SetContext(ctx)
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q).content }`, srv.URL)); err != nil {
		t.Fatalf("加载脚本: %v", err)
	}
	start := time.Now()
	if _, err := e.Call("go"); err == nil {
		t.Fatal("慢服务应被引擎 ctx 超时截断并报错")
	}
	if elapsed := time.Since(start); elapsed > 800*time.Millisecond {
		t.Fatalf("req 应遵守引擎 ctx（60ms），实际耗时 %s", elapsed)
	}
}

func TestReqResolvesRelativeURLAgainstScriptBase(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dr/js/rule.js" {
			_, _ = w.Write([]byte(`function go(){ return req('/category/data?pageNo=1').content }`))
			return
		}
		gotPath = r.URL.Path
		if r.URL.Query().Get("pageNo") != "1" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("relative-ok"))
	}))
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/dr/js/rule.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatalf("req 相对地址应可用: %v", err)
	}
	if v.String() != "relative-ok" {
		t.Fatalf("content = %q", v.String())
	}
	if gotPath != "/category/data" {
		t.Fatalf("请求路径 = %q，期望 /category/data", gotPath)
	}
}

func TestDecodeBody(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		want        string
	}{
		{
			name:        "GBK",
			body:        []byte{0xd6, 0xd0, 0xce, 0xc4}, // 中文
			contentType: "text/html; charset=GBK",
			want:        "中文",
		},
		{
			name:        "UTF-8 fallback",
			body:        []byte("中文"),
			contentType: "application/json; charset=utf-8",
			want:        "中文",
		},
		{
			name:        "GBK without charset",
			body:        []byte{0xd6, 0xd0, 0xce, 0xc4},
			contentType: "",
			want:        "中文",
		},
		{
			name:        "GBK with content type but no charset",
			body:        []byte{0xd6, 0xd0, 0xce, 0xc4},
			contentType: "text/html",
			want:        "中文",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeBody(tt.body, tt.contentType); got != tt.want {
				t.Fatalf("decodeBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReqDecodesGBKResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=gb2312")
		_, _ = w.Write([]byte{0xd6, 0xd0, 0xce, 0xc4})
	}))
	defer srv.Close()

	e := New()
	if err := e.Load(fmt.Sprintf(`function go(){ return req(%q).content }`, srv.URL)); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil || v.String() != "中文" {
		t.Fatalf("req GBK content = %v, %v", v, err)
	}
}

func TestLoadFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`function loaded(){ return "ok" }`))
	}))
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("loaded")
	if err != nil || v.String() != "ok" {
		t.Fatalf("loaded() = %v, %v", v, err)
	}
}
