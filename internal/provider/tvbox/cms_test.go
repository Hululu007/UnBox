package tvbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientVideolist(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Query().Get("ac") != "videolist" {
			t.Errorf("ac = %q, 期望 videolist", r.URL.Query().Get("ac"))
		}
		w.Write([]byte(`{"code":1,"list":[{"vod_id":1001,"vod_name":"示例影片A","vod_pic":"http://x/p.jpg","type_id":16,"type_name":"示例分类"}]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	items, err := c.videolist(context.Background(), "16", "", 0)
	if err != nil {
		t.Fatalf("videolist 失败: %v", err)
	}
	if len(items) != 1 || items[0].VodName != "示例影片A" || items[0].TypeName != "示例分类" {
		t.Fatalf("解析错误: %+v", items)
	}
	if gotPath != "/" {
		t.Errorf("path = %q", gotPath)
	}
}

// 真实 CMS 源常见把 vod_id / type_id 返回成字符串（甚至带前后空格，
// 或非纯数字 id）；旧实现用 int64 硬解会让整个响应解析失败，报
// 「json: cannot unmarshal string into Go struct field cmsVideo.list.vod_id of type int64」，
// 连同一响应里其它正常条目也一起丢掉。
func TestClientVideolistTolerantIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":1,"list":[
			{"vod_id":"79337","vod_name":"字符串ID","type_id":"16","type_name":"分类A"},
			{"vod_id":79338,"vod_name":"数字ID","type_id":16,"type_name":"分类B"},
			{"vod_id":" 79339 ","vod_name":"带空格ID","type_id":"","type_name":"分类C"},
			{"vod_id":"tv-9001","vod_name":"非数字ID","type_id":"16","type_name":"分类D"}
		]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	items, err := c.videolist(context.Background(), "16", "", 1)
	if err != nil {
		t.Fatalf("字符串 id 不应导致整个响应解析失败: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("应解析出 4 条，得 %d", len(items))
	}
	if items[0].VodID.String() != "79337" {
		t.Errorf("字符串 id 应原样保留，得 %q", items[0].VodID.String())
	}
	if items[2].VodID.String() != "79339" {
		t.Errorf("带空格 id 应去除空白，得 %q", items[2].VodID.String())
	}
	if items[3].VodID.String() != "tv-9001" {
		t.Errorf("非数字 id 应原样保留，得 %q", items[3].VodID.String())
	}
}

// 源站失效时常返回 HTML（停放页/拦截页），旧实现只会冒出
// `invalid character '<' looking for beginning of value`，看不出真正原因。
func TestClientVideolistHTMLResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!DOCTYPE html><html><head><title>站点已停止维护</title></head></html>"))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	_, err := c.videolist(context.Background(), "", "", 1)
	if err == nil {
		t.Fatal("HTML 响应应报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "网页而不是 JSON") {
		t.Fatalf("错误应说明返回了网页，实际: %v", err)
	}
	if strings.Contains(msg, "invalid character") {
		t.Fatalf("不应再暴露 json 包的原始报错，实际: %v", err)
	}
}

func TestClientBaseStripsQuery(t *testing.T) {
	// 无水印采集站点的 api 内嵌了 ?ac=list，须剥离后再拼参数。
	c := newClient("https://api.example.com/api.php/provide/vod/?ac=list")
	if c.base != "https://api.example.com/api.php/provide/vod/" {
		t.Fatalf("base 未剥离 query: %q", c.base)
	}
}

func TestClientDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") != "detail" || r.URL.Query().Get("ids") != "1001" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"code":1,"list":[{"vod_id":1001,"vod_name":"示例影片A","vod_play_from":"线路A$$$线路B","vod_play_url":"第01集$a#第02集$b"}]}`))
	}))
	defer srv.Close()

	v, err := newClient(srv.URL).detail(context.Background(), "1001")
	if err != nil {
		t.Fatalf("detail 失败: %v", err)
	}
	if v.VodPlayFrom != "线路A$$$线路B" {
		t.Fatalf("detail 解析错误: %+v", v)
	}
}

func TestClientCategoriesUsesClassMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") != "list" {
			t.Errorf("ac = %q, want list", r.URL.Query().Get("ac"))
		}
		_, _ = w.Write([]byte(`{"code":1,"class":[{"type_id":1,"type_name":"国产剧"},{"type_id":2,"type_name":"韩国剧"}],"list":[{"vod_id":1,"vod_name":"当前页","type_id":1,"type_name":"国产剧"}]}`))
	}))
	defer srv.Close()

	classes, err := newClient(srv.URL).categories(context.Background())
	if err != nil {
		t.Fatalf("categories 失败: %v", err)
	}
	if len(classes) != 2 || classes[1].TypeName != "韩国剧" {
		t.Fatalf("分类解析错误: %+v", classes)
	}
}

// 筛选值必须作为 URL 编码的 JSON 查参 filter 发给站点（maccms 惯例）。
func TestVideolistFilteredEncodesFilterParam(t *testing.T) {
	var gotFilter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilter = r.URL.Query().Get("filter")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"list":[]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	if _, err := c.listFiltered(context.Background(), "", "", 1, `{"area":"内地","year":"2023"}`); err != nil {
		t.Fatal(err)
	}
	if gotFilter == "" {
		t.Fatal("filter 参数缺失")
	}
	decoded, err := url.QueryUnescape(gotFilter)
	if err != nil || decoded != `{"area":"内地","year":"2023"}` {
		t.Fatalf("filter 参数应为 URL 编码的 JSON，实际 %q（解码 %q）", gotFilter, decoded)
	}
}

// 空 filter（无筛选）时不应拼 filter 参数，保持与旧 videolist 一致。
func TestVideolistFilteredOmitsEmptyFilter(t *testing.T) {
	var hasFilter bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasFilter = r.URL.Query().Get("filter") != ""
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"list":[]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL)
	if _, err := c.listFiltered(context.Background(), "1", "", 1, ""); err != nil {
		t.Fatal(err)
	}
	if hasFilter {
		t.Fatal("空 filter 不应出现在查询串中")
	}
}
