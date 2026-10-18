package tvbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/httpx"
)

// tvboxUA 是 FongMi多线路 客户端惯例 UA，部分 CMS 按它鉴权。
const tvboxUA = "okhttp/3.12.11"

// cmsVideo 是 CMS 接口返回的影片条目（列表与详情共用）。
type cmsVideo struct {
	VodID       flexID `json:"vod_id"`
	VodName     string `json:"vod_name"`
	VodPic      string `json:"vod_pic"`
	TypeID      flexID `json:"type_id"`
	TypeName    string `json:"type_name"`
	VodRemarks  string `json:"vod_remarks"`
	VodYear     string `json:"vod_year"`
	VodArea     string `json:"vod_area"`
	VodContent  string `json:"vod_content"`
	VodPlayFrom string `json:"vod_play_from"`
	VodPlayURL  string `json:"vod_play_url"`
}

type cmsClass struct {
	TypeID   flexID `json:"type_id"`
	TypeName string `json:"type_name"`
}

// flexID 兼容 CMS 源返回 id 的两种形态：数字（79337）与字符串（"79337"、" 79337 "）。
//
// 真实源里两者都常见，甚至同一响应混用。用 int64 硬解时，只要有一条是字符串，
// 整个响应的 json.Unmarshal 就会失败：
//
//	json: cannot unmarshal string into Go struct field cmsVideo.list.vod_id of type int64
//
// 同一批里其它正常条目会一起丢掉。非纯数字 id（如 "tv-9001"）也原样保留，
// 因为部分源确实用它做影片标识。
type flexID struct {
	raw string
}

// UnmarshalJSON 同时接受 JSON 数字与字符串；null / 缺失视为空。
func (f *flexID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		f.raw = ""
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		f.raw = strings.TrimSpace(str)
		return nil
	}
	f.raw = s
	return nil
}

// MarshalJSON 统一按字符串输出，避免数字/字符串混用导致前端类型抖动。
func (f flexID) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.raw)
}

// String 返回去空白后的原始 id 文本。
func (f flexID) String() string { return f.raw }

type cmsResp struct {
	Code      int             `json:"code"`
	List      []cmsVideo      `json:"list"`
	Class     []cmsClass      `json:"class"`
	Page      json.RawMessage `json:"page"`
	PageCount json.RawMessage `json:"pagecount"`
	Total     json.RawMessage `json:"total"`
}

func jsonInt(raw json.RawMessage) int {
	var n int
	if len(raw) > 0 && json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if len(raw) > 0 && json.Unmarshal(raw, &s) == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(s))
	}
	return n
}

// decodeCMSJSON 解析 CMS / drpy 的响应体。
//
// 源站失效时经常返回一个 HTML 页面（域名停放页、403 提示页、跳转中间页），
// 直接交给 json.Unmarshal 只会得到难以定位的
// `invalid character '<' looking for beginning of value`。
// 这里先识别网页再报错，并把开头片段带出来，方便判断是站点挂了还是 api 填错。
func decodeCMSJSON(raw []byte, dst any) error {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")))
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return fmt.Errorf("源站返回了网页而不是 JSON 接口（开头为 %q），该站点 api 可能已失效或被拦截", headSnippet(trimmed))
	}
	return json.Unmarshal(trimmed, dst)
}

// headSnippet 截取响应开头一段文本用于报错，避免把整页 HTML 塞进日志。
func headSnippet(b []byte) string {
	const max = 60
	if len(b) > max {
		b = b[:max]
	}
	return strings.TrimSpace(string(b))
}

// client 是单个 CMS 站点的 JSON API 客户端。
type client struct {
	base string
	hc   *http.Client
}

// newClient 用站点 api 基地址构造客户端；剥离 api 内可能内嵌的 query。
func newClient(api string) *client {
	base, _, _ := strings.Cut(api, "?")
	return &client{base: base, hc: httpx.NewClient(15 * time.Second)}
}

// videolist 请求列表/分类/搜索（t、wd 为可空过滤条件）。
func (c *client) videolist(ctx context.Context, t, wd string, page int) ([]cmsVideo, error) {
	resp, err := c.videolistPage(ctx, t, wd, page)
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

func (c *client) videolistPage(ctx context.Context, t, wd string, page int) (cmsResp, error) {
	q := url.Values{"ac": {"videolist"}, "pg": {fmt.Sprintf("%d", page)}}
	if t != "" {
		q.Set("t", t)
	}
	if wd != "" {
		q.Set("wd", wd)
	}
	raw, err := c.get(ctx, q)
	if err != nil {
		return cmsResp{}, err
	}
	var resp cmsResp
	if err := decodeCMSJSON(raw, &resp); err != nil {
		return cmsResp{}, fmt.Errorf("解析列表响应失败: %w", err)
	}
	if resp.Code != 1 {
		return cmsResp{}, fmt.Errorf("站点返回错误码 %d", resp.Code)
	}
	return resp, nil
}

func (c *client) categories(ctx context.Context) ([]cmsClass, error) {
	q := url.Values{"ac": {"list"}, "pg": {"1"}}
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp cmsResp
	if err := decodeCMSJSON(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析分类响应失败: %w", err)
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("站点返回错误码 %d", resp.Code)
	}
	return resp.Class, nil
}

// detail 请求单部影片详情。
func (c *client) detail(ctx context.Context, ids string) (*cmsVideo, error) {
	q := url.Values{"ac": {"detail"}, "ids": {ids}}
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp cmsResp
	if err := decodeCMSJSON(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析详情响应失败: %w", err)
	}
	if resp.Code != 1 || len(resp.List) == 0 {
		return nil, fmt.Errorf("站点返回错误码 %d 或无数据", resp.Code)
	}
	return &resp.List[0], nil
}

// get 发起带 UA 的 GET，返回响应体。
func (c *client) get(ctx context.Context, q url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", tvboxUA)
	resp, body, err := httpx.DoReadAll(c.hc, req, 0)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("站点返回 HTTP %d", resp.StatusCode)
	}
	return body, nil
}
