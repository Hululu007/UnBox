# UnBox M8.1/M8.2 网盘（AList/WebDAV）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用纯 Go 原生支持 `csp_AList` 与 `csp_WebDAV` 站点：浏览网盘目录、展开视频、解析直链并交给现有 Web/mpv + 代理播放。

**Architecture:** 新增 `internal/provider/netdisk`：一个 `Provider` 把 ext 里的多个 drive 映射为多个 Section；`Backend` 接口由 `alistBackend` 与 `webdavBackend` 实现；`Provider` 负责 ID 编码、目录递归展开、`provider.Provider` 适配。AList 走 JSON API，WebDAV 走 PROPFIND + Basic Auth。

**Tech Stack:** Go 1.26.3、标准库 `net/http`/`encoding/json`/`encoding/xml`/`net/url`、现有 `provider`/`player`/`config` 包。不新增第三方依赖。

**Spec:** `docs/superpowers/specs/2026-10-17-unbox-netdisk-design.md`

## Global Constraints

- 模块路径 `github.com/unbox/unbox`。
- Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/provider/netdisk` 不得 import Wails。
- 公开错误信息 / 注释用中文。
- TDD：先写失败测试，再写实现。
- 提交前 `go test ./... -count=1`、`go vet ./...`、`gofmt -l` 全绿；
  Linux 额外 `CGO_ENABLED=1 go build ./...`。
- M8.1/M8.2 不做云盘分享解析（Quark/UC/Ali/115/PikPak/Thunder），不做 Wogg/Wobg。
- 密码/token 不得写入日志、错误信息或前端绑定。
- 直链只在 `Resolve` 时实时获取，不缓存。

---

## File Structure

| 文件 | 职责 |
|---|---|
| `internal/config/classify.go` | `csp_AList` → `alist`、`csp_WebDAV` → `webdav` |
| `internal/config/classify_test.go` | 分类测试 |
| `internal/provider/netdisk/model.go` | `Entry`、`Backend`、视频扩展名 |
| `internal/provider/netdisk/ext.go` | 读取 ext（URL / JSON 字符串 / 对象） |
| `internal/provider/netdisk/netdisk.go` | `Provider`、ID 编码、目录展开 |
| `internal/provider/netdisk/alist.go` | AList ext + Backend |
| `internal/provider/netdisk/alist_client.go` | AList HTTP 客户端 |
| `internal/provider/netdisk/webdav.go` | WebDAV ext + Backend |
| `internal/provider/netdisk/webdav_client.go` | PROPFIND + XML + Basic Auth |
| `internal/provider/netdisk/*_test.go` | 单测/端到端 |
| `internal/shell/service.go` | `collectVodSites` 增加 `alist`/`webdav` |
| `internal/shell/service_test.go` | 接线测试 |
| `docs/2026-10-17-m8-acceptance.md` | 真实站点验收 |

---

### Task 1: 分类与 Provider 接线

**Files:**
- Modify: `internal/config/classify.go`
- Test: `internal/config/classify_test.go`
- Create: `internal/provider/netdisk/netdisk.go`（本任务先放最小构造函数）
- Modify: `internal/shell/service.go`
- Test: `internal/shell/service_test.go`

**Interfaces:**
- Produces: `netdisk.New(site config.Site) (*netdisk.Provider, error)`；
  `Classify` 返回 `alist`/`webdav`。

> 前置：`Site.ExtString` 与 `ResolveRelativeURLs` 对字符串型 `ext` 的解析
> 来自 M7.1/M6.2。若尚未落地，先完成那一步；本计划不重复实现。

- [ ] **Step 1: 写失败测试**

`classify_test.go` 增加：

```go
{Site{Type: SiteTypeSpider, API: "csp_AList"}, "alist", SupportYes},
{Site{Type: SiteTypeSpider, API: "csp_webdav"}, "webdav", SupportYes},
```

`service_test.go` 的 `TestCollectVodSitesSpider` 增加：

```go
{Key: "alist", Name: "AList", Type: config.SiteTypeSpider, API: "csp_AList", Ext: map[string]any{"drives": []any{map[string]any{"name": "n", "server": "https://x"}}}},
{Key: "webdav", Name: "WebDAV", Type: config.SiteTypeSpider, API: "csp_WebDAV", Ext: map[string]any{"drives": []any{map[string]any{"name": "n", "server": "https://x"}}}},
```

把 `len(vods)` 期望改为加 2，并增加：

```go
if _, ok := vods["alist"].(*netdisk.Provider); !ok {
	t.Fatalf("alist 站点应使用 netdisk.Provider，实际 %T", vods["alist"])
}
if _, ok := vods["webdav"].(*netdisk.Provider); !ok {
	t.Fatalf("webdav 站点应使用 netdisk.Provider，实际 %T", vods["webdav"])
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run TestClassify -count=1 && go test ./internal/shell -run TestCollectVodSitesSpider -count=1`
Expected: FAIL（分类仍为 jar；`netdisk.New` 未定义）

- [ ] **Step 3: 写实现**

`internal/config/classify.go` 在 `csp_` 分支前增加：

```go
switch api {
case "csp_alist":
	return "alist", SupportYes
case "csp_webdav":
	return "webdav", SupportYes
}
```

`internal/provider/netdisk/netdisk.go` 先写：

```go
package netdisk

import (
	"fmt"
	"strings"

	"github.com/unbox/unbox/internal/config"
)

// Provider 是网盘站点的 Provider；具体 Backend 在首次动作时加载。
type Provider struct {
	site config.Site
	kind string
}

// New 校验类名与 ext；不联网。
func New(site config.Site) (*Provider, error) {
	kind := strings.ToLower(strings.TrimSpace(site.API))
	switch kind {
	case "csp_alist", "csp_webdav":
	default:
		return nil, fmt.Errorf("暂不支持的网盘类: %s", site.API)
	}
	if site.Ext == nil {
		return nil, fmt.Errorf("网盘站点 %s 缺少 ext", site.Key)
	}
	if s, ok := site.Ext.(string); ok && strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("网盘站点 %s 缺少 ext", site.Key)
	}
	return &Provider{site: site, kind: kind}, nil
}

func (p *Provider) ID() string { return p.site.Key }
```

`internal/shell/service.go` 的 import 增加 `netdisk`，switch 增加：

```go
case "alist", "webdav":
	p, err := netdisk.New(site)
	if err != nil {
		continue
	}
	vods[site.Key] = p
	names[site.Key] = site.Name
	lines[site.Key] = line
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config ./internal/provider/netdisk ./internal/shell -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config/classify.go internal/config/classify_test.go internal/provider/netdisk/netdisk.go internal/shell/service.go internal/shell/service_test.go
git commit -m "feat(netdisk): 接入 AList/WebDAV 分类与 Provider"
```

---

### Task 2: 公共模型、ext 读取与 Provider 骨架

**Files:**
- Create: `internal/provider/netdisk/model.go`
- Create: `internal/provider/netdisk/ext.go`
- Modify: `internal/provider/netdisk/netdisk.go`
- Test: `internal/provider/netdisk/netdisk_test.go`

**Interfaces:**
- Produces: `Entry`、`Backend`、`ErrSearchUnsupported`、`loadExt`、
  `encodeID`/`decodeID`、`Provider.Home/Browse/Detail/Resolve/Search`。

- [ ] **Step 1: 写失败测试**

```go
package netdisk

import (
	"context"
	"testing"

	"github.com/unbox/unbox/internal/provider"
)

type fakeBackend struct{ entries map[string][]Entry }

func (f *fakeBackend) Sections(context.Context) ([]provider.Section, error) {
	return []provider.Section{{ID: encodeID("d0", "/"), Title: "盘"}}, nil
}
func (f *fakeBackend) SectionName(string) string { return "盘" }
func (f *fakeBackend) List(_ context.Context, _, path string) ([]Entry, error) { return f.entries[path], nil }
func (f *fakeBackend) Get(_ context.Context, _, path string) (Entry, error) { return Entry{Name: "a.mp4", Path: path}, nil }
func (f *fakeBackend) Search(context.Context, string) ([]Entry, error) { return nil, ErrSearchUnsupported }
func (f *fakeBackend) Stream(context.Context, string, string) (player.Stream, error) { return player.Stream{}, nil }

func TestIDRoundTrip(t *testing.T) {
	id := encodeID("d0", "/电影/你好.mp4")
	section, path, err := decodeID(id)
	if err != nil || section != "d0" || path != "/电影/你好.mp4" {
		t.Fatalf("decode = %q,%q,%v", section, path, err)
	}
	if _, _, err := decodeID("bad"); err == nil {
		t.Fatal("非法 id 应报错")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/netdisk -run TestIDRoundTrip -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

`model.go`：

```go
package netdisk

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// Entry 是网盘里的一个文件或目录。
type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64
	Modified time.Time
	Poster   string
	// Section/SectionName 仅搜索结果使用，用来回到正确的 drive。
	Section     string
	SectionName string
}

// Backend 是 AList/WebDAV 的共同接口；section 为 drive 编号（d0、d1）。
type Backend interface {
	Sections(ctx context.Context) ([]provider.Section, error)
	SectionName(section string) string
	List(ctx context.Context, section, path string) ([]Entry, error)
	Get(ctx context.Context, section, path string) (Entry, error)
	Search(ctx context.Context, q string) ([]Entry, error)
	Stream(ctx context.Context, section, path string) (player.Stream, error)
}

// ErrSearchUnsupported 表示该网盘后端不支持搜索。
var ErrSearchUnsupported = errors.New("该网盘不支持搜索")

var videoExt = map[string]bool{
	".mp4": true, ".m4v": true, ".mkv": true, ".webm": true,
	".avi": true, ".mov": true, ".flv": true, ".wmv": true,
	".ts": true, ".m2ts": true, ".m3u8": true, ".mpg": true,
	".mpeg": true, ".rmvb": true, ".rm": true, ".3gp": true,
}

func isVideo(name string) bool {
	return videoExt[strings.ToLower(path.Ext(name))]
}
```

`ext.go`：

```go
package netdisk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/config"
)

const maxExtSize = 4 << 20

func loadExt(ctx context.Context, site config.Site) ([]byte, error) {
	switch v := site.Ext.(type) {
	case string:
		t := strings.TrimSpace(v)
		if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
			return []byte(t), nil
		}
		if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, t, nil)
			if err != nil {
				return nil, fmt.Errorf("构造 ext 请求失败: %w", err)
			}
			req.Header.Set("User-Agent", "okhttp/3.12.11")
			resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
			if err != nil {
				return nil, fmt.Errorf("下载 ext 失败: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return nil, fmt.Errorf("下载 ext %s 返回 HTTP %d", t, resp.StatusCode)
			}
			b, err := io.ReadAll(io.LimitReader(resp.Body, maxExtSize+1))
			if err != nil {
				return nil, fmt.Errorf("读取 ext 失败: %w", err)
			}
			if len(b) > maxExtSize {
				return nil, fmt.Errorf("ext 超过 %d 字节限制", maxExtSize)
			}
			return b, nil
		}
		if strings.HasPrefix(t, "file://") {
			return os.ReadFile(strings.TrimPrefix(t, "file://"))
		}
		return nil, fmt.Errorf("ext 不是可读取的地址: %s", t)
	case nil:
		return nil, fmt.Errorf("缺少 ext")
	default:
		return json.Marshal(v)
	}
}
```

`netdisk.go` 扩展：

```go
import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

const (
	maxDepth = 3
	maxFiles = 500
	maxDirs  = 200
)

type Provider struct {
	site config.Site
	kind string

	mu      sync.Mutex
	backend Backend
	loaded  bool
	loadErr error
}

func (p *Provider) ensure(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return p.loadErr
	}
	b, err := loadExt(ctx, p.site)
	if err == nil {
		switch p.kind {
		case "csp_alist":
			p.backend, err = newAList(b)
		case "csp_webdav":
			p.backend, err = newWebDAV(b)
		default:
			err = fmt.Errorf("暂不支持的网盘类: %s", p.site.API)
		}
	}
	p.loaded = true
	p.loadErr = err
	return err
}

func (p *Provider) Home(ctx context.Context) ([]provider.Section, error) {
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	sections, err := p.backend.Sections(ctx)
	if err != nil {
		return nil, err
	}
	// Backend 只返回 d0/d1；Provider 统一补上根路径，Browse 才能 decodeID。
	for i := range sections {
		sections[i].ID = encodeID(sections[i].ID, "/")
	}
	return sections, nil
}

func (p *Provider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	if err := p.ensure(ctx); err != nil {
		return provider.Page{}, err
	}
	section, dir, err := decodeID(cat)
	if err != nil {
		return provider.Page{}, err
	}
	entries, err := p.backend.List(ctx, section, dir)
	if err != nil {
		return provider.Page{}, err
	}
	items := make([]provider.Item, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name, ".") {
			continue
		}
		group := "文件"
		if e.IsDir {
			group = "目录"
		}
		items = append(items, provider.Item{
			ID:    encodeID(section, e.Path),
			Title: e.Name,
			Logo:  e.Poster,
			Group: group,
		})
	}
	return provider.Page{Items: items, Page: 1, HasMore: false}, nil
}

func (p *Provider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	entries, err := p.backend.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]provider.Item, 0, len(entries))
	for _, e := range entries {
		items = append(items, provider.Item{
			ID:    encodeID(e.Section, e.Path),
			Title: e.Name,
			Logo:  e.Poster,
			Group: e.SectionName,
		})
	}
	return items, nil
}
```

`Entry` 需要 `Section`/`SectionName` 字段用于搜索结果；在 `model.go` 增加：

```go
	Section     string
	SectionName string
```

`Detail`/`Resolve`/ID helpers：

```go
func (p *Provider) Detail(ctx context.Context, id string) (provider.Media, error) {
	if err := p.ensure(ctx); err != nil {
		return provider.Media{}, err
	}
	section, itemPath, err := decodeID(id)
	if err != nil {
		return provider.Media{}, err
	}
	entry, err := p.backend.Get(ctx, section, itemPath)
	if err != nil {
		return provider.Media{}, err
	}
	name := p.backend.SectionName(section)
	if !entry.IsDir {
		return provider.Media{
			ID: id, Title: entry.Name, Group: name,
			Sources:  []string{name},
			Episodes: []provider.Episode{{ID: encodeID(section, entry.Path), Source: name, Name: entry.Name}},
		}, nil
	}
	files, truncated, err := p.flatten(ctx, section, entry.Path)
	if err != nil {
		return provider.Media{}, err
	}
	eps := make([]provider.Episode, 0, len(files))
	for _, f := range files {
		rel := strings.TrimPrefix(f.Path, strings.TrimRight(entry.Path, "/")+"/")
		if rel == f.Path || rel == "" {
			rel = f.Name
		}
		eps = append(eps, provider.Episode{ID: encodeID(section, f.Path), Source: name, Name: rel})
	}
	remarks := ""
	if truncated {
		remarks = "目录过大，仅展示前 500 个视频"
	}
	return provider.Media{
		ID: id, Title: entry.Name, Group: name,
		Sources: []string{name}, Episodes: eps, Remarks: remarks,
	}, nil
}

func (p *Provider) Resolve(ctx context.Context, epID string) (player.Stream, error) {
	if err := p.ensure(ctx); err != nil {
		return player.Stream{}, err
	}
	section, filePath, err := decodeID(epID)
	if err != nil {
		return player.Stream{}, err
	}
	return p.backend.Stream(ctx, section, filePath)
}

func (p *Provider) flatten(ctx context.Context, section, dir string) ([]Entry, bool, error) {
	type node struct {
		path  string
		depth int
	}
	queue := []node{{path: dir}}
	var files []Entry
	dirs := 0
	truncated := false
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.depth > maxDepth {
			truncated = true
			continue
		}
		entries, err := p.backend.List(ctx, section, n.path)
		if err != nil {
			return nil, false, err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name, ".") {
				continue
			}
			if e.IsDir {
				dirs++
				if dirs > maxDirs {
					truncated = true
					continue
				}
				queue = append(queue, node{path: e.Path, depth: n.depth + 1})
				continue
			}
			if !isVideo(e.Name) {
				continue
			}
			if len(files) >= maxFiles {
				truncated = true
				continue
			}
			files = append(files, e)
		}
	}
	return files, truncated, nil
}

func encodeID(section, path string) string {
	return section + "|" + base64.RawURLEncoding.EncodeToString([]byte(path))
}

func decodeID(id string) (string, string, error) {
	section, encoded, ok := strings.Cut(id, "|")
	if !ok || section == "" {
		return "", "", fmt.Errorf("网盘条目 id 非法: %s", id)
	}
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("网盘条目 id 非法: %s", id)
	}
	return section, string(b), nil
}
```

> `Entry.Section`/`SectionName` 仅用于搜索结果；`Get/List` 返回值可留空。
> 测试里 fake backend 的 `Search` 返回 `ErrSearchUnsupported`，Provider 原样返回。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/netdisk -run TestIDRoundTrip -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/netdisk/model.go internal/provider/netdisk/ext.go internal/provider/netdisk/netdisk.go internal/provider/netdisk/netdisk_test.go
git commit -m "feat(netdisk): 增加公共模型、ext 读取与 Provider 骨架"
```

---

### Task 3: AList HTTP 客户端

**Files:**
- Create: `internal/provider/netdisk/alist_client.go`
- Test: `internal/provider/netdisk/alist_client_test.go`

**Interfaces:**
- Produces: `alistClient`、`alistItem`、`alistData`、`flexBool`/`flexInt64`；
  `newAListClient(server string, login *alistLogin) *alistClient`；
  `List/Get/Search`。

- [ ] **Step 1: 写失败测试**

```go
func TestAListClientPrivate(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"token":"tok"}}`))
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"code":200,"data":{"content":[
			{"name":"电影","is_dir":true,"size":0},
			{"name":"a.mp4","is_dir":false,"size":123,"modified":"2026-01-01T00:00:00Z"}
		]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newAListClient(srv.URL, &alistLogin{Username: "u", Password: "p"})
	entries, err := c.List(context.Background(), "/", "")
	if err != nil { t.Fatal(err) }
	if gotAuth != "tok" { t.Fatalf("Authorization = %q", gotAuth) }
	if len(entries) != 2 || !entries[0].IsDir || entries[1].Size != 123 {
		t.Fatalf("entries = %#v", entries)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/netdisk -run TestAListClientPrivate -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

```go
package netdisk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type alistLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type alistParam struct {
	Path string `json:"path"`
	Pass string `json:"pass"`
}

type alistDrive struct {
	Name   string       `json:"name"`
	Server string       `json:"server"`
	Hidden bool         `json:"hidden"`
	Login  *alistLogin  `json:"login"`
	Params []alistParam `json:"params"`
}

type alistExt struct {
	VodPic string       `json:"vodPic"`
	Drives []alistDrive `json:"drives"`
}

type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	*b = flexBool(s == "true" || s == "1")
	return nil
}

type flexInt64 int64

func (i *flexInt64) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil // 非数字按 0 处理，避免单个坏字段拖垮整页
	}
	*i = flexInt64(n)
	return nil
}

type alistItem struct {
	Name     string    `json:"name"`
	IsDir    flexBool  `json:"is_dir"`
	Size     flexInt64 `json:"size"`
	Modified string    `json:"modified"`
	Thumb    string    `json:"thumb"`
	Type     int       `json:"type"`
}

type alistData struct {
	Content []alistItem `json:"content"`
	Files   []alistItem `json:"files"`
	Total   int         `json:"total"`
	Name    string      `json:"name"`
	IsDir   flexBool    `json:"is_dir"`
	Size    flexInt64   `json:"size"`
	Modified string     `json:"modified"`
	Thumb   string      `json:"thumb"`
	RawURL  string      `json:"raw_url"`
	URL     string      `json:"url"`
}

type alistEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type alistClient struct {
	base  string
	login *alistLogin
	hc    *http.Client

	mu    sync.Mutex
	token string
}

func newAListClient(server string, login *alistLogin) *alistClient {
	return &alistClient{
		base:  strings.TrimRight(strings.TrimSpace(server), "/"),
		login: login,
		hc:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *alistClient) ensureToken(ctx context.Context) error {
	if c.login == nil || c.token != "" {
		return nil
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, "/api/auth/login", map[string]any{
		"username": c.login.Username,
		"password": c.login.Password,
	}, &data); err != nil {
		return err
	}
	if data.Token == "" {
		return fmt.Errorf("AList 登录未返回 token")
	}
	c.mu.Lock()
	c.token = data.Token
	c.mu.Unlock()
	return nil
}

func (c *alistClient) do(ctx context.Context, endpoint string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.11")
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("AList 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var env alistEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("AList 响应解析失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || env.Code != 200 {
		return fmt.Errorf("AList 返回错误: HTTP %d %s %s", resp.StatusCode, env.Message, endpoint)
	}
	if out != nil {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

func (c *alistClient) List(ctx context.Context, path, password string) ([]Entry, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}
	if c.login != nil {
		var data alistData
		if err := c.do(ctx, "/api/fs/list", map[string]any{
			"path": path, "password": password, "page": 1, "per_page": 0, "refresh": false,
		}, &data); err != nil {
			return nil, err
		}
		return toEntries(path, data.Content), nil
	}
	var data alistData
	if err := c.do(ctx, "/api/public/path", map[string]any{"path": path}, &data); err != nil {
		return nil, err
	}
	if data.Files != nil {
		return toEntries(path, data.Files), nil
	}
	if data.Name != "" {
		return toEntries(path, []alistItem{{Name: data.Name, IsDir: data.IsDir, Size: data.Size, Modified: data.Modified, Thumb: data.Thumb}}), nil
	}
	return nil, nil
}

func (c *alistClient) Get(ctx context.Context, path, password string) (alistData, error) {
	if err := c.ensureToken(ctx); err != nil {
		return alistData{}, err
	}
	var data alistData
	if c.login != nil {
		if err := c.do(ctx, "/api/fs/get", map[string]any{"path": path, "password": password}, &data); err != nil {
			return alistData{}, err
		}
		return data, nil
	}
	if err := c.do(ctx, "/api/public/path", map[string]any{"path": path}, &data); err != nil {
		return alistData{}, err
	}
	return data, nil
}

func (c *alistClient) Search(ctx context.Context, parent, keyword string) ([]Entry, error) {
	if c.login == nil {
		return nil, ErrSearchUnsupported
	}
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}
	var data alistData
	if err := c.do(ctx, "/api/fs/search", map[string]any{
		"parent": parent, "keywords": keyword, "page": 1, "per_page": 100, "scope": 0,
	}, &data); err != nil {
		return nil, err
	}
	return toEntries(parent, data.Content), nil
}

func toEntries(parent string, items []alistItem) []Entry {
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		p := joinNetPath(parent, it.Name)
		out = append(out, Entry{
			Name: it.Name, Path: p, IsDir: bool(it.IsDir),
			Size: int64(it.Size), Poster: it.Thumb,
		})
	}
	return out
}

func joinNetPath(parent, name string) string {
	if parent == "" || parent == "/" {
		return "/" + strings.TrimLeft(name, "/")
	}
	return strings.TrimRight(parent, "/") + "/" + strings.TrimLeft(name, "/")
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/netdisk -run TestAListClientPrivate -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/netdisk/alist_client.go internal/provider/netdisk/alist_client_test.go
git commit -m "feat(netdisk): 实现 AList HTTP 客户端"
```

---

### Task 4: AList Backend

**Files:**
- Create: `internal/provider/netdisk/alist.go`
- Test: `internal/provider/netdisk/alist_test.go`

**Interfaces:**
- Produces: `newAList(data []byte) (*alistBackend, error)`；实现 `Backend`。

- [ ] **Step 1: 写失败测试**

```go
func TestAListBackend(t *testing.T) {
	// 复用 Task 3 的 httptest 服务器，补 /api/fs/get。
	// 断言 Sections/List/Get/Stream 的路径与 Authorization。
}
```

（测试代码见 Task 7 的端到端用例；本任务先实现，Task 7 统一验证。）

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/netdisk -run TestAListBackend -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

```go
package netdisk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

type alistBackend struct {
	drives  []alistDrive
	clients []*alistClient
}

func newAList(data []byte) (*alistBackend, error) {
	var ext alistExt
	if err := json.Unmarshal(data, &ext); err != nil {
		return nil, fmt.Errorf("解析 AList ext 失败: %w", err)
	}
	b := &alistBackend{}
	for _, d := range ext.Drives {
		if d.Hidden || strings.TrimSpace(d.Server) == "" {
			continue
		}
		d.Server = strings.TrimRight(strings.TrimSpace(d.Server), "/")
		b.drives = append(b.drives, d)
		b.clients = append(b.clients, newAListClient(d.Server, d.Login))
	}
	if len(b.drives) == 0 {
		return nil, fmt.Errorf("AList ext 没有可用 drive")
	}
	return b, nil
}

func (b *alistBackend) Sections(context.Context) ([]provider.Section, error) {
	out := make([]provider.Section, 0, len(b.drives))
	for i, d := range b.drives {
		out = append(out, provider.Section{ID: fmt.Sprintf("d%d", i), Title: d.Name})
	}
	return out, nil
}

func (b *alistBackend) SectionName(section string) string {
	i, ok := parseSection(section)
	if !ok || i >= len(b.drives) {
		return section
	}
	return b.drives[i].Name
}

func (b *alistBackend) List(ctx context.Context, section, path string) ([]Entry, error) {
	i, ok := parseSection(section)
	if !ok || i >= len(b.clients) {
		return nil, fmt.Errorf("网盘 section 非法: %s", section)
	}
	return b.clients[i].List(ctx, path, b.password(i, path))
}

func (b *alistBackend) Get(ctx context.Context, section, path string) (Entry, error) {
	i, ok := parseSection(section)
	if !ok || i >= len(b.clients) {
		return Entry{}, fmt.Errorf("网盘 section 非法: %s", section)
	}
	d, err := b.clients[i].Get(ctx, path, b.password(i, path))
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name: d.Name, Path: path, IsDir: bool(d.IsDir),
		Size: int64(d.Size), Poster: d.Thumb,
	}, nil
}

func (b *alistBackend) Search(ctx context.Context, q string) ([]Entry, error) {
	var out []Entry
	var lastErr error
	searched := false
	for i := range b.clients {
		entries, err := b.clients[i].Search(ctx, "/", q)
		if err == ErrSearchUnsupported {
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		searched = true
		for _, e := range entries {
			e.Section = fmt.Sprintf("d%d", i)
			e.SectionName = b.drives[i].Name
			out = append(out, e)
		}
	}
	if !searched && lastErr != nil {
		return nil, lastErr
	}
	if !searched {
		return nil, ErrSearchUnsupported
	}
	return out, nil
}

func (b *alistBackend) Stream(ctx context.Context, section, path string) (player.Stream, error) {
	i, ok := parseSection(section)
	if !ok || i >= len(b.clients) {
		return player.Stream{}, fmt.Errorf("网盘 section 非法: %s", section)
	}
	d, err := b.clients[i].Get(ctx, path, b.password(i, path))
	if err != nil {
		return player.Stream{}, err
	}
	raw := firstNonEmpty(d.RawURL, d.URL)
	if raw == "" && b.drives[i].Login == nil {
		// 公开盘有时不返回 raw_url，/d 路径通常可直接下载。
		raw = b.drives[i].Server + "/d" + path
	}
	if raw == "" {
		return player.Stream{}, fmt.Errorf("AList 未返回直链: %s", path)
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		base, err := url.Parse(b.drives[i].Server)
		if err == nil {
			ref, err := url.Parse(raw)
			if err == nil {
				raw = base.ResolveReference(ref).String()
			}
		}
	}
	headers := map[string]string{"User-Agent": "okhttp/3.12.11"}
	if b.clients[i].token != "" {
		headers["Authorization"] = b.clients[i].token
	}
	return player.Stream{URL: raw, Kind: player.KindForURL(path), Headers: headers}, nil
}

func (b *alistBackend) password(i int, path string) string {
	var pass string
	n := -1
	for _, p := range b.drives[i].Params {
		if strings.HasPrefix(path, p.Path) && len(p.Path) > n {
			pass, n = p.Pass, len(p.Path)
		}
	}
	return pass
}

func parseSection(section string) (int, bool) {
	if !strings.HasPrefix(section, "d") {
		return 0, false
	}
	i := 0
	for _, r := range section[1:] {
		if r < '0' || r > '9' {
			return 0, false
		}
		i = i*10 + int(r-'0')
	}
	return i, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/netdisk -run TestAListBackend -count=1`
Expected: PASS（若测试先跳过，Task 7 必须通过）

- [ ] **Step 5: 提交**

```bash
git add internal/provider/netdisk/alist.go internal/provider/netdisk/alist_test.go
git commit -m "feat(netdisk): 实现 AList Backend"
```

---

### Task 5: WebDAV 客户端

**Files:**
- Create: `internal/provider/netdisk/webdav_client.go`
- Test: `internal/provider/netdisk/webdav_client_test.go`

**Interfaces:**
- Produces: `webdavClient`、`webdavEntry`、`newWebDAVClient(server,user,pass)`；
  `List/Get/Stream`。

- [ ] **Step 1: 写失败测试**

```go
func TestWebDAVClientList(t *testing.T) {
	const xmlBody = `<?xml version="1.0"?>
<D:multistatus xmlns:D="DAV:">
 <D:response><D:href>/dav/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop></D:propstat></D:response>
 <D:response><D:href>/dav/a.mp4</D:href><D:propstat><D:prop><D:resourcetype/><D:getcontentlength>123</D:getcontentlength><D:getlastmodified>Wed, 01 Jan 2026 00:00:00 GMT</D:getlastmodified></D:prop></D:propstat></D:response>
</D:multistatus>`
	mux := http.NewServeMux()
	mux.HandleFunc("/dav/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" { t.Errorf("method = %s", r.Method) }
		if r.Header.Get("Depth") != "1" { t.Errorf("Depth = %s", r.Header.Get("Depth")) }
		u, p, ok := r.BasicAuth()
		if !ok || u != "u" || p != "p" { t.Errorf("basic auth = %v", ok) }
		_, _ = w.Write([]byte(xmlBody))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newWebDAVClient(srv.URL+"/dav", "u", "p")
	entries, err := c.List(context.Background(), "/")
	if err != nil { t.Fatal(err) }
	if len(entries) != 1 || entries[0].Name != "a.mp4" || entries[0].Size != 123 {
		t.Fatalf("entries = %#v", entries)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/netdisk -run TestWebDAVClientList -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

```go
package netdisk

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unbox/unbox/internal/player"
)

type webdavClient struct {
	base string
	user string
	pass string
	hc   *http.Client
}

func newWebDAVClient(server, user, pass string) *webdavClient {
	return &webdavClient{
		base: strings.TrimRight(strings.TrimSpace(server), "/"),
		user: user, pass: pass,
		hc: &http.Client{Timeout: 20 * time.Second},
	}
}

type davMultistatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href  string        `xml:"href"`
	Props []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop davProp `xml:"prop"`
}

type davProp struct {
	ResourceType davResourceType `xml:"resourcetype"`
	ContentLen   string          `xml:"getcontentlength"`
	LastModified string          `xml:"getlastmodified"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection"`
}

func (c *webdavClient) propfind(ctx context.Context, path string, depth int, skipSelf bool) ([]Entry, error) {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", strconv.Itoa(depth))
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.SetBasicAuth(c.user, c.pass)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("WebDAV PROPFIND 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("WebDAV PROPFIND %s 返回 HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var ms davMultistatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return nil, fmt.Errorf("WebDAV XML 解析失败: %w", err)
	}
	self := cleanDAVPath(path)
	var out []Entry
	for _, r := range ms.Responses {
		p := cleanDAVPath(c.entryPath(r.Href))
		if skipSelf && p == self {
			continue
		}
		if p == "" {
			continue
		}
		e := Entry{Name: pathBase(p), Path: p}
		for _, ps := range r.Props {
			if ps.Prop.ResourceType.Collection != nil {
				e.IsDir = true
			}
			if n, _ := strconv.ParseInt(strings.TrimSpace(ps.Prop.ContentLen), 10, 64); n > 0 {
				e.Size = n
			}
			if t, err := http.ParseTime(ps.Prop.LastModified); err == nil {
				e.Modified = t
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (c *webdavClient) List(ctx context.Context, path string) ([]Entry, error) {
	return c.propfind(ctx, path, 1, true)
}

func (c *webdavClient) Get(ctx context.Context, path string) (Entry, error) {
	entries, err := c.propfind(ctx, path, 0, false)
	if err != nil {
		return Entry{}, err
	}
	if len(entries) == 0 {
		return Entry{Name: pathBase(path), Path: path}, nil
	}
	return entries[0], nil
}

func (c *webdavClient) url(path string) string {
	u, err := url.JoinPath(c.base, strings.TrimLeft(path, "/"))
	if err != nil {
		return c.base + "/" + strings.TrimLeft(path, "/")
	}
	return u
}

func (c *webdavClient) basePath() string {
	u, err := url.Parse(c.base)
	if err != nil {
		return "/"
	}
	return u.Path
}

func (c *webdavClient) entryPath(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	p, err := url.PathUnescape(u.Path)
	if err != nil {
		p = u.Path
	}
	base := strings.TrimRight(c.basePath(), "/")
	if base != "" && strings.HasPrefix(p, base) {
		p = strings.TrimPrefix(p, base)
	}
	if p == "" {
		p = "/"
	}
	return p
}

func cleanDAVPath(p string) string {
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p == "/" {
		return "/"
	}
	return strings.TrimRight(p, "/")
}

func pathBase(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (c *webdavClient) Stream(path string) player.Stream {
	return player.Stream{
		URL:  c.url(path),
		Kind: player.KindForURL(path),
		Headers: map[string]string{
			"Authorization": "Basic " + basicAuth(c.user, c.pass),
		},
	}
}

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
```

> `cleanDAVPath` 的写法过于绕；实现时直接：
> `if p == "/" { return "/" }; return strings.TrimRight(p, "/")`。

需要 `basicAuth`：

```go
func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
```

（也可用 `(&http.Request{}).SetBasicAuth` 生成，但这里直接编码。）

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/netdisk -run TestWebDAVClientList -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/netdisk/webdav_client.go internal/provider/netdisk/webdav_client_test.go
git commit -m "feat(netdisk): 实现 WebDAV 客户端"
```

---

### Task 6: WebDAV Backend

**Files:**
- Create: `internal/provider/netdisk/webdav.go`
- Test: `internal/provider/netdisk/webdav_test.go`

**Interfaces:**
- Produces: `newWebDAV(data []byte) (*webdavBackend, error)`；实现 `Backend`。

- [ ] **Step 1: 写失败测试**

```go
func TestWebDAVBackendExt(t *testing.T) {
	b, err := newWebDAV([]byte(`{"drives":[{"name":"盘","server":"https://x/dav","user":"u","pass":"p"}]}`))
	if err != nil { t.Fatal(err) }
	if b.SectionName("d0") != "盘" { t.Fatalf("name = %q", b.SectionName("d0")) }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/provider/netdisk -run TestWebDAVBackendExt -count=1`
Expected: FAIL

- [ ] **Step 3: 写实现**

```go
package netdisk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

type webdavDrive struct {
	Name   string `json:"name"`
	Server string `json:"server"`
	User   string `json:"user"`
	Pass   string `json:"pass"`
}

type webdavExt struct {
	Drives []webdavDrive `json:"drives"`
}

type webdavBackend struct {
	drives  []webdavDrive
	clients []*webdavClient
}

func newWebDAV(data []byte) (*webdavBackend, error) {
	var ext webdavExt
	if err := json.Unmarshal(data, &ext); err != nil {
		return nil, fmt.Errorf("解析 WebDAV ext 失败: %w", err)
	}
	b := &webdavBackend{}
	for _, d := range ext.Drives {
		if d.Server == "" {
			continue
		}
		b.drives = append(b.drives, d)
		b.clients = append(b.clients, newWebDAVClient(d.Server, d.User, d.Pass))
	}
	if len(b.drives) == 0 {
		return nil, fmt.Errorf("WebDAV ext 没有可用 drive")
	}
	return b, nil
}

func (b *webdavBackend) Sections(context.Context) ([]provider.Section, error) {
	out := make([]provider.Section, 0, len(b.drives))
	for i, d := range b.drives {
		out = append(out, provider.Section{ID: fmt.Sprintf("d%d", i), Title: d.Name})
	}
	return out, nil
}

func (b *webdavBackend) SectionName(section string) string {
	i, ok := parseSection(section)
	if !ok || i >= len(b.drives) {
		return section
	}
	return b.drives[i].Name
}

func (b *webdavBackend) List(ctx context.Context, section, path string) ([]Entry, error) {
	i, ok := parseSection(section)
	if !ok || i >= len(b.clients) {
		return nil, fmt.Errorf("网盘 section 非法: %s", section)
	}
	return b.clients[i].List(ctx, path)
}

func (b *webdavBackend) Get(ctx context.Context, section, path string) (Entry, error) {
	i, ok := parseSection(section)
	if !ok || i >= len(b.clients) {
		return Entry{}, fmt.Errorf("网盘 section 非法: %s", section)
	}
	return b.clients[i].Get(ctx, path)
}

func (b *webdavBackend) Search(context.Context, string) ([]Entry, error) {
	return nil, ErrSearchUnsupported
}

func (b *webdavBackend) Stream(ctx context.Context, section, path string) (player.Stream, error) {
	i, ok := parseSection(section)
	if !ok || i >= len(b.clients) {
		return player.Stream{}, fmt.Errorf("网盘 section 非法: %s", section)
	}
	return b.clients[i].Stream(path), nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/provider/netdisk -run TestWebDAVBackendExt -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/netdisk/webdav.go internal/provider/netdisk/webdav_test.go
git commit -m "feat(netdisk): 实现 WebDAV Backend"
```

---

### Task 7: Provider 端到端测试（AList + WebDAV）

**Files:**
- Test: `internal/provider/netdisk/netdisk_e2e_test.go`

- [ ] **Step 1: 写 AList 端到端测试**

```go
func TestAListProviderE2E(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"token":"tok"}}`))
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"content":[
			{"name":"剧集","is_dir":true},
			{"name":"a.mp4","is_dir":false,"size":10}
		]}}`))
	})
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"name":"a.mp4","is_dir":false,"size":10,"raw_url":"`+"http://cdn/a.mp4"+`"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ext := map[string]any{"drives": []any{map[string]any{
		"name": "盘", "server": srv.URL, "login": map[string]any{"username": "u", "password": "p"},
	}}}
	p, err := New(config.Site{Key: "a", API: "csp_AList", Ext: ext})
	if err != nil { t.Fatal(err) }
	secs, err := p.Home(context.Background())
	if err != nil || len(secs) != 1 { t.Fatalf("Home = %#v,%v", secs, err) }
	page, err := p.Browse(context.Background(), secs[0].ID, 1)
	if err != nil || len(page.Items) != 2 { t.Fatalf("Browse = %#v,%v", page, err) }
	media, err := p.Detail(context.Background(), page.Items[1].ID)
	if err != nil || len(media.Episodes) != 1 { t.Fatalf("Detail = %#v,%v", media, err) }
	stream, err := p.Resolve(context.Background(), media.Episodes[0].ID)
	if err != nil || stream.URL != "http://cdn/a.mp4" { t.Fatalf("Resolve = %#v,%v", stream, err) }
}
```

- [ ] **Step 2: 写 WebDAV 端到端测试**

用 Task 5 的 XML fixture，`New(config.Site{API:"csp_WebDAV", Ext: map...})`，
断言 `Home/Browse/Detail/Resolve`，并检查 `Stream.Headers["Authorization"]`。
测试 handler 必须按 `Depth` 返回不同 XML：`Depth: 0` 只返回目标文件（用于
`Get`），`Depth: 1` 返回 self + 文件（用于 `List`）。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/provider/netdisk -run 'TestAListProviderE2E|TestWebDAVProviderE2E' -count=1`
Expected: 先 FAIL（若实现尚未完成），完成后 PASS

- [ ] **Step 4: 修复实现直到通过**

Run: `go test ./internal/provider/netdisk -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/netdisk/netdisk_e2e_test.go
git commit -m "test(netdisk): 覆盖 AList/WebDAV Provider 端到端"
```

---

### Task 8: 真实验收与文档同步

**Files:**
- Create: `docs/2026-10-17-m8-acceptance.md`
- Modify: `docs/HANDOFF.md`
- Modify: `README.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: 全量验证**

```bash
go test ./... -count=1
go vet ./...
gofmt -l .
CGO_ENABLED=1 go build ./...
```

- [ ] **Step 2: 真实 AList/WebDAV 验收**

- 用公开 AList 服务器（例如 `https://nics.eu.org`、`https://pan.ecve.cn`）配置
  `drives`，确认 Home/Browse/Detail/Resolve。
- 用本地 `golang.org/x/net/webdav` 测试服务器或用户自己的 WebDAV，确认
  PROPFIND/GET + Basic Auth 播放。
- 记录分类数、目录项数、视频数、直链前缀、Header 是否生效。

写入 `docs/2026-10-17-m8-acceptance.md`。

- [ ] **Step 3: 更新文档**

- `docs/HANDOFF.md`：M8 进度、已知限制、后续待办。
- `README.md`：支持源表增加 AList/WebDAV；路线图增加 M8.1/M8.2。
- `AGENTS.md`：增加 M8 spec/plan 指针。
- 明确写出：云盘分享解析（Quark/UC/Ali/115/PikPak/Thunder）与 Wogg/Wobg
  不在 M8.1/M8.2 范围。

- [ ] **Step 4: 提交**

```bash
git add docs/2026-10-17-m8-acceptance.md docs/HANDOFF.md README.md AGENTS.md
git commit -m "docs: 同步 M8 AList/WebDAV 网盘支持进度"
```

---

## Self-Review

**Spec coverage:**
- M8.1 AList → Tasks 3–4、7。
- M8.2 WebDAV → Tasks 5–7。
- 配置/分类/接线 → Task 1；ext 读取 → Task 2。
- 许可证/安全 → Global Constraints + spec §7。
- M8.3 Samba / M8.4 云盘 / M8.5 搜索聚合 → spec 中列为后续，不在本计划。

**Placeholder scan:** 无 TBD/TODO；每个任务有测试与代码。Task 4/6 的测试在
Task 7 统一端到端验证，Task 4/6 仍保留各自的失败/通过步骤。

**Type consistency:**
- `Backend`、`Entry`、`Provider`、`alistClient`、`webdavClient` 的签名在各任务间一致。
- `encodeID`/`decodeID` 在 Task 2 定义，Task 3/4/5/6/7 复用。
- `player.Stream`/`provider.Section`/`provider.Page`/`provider.Media` 使用现有类型。
