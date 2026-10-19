# 点播筛选 UI 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在点播页提供「筛选」抽屉/弹层，让有筛选项的分类（drpy2 规则站 + CMS）可以按维度选择并重拉列表；只有存在筛选项时才显示筛选按钮。

**Architecture:** 统一筛选模型 `provider.FilterGroup` 挂在 `Section.Filters` 上；`Provider` 新增 `BrowseFiltered(ctx, cat, page, filters)`，`Browse` 保留并委托空参，接口零破坏。drpy2 规则站从 `rule.filter_def` 组装筛选项、选择值进 `categoryContent` 的 `extend` 与 `{{fl.xxx}}` 模板；CMS 从 `ac=list` 的每类 `filter`/`filters` 解析、选择值拼 `filter=<URL编码 JSON>` 查询参数。前端新增 `vodFilters.ts` 纯逻辑状态机 + 筛选弹层组件，只有分类有非空筛选项时按钮才 `v-if` 渲染。

**Tech Stack:** Go 1.26.3 / Wails v3 beta.26 / Vue3 + TypeScript（vitest）。

**Spec:** `docs/superpowers/specs/2026-10-19-unbox-vod-filter-ui-design.md`

## Global Constraints

- Go 模块路径 `github.com/unbox/unbox`；Wails 代码只允许在 `internal/shell/`、`cmd/unbox/`、`frontend/`；`internal/crawler` 与 `internal/provider` 不得 import Wails。
- `internal/crawler` 不得 import `internal/provider`（依赖方向：crawler ← provider ← shell）。筛选组组装器必须放 `internal/provider/tvbox`。
- 常规提交前绿：`go test ./... -count=1`、`go vet ./...`、`gofmt -l`；前端 `npm test`（vitest）+ `npx vue-tsc --noEmit`。
- 绑定生成：`wails3 generate bindings` **必须带 `-ts`**（beta.26 默认只出 `.js`）。改完 Go 绑定相关代码后跑（`build/Taskfile.yml` 的 `generate:bindings` 已带 `-ts -i`，用 `wails3 task generate:bindings`）。
- 不改 `Provider.Browse` 的既有签名；所有现有 `Browse` 行为保持不变（回归全绿）。
- 公开错误信息/注释用中文。TDD：改代码先写失败测试。
- 不做：筛选值持久化、URL 记忆、多选/区间、drpy http 服务（`Drpy`）筛选、CMS POST 探测。

---

### Task 1: provider 筛选模型与 `BrowseFiltered` 接口

**Files:**
- Create: `internal/provider/filters.go`
- Modify: `internal/provider/provider.go`（Section 增字段、Provider 增方法）

**Interfaces:**
- Produces:
  - `type FilterOption struct { Name string; Value string }`
  - `type FilterGroup struct { Key string; Name string; Options []FilterOption }`
  - `Section.Filters []FilterGroup`
  - `BrowseFiltered(ctx context.Context, cat string, page int, filters map[string]string) (Page, error)`

- [ ] **Step 1: 写失败测试**（`internal/provider/filters_test.go`，验证 Filters 字段与 BrowseFiltered 出现在接口上——用 live 假实现编译期断言即可）

```go
package provider

import (
	"context"
	"testing"
)

// 编译期接口完整性：实现方必须提供 BrowseFiltered，Browse 行为保持。
type stubProvider struct{}
func (stubProvider) ID() string                                   { return "" }
func (stubProvider) Home(context.Context) ([]Section, error)      { return nil, nil }
func (stubProvider) Browse(ctx context.Context, cat string, page int) (Page, error) {
	return (stubProvider{}).BrowseFiltered(ctx, cat, page, nil)
}
func (stubProvider) BrowseFiltered(ctx context.Context, cat string, page int, _ map[string]string) (Page, error) {
	return Page{}, nil
}
func (stubProvider) Search(context.Context, string) ([]Item, error)    { return nil, nil }
func (stubProvider) Detail(context.Context, string) (Media, error)     { return Media{}, nil }
func (stubProvider) Resolve(context.Context, string) (player.Stream, error) {
	return player.Stream{}, nil
}

func TestSectionCarriesFilters(t *testing.T) {
	s := Section{ID: "1", Title: "电影", Filters: []FilterGroup{{
		Key: "area", Name: "地区",
		Options: []FilterOption{{Name: "全部", Value: ""}, {Name: "内地", Value: "内地"}},
	}}}
	if len(s.Filters) != 1 || s.Filters[0].Key != "area" {
		t.Fatalf("Section 未携带过滤器: %+v", s)
	}
}

var _ Provider = stubProvider{}
```

- [ ] **Step 2: 验证红**：`go build ./internal/provider/` → 编译失败（BrowseFiltered 未定义）

- [ ] **Step 3: 实现**

`internal/provider/filters.go`：

```go
package provider

// FilterOption 是单个可选项（如「内地」），Name 展示、Value 提交给站点。
type FilterOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FilterGroup 是一个筛选维度（地区 / 年份 / 语言…）。
type FilterGroup struct {
	Key     string         `json:"key"`
	Name    string         `json:"name"`
	Options []FilterOption `json:"options"`
}
```

`provider.go` 的 `Section` 增加字段、`Provider` 增加方法（`Browse` 签名不动）：

```go
type Section struct {
	ID      string
	Title   string
	Filters []FilterGroup // 该分类的筛选定义；无筛选时为空
}

type Provider interface {
	ID() string
	Home(ctx context.Context) ([]Section, error)
	Browse(ctx context.Context, cat string, page int) (Page, error)
	// BrowseFiltered 带筛选值浏览分类；filters 为「维度 key → 选中 value」，
	// 未覆盖的维度由实现方填站点默认值；filters 为 nil 等价 Browse。
	BrowseFiltered(ctx context.Context, cat string, page int, filters map[string]string) (Page, error)
	Search(ctx context.Context, q string) ([]Item, error)
	Detail(ctx context.Context, id string) (Media, error)
	Resolve(ctx context.Context, id string) (player.Stream, error)
}
```

- [ ] **Step 4: 验证绿**：`go test ./internal/provider/ -count=1 && go vet ./internal/provider/ && gofmt -l internal/provider/`

- [ ] **Step 5: 提交**

```bash
git add internal/provider/filters.go internal/provider/filters_test.go internal/provider/provider.go
git commit -m "feat(provider): 统一筛选模型 FilterGroup + BrowseFiltered 接口"
```

---

### Task 2: 各 Provider 实现 `BrowseFiltered`（live / CMS / Drpy / Spider 先走空参）

**Files:**
- Modify: `internal/provider/live/live.go`
- Modify: `internal/provider/tvbox/tvbox.go`
- Modify: `internal/provider/tvbox/drpy.go`
- Modify: `internal/provider/tvbox/spider.go`

**Interfaces:**
- Consumes: Task 1 的 `Provider.BrowseFiltered` 签名。
- Produces: 全部 Provider 满足接口编译；`Browse` 均委托 `BrowseFiltered(nil)`。

- [ ] **Step 1: 写失败测试**（编译期即可，见 Task 1 的 `var _ Provider = stubProvider{}` 模式；本任务内每个 Provider 包加断言）

`internal/provider/live/live_test.go` 增：

```go
var _ provider.Provider = (*Provider)(nil)
```

`internal/provider/tvbox/provider_test.go`（新建，若不存在）增：

```go
var _ provider.Provider = (*Provider)(nil)
var _ provider.Provider = (*Spider)(nil)
var _ provider.Provider = (*Drpy)(nil)
```

- [ ] **Step 2: 验证红**：`go build ./...` → 编译失败（缺 BrowseFiltered 实现）

- [ ] **Step 3: 实现**

把已有的分类逻辑平移到 `BrowseFiltered`（filters 为空参 `nil` 传下去），`Browse` 委托：

`live.go`：

```go
func (p *Provider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}

// BrowseFiltered 直播不参与筛选，忽略 filters。
func (p *Provider) BrowseFiltered(ctx context.Context, cat string, page int, _ map[string]string) (provider.Page, error) {
	// …原 Browse 的函数体原样搬到这里…
}
```

`tvbox.go`：

```go
func (p *Provider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}

func (p *Provider) BrowseFiltered(ctx context.Context, cat string, page int, filters map[string]string) (provider.Page, error) {
	// 原 Browse 函数体；videolistPage 的 filter 参数在本任务先传 ""（Task 5 再接入）。
	…原函数体…
}
```

`drpy.go`、`spider.go` 同样：原 Browse 体移入 `BrowseFiltered`，`Browse` 委托（task 6 才接 filters 语义）。

- [ ] **Step 4: 验证绿**：`go build ./... && go test ./... -count=1`

- [ ] **Step 5: 提交**

```bash
git add internal/provider/live internal/provider/tvbox
git commit -m "refactor(provider): 各 Provider 实现 BrowseFiltered 并让 Browse 委托空参"
```

---

### Task 3: crawler `VodCategoryExtend`（extend 进 categoryContent + filter_url 模板合并）

**Files:**
- Modify: `internal/crawler/drpy.go`
- Modify: `internal/crawler/template.go`
- Test: `internal/crawler/template_test.go`

**Interfaces:**
- Consumes: `Rule.FilterDef map[string]any`、`Rule.FilterURL`、`filterDefaults` / `renderFilterTemplate`。
- Produces: `func (e *Engine) VodCategoryExtend(tid string, pg int, extend map[string]string) ([]Vod, error)`；`VodCategory(tid, pg)` 委托 `VodCategoryExtend(tid, pg, nil)`。

- [ ] **Step 1: 写失败测试**（`internal/crawler/template_test.go`）

```go
// 用户选择的筛选值必须同时进入 categoryContent 的 extend 与 {{fl.xxx}} 模板：
// 旧实现只用 filter_def 默认值渲染，选择值无处可去。
func TestVodCategoryExtendPassesExtendAndMergesTemplate(t *testing.T) {
	e := New()
	src := `var rule = {
		host: "https://example.com",
		url: "/list/fyclass-fyfilter-fypage.html",
		filter_url: "&area={{fl.area}}&year={{fl.year}}",
		filter_def: {"1": {"area": "内地", "year": "2023"}}
	};
	function categoryContent(name, tid, pg, filter, extend){
		return JSON.stringify({list:[{vod_id:"1", vod_name:"热片", vod_remarks: extend ? (extend.area + "/" + extend.year) : "无"}]});
	}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	// 用户选了不同地区：模板里的 {{fl.area}} 应换成用户值而不是默认值
	vods, err := e.VodCategoryExtend("1", 1, map[string]string{"area": "欧美"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vods) == 0 || vods[0].VodName != "热片" {
		t.Fatalf("category 返回异常: %+v", vods)
	}
	if vods[0].VodRemarks != "欧美/2023" {
		t.Fatalf("extend 未把用户选择传入脚本: %q", vods[0].VodRemarks)
	}
}
```

- [ ] **Step 2: 验证红**：`go test ./internal/crawler/ -run TestVodCategoryExtendPassesExtendAndMergesTemplate -count=1` → FAIL（VodCategoryExtend 未定义）

- [ ] **Step 3: 实现**

`template.go` 的 `VodCategory` 改为：

```go
func (e *Engine) VodCategory(tid string, pg int) ([]Vod, error) {
	return e.VodCategoryExtend(tid, pg, nil)
}

func (e *Engine) VodCategoryExtend(tid string, pg int, extend map[string]string) ([]Vod, error) {
	for _, name := range []string{"categoryContent", "category", "categoryVod"} {
		if hasFunction(e, name) {
			ext := e.vm.NewObject()
			for k, v := range extend {
				_ = ext.Set(k, v)
			}
			return e.callVods(name, e.vm.ToValue(tid), e.vm.ToValue(strconv.Itoa(pg)), e.vm.ToValue(false), ext)
		}
	}
	rule, err := e.Rule()
	if err != nil {
		return nil, noRuleError("分类", "categoryContent/category/categoryVod")
	}
	if rule.URL != "" {
		path := fillURL(applyFilterTemplateWithSel(rule.URL, tid, rule, extend), tid, pg)
		html, fetchErr := e.fetchHTMLWithRule(joinURL(rule.Host, path), rule)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return e.extractVods("一级", html, rule)
	}
	…原 VodCategory 的 rule.ClassURL 分支原样保留…
}
```

`drpy.go` 增（保留 `applyFilterTemplate` 供 VodCategory 原路径，避免行为漂移）：

```go
// applyFilterTemplateWithSel 在 filterDefaults 之上合并用户选择（选择覆盖默认值），
// 再渲染 {{fl.xxx}}。sel 为 nil 等价 applyFilterTemplate（只有默认值）。
func applyFilterTemplateWithSel(path, tid string, rule *Rule, sel map[string]string) string {
	if rule == nil || strings.TrimSpace(rule.FilterURL) == "" {
		return path
	}
	values := filterDefaults(rule, tid)
	for k, v := range sel {
		values[k] = v
	}
	return renderFilterTemplate(expandFilterURL(path, tid, rule), values)
}

// expandFilterURL 把原 applyFilterTemplate 里 fyfilter→filter_url（或追加）展开单独抽出，
// 避免重复。原 applyFilterTemplate 改为调用 expandFilterURL + filterDefaults 渲染。
func expandFilterURL(path, tid string, rule *Rule) string {
	filterURL := strings.TrimSpace(rule.FilterURL)
	if strings.Contains(path, "fyfilter") {
		path = strings.Replace(path, "fyfilter", filterURL, 1)
	} else {
		if !strings.HasSuffix(path, "&") && !strings.HasPrefix(filterURL, "&") {
			path += "&"
		}
		path += filterURL
	}
	return strings.ReplaceAll(path, "fyclass", tid)
}
```

重构 `applyFilterTemplate` 为：

```go
func applyFilterTemplate(path, tid string, rule *Rule) string {
	if rule == nil || strings.TrimSpace(rule.FilterURL) == "" {
		return path
	}
	return renderFilterTemplate(expandFilterURL(path, tid, rule), filterDefaults(rule, tid))
}
```

- [ ] **Step 4: 验证绿**：`go test ./internal/crawler/ -count=1 && go vet ./internal/crawler/ && gofmt -l internal/crawler/`

- [ ] **Step 5: 提交**

```bash
git add internal/crawler/drpy.go internal/crawler/template.go internal/crawler/template_test.go
git commit -m "feat(crawler): VodCategoryExtend 透传筛选值到 extend 与 filter_url 模板"
```

---

### Task 4: tvbox 筛选组组装器（rule.filter_def + cms filter/filters）

**Files:**
- Create: `internal/provider/tvbox/filters.go`
- Test: `internal/provider/tvbox/filters_test.go`

**Interfaces:**
- Consumes: `provider.FilterGroup/FilterOption`（Task 1）；`cmsClass`、`Rule.FilterDef`。
- Produces:
  - `func filterGroupsFromFilterDef(raw map[string]any) []provider.FilterGroup`
  - `func filterGroupsFromCMSFilter(raw json.RawMessage) []provider.FilterGroup`
  - `func filterGroupsFromCMSDict(raw map[string]any) []provider.FilterGroup`（共用字典组装）

- [ ] **Step 1: 写失败测试**

```go
package tvbox

import (
	"testing"

	"github.com/unbox/unbox/internal/provider"
)

func TestFilterGroupsFromFilterDefNestedForm(t *testing.T) {
	// drpy2 filter_def 常见形态：{tid: {key: {name, options:[{n,v}], value}}}
	raw := map[string]any{
		"area": map[string]any{
			"name":    "地区",
			"options": []any{map[string]any{"n": "全部", "v": ""}, map[string]any{"n": "内地", "v": "内地"}},
			"value":   "",
		},
		"year": map[string]any{
			"name":    "年份",
			"options": []any{map[string]any{"n": "2023", "v": "2023"}},
		},
		"empty": map[string]any{"name": "空维度", "options": []any{}},
	}
	got := filterGroupsFromFilterDef(raw)
	if len(got) != 2 {
		t.Fatalf("空 options 的维度应剔除，实际 %d 组: %+v", len(got), got)
	}
	byKey := map[string]provider.FilterGroup{}
	for _, g := range got {
		byKey[g.Key] = g
	}
	if g := byKey["area"]; g.Name != "地区" || len(g.Options) != 2 || g.Options[1].Value != "内地" {
		t.Fatalf("area 组装错误: %+v", g)
	}
	if g := byKey["year"]; len(g.Options) != 1 || g.Options[0].Name != "2023" {
		t.Fatalf("year 组装错误: %+v", g)
	}
}

func TestFilterGroupsFromFilterDefScalarFormYieldsNothing(t *testing.T) {
	// 标量默认值形态（现 filterDefaults 处理的）没有可选列表，不暴露筛选
	raw := map[string]any{"area": "内地", "year": "2023"}
	if got := filterGroupsFromFilterDef(raw); len(got) != 0 {
		t.Fatalf("标量默认值不应产生筛选项: %+v", got)
	}
}
```

- [ ] **Step 2: 验证红**：`go test ./internal/provider/tvbox/ -run TestFilterGroups -count=1` → FAIL（函数未定义）

- [ ] **Step 3: 实现**（`filters.go`，只认嵌套对象形态，`n`/`v` 双 key、空 options 剔除、解析失败静默降级）

```go
package tvbox

import (
	"encoding/json"

	"github.com/unbox/unbox/internal/provider"
)

// optionText 兼容 option 的 n / {name,value} 两种写法：优先 display（n/name），
// 提交值（v/value）缺省回退到显示名。
func optionText(opt map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := opt[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// filterGroupsFromFilterDef 组装 drpy2 rule.filter_def 某分类的筛选组。
// 只认「name + options」的嵌套对象形态；options 为空或解析失败则该维度剔除。
// 标量默认值形态（如 {"area":"内地"}）无可选项，返回空。
func filterGroupsFromFilterDef(raw map[string]any) []provider.FilterGroup {
	return filterGroupsFromDict(raw)
}

// filterGroupsFromCMSDict 把任意 {key:{name, options:[…]}, …} 字典组装成筛选组。
func filterGroupsFromDict(raw map[string]any) []provider.FilterGroup {
	var out []provider.FilterGroup
	for key, value := range raw {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name := optionText(entry, "name", "n")
		rawOpts, ok := entry["options"].([]any)
		if !ok || len(rawOpts) == 0 {
			continue
		}
		opts := make([]provider.FilterOption, 0, len(rawOpts))
		for _, ro := range rawOpts {
			opt, ok := ro.(map[string]any)
			if !ok {
				continue
			}
			display := optionText(opt, "n", "name")
			if display == "" {
				continue
			}
			val := optionText(opt, "v", "value")
			if val == "" {
				val = display
			}
			opts = append(opts, provider.FilterOption{Name: display, Value: val})
		}
		if len(opts) == 0 {
			continue
		}
		out = append(out, provider.FilterGroup{Key: key, Name: name, Options: opts})
	}
	return out
}

// filterGroupsFromCMSFilter 解析 maccms class 条目里的 filter/filters 字段。
// 畸形 / 空内容一律返回 nil（不阻断分类浏览）。
func filterGroupsFromCMSFilter(raw json.RawMessage) []provider.FilterGroup {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" || string(raw) == "{}" {
		return nil
	}
	var dict map[string]any
	if err := json.Unmarshal(raw, &dict); err != nil {
		return nil
	}
	return filterGroupsFromDict(dict)
}
```

- [ ] **Step 4: 验证绿**：补充 CMS 形态用例后 `go test ./internal/provider/tvbox/ -count=1`（加：`filterGroupsFromCMSFilter([]byte(`{"area":{"name":"地区","options":[{"n":"全部","v":""}]}}`))` 返回 1 组；`[]`/`{}`/null/畸形 JSON 返回 nil）

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/filters.go internal/provider/tvbox/filters_test.go
git commit -m "feat(tvbox): 筛选组组装器（rule.filter_def + cms filter/filters）"
```

---

### Task 5: CMS BrowseFiltered 接 filter 参数 + Home 挂 Filters

**Files:**
- Modify: `internal/provider/tvbox/cms.go`
- Modify: `internal/provider/tvbox/tvbox.go`

**Interfaces:**
- Consumes: `filterGroupsFromCMSFilter`（Task 4）。
- Produces: `client.listFiltered(ctx, t, wd string, page int, filter string)`、`client.videolistFilteredPage(ctx, t, wd string, page int, filter string)`；`Provider.BrowseFiltered` 真正接筛选值。

- [ ] **Step 1: 写失败测试**（`internal/provider/tvbox/cms_test.go` 或现存测试；断言 filter 参数出现在请求 query 且 URL 编码正确）

```go
func TestVideolistFilteredEncodesFilterParam(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"list":[]}`))
	}))
	defer srv.Close()
	c := &client{base: srv.URL, hc: httpx.NewClient(5 * time.Second)}
	if _, err := c.listFiltered(context.Background(), "", "", 1, `{"area":"内地","year":"2023"}`); err != nil {
		t.Fatal(err)
	}
	if got.Get("filter") == "" {
		t.Fatal("filter 参数缺失")
	}
	filterJSON, err := url.QueryUnescape(got.Get("filter"))
	if err != nil || filterJSON != `{"area":"内地","year":"2023"}` {
		t.Fatalf("filter 参数应为 URL 编码的 JSON，实际 %q", got.Get("filter"))
	}
}
```

- [ ] **Step 2: 验证红**：`go test ./internal/provider/tvbox/ -run TestVideolistFiltered -count=1` → FAIL（listFiltered 未定义）

- [ ] **Step 3: 实现**

`cms.go`：

```go
// listFiltered 带筛选条件拉列表；filter 为选中的维度 JSON 文本（空串表示无筛选）。
func (c *client) listFiltered(ctx context.Context, t, wd string, page int, filter string) ([]cmsVideo, error) {
	resp, err := c.videolistFilteredPage(ctx, t, wd, page, filter)
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

func (c *client) videolistFilteredPage(ctx context.Context, t, wd string, page int, filter string) (cmsResp, error) {
	q := url.Values{"ac": {"videolist"}, "pg": {fmt.Sprintf("%d", page)}}
	if t != "" {
		q.Set("t", t)
	}
	if wd != "" {
		q.Set("wd", wd)
	}
	if filter != "" && filter != "{}" {
		q.Set("filter", filter) // url.Values.Encode 自动 URL 编码
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

// videolistPage 保持原签名与行为，委托空 filter。
func (c *client) videolistPage(ctx context.Context, t, wd string, page int) (cmsResp, error) {
	return c.videolistFilteredPage(ctx, t, wd, page, "")
}
```

`cms.go` 的 `cmsClass` 增字段与解析辅助：

```go
type cmsClass struct {
	TypeID   flexID          `json:"type_id"`
	TypeName string          `json:"type_name"`
	Filters  json.RawMessage `json:"filters"`
	Filter   json.RawMessage `json:"filter"`
}

// filterGroups 返回该分类解析出的筛选组；失败/空则 nil。
func (c cmsClass) filterGroups() []provider.FilterGroup {
	if len(c.Filters) > 0 {
		if g := filterGroupsFromCMSFilter(c.Filters); len(g) > 0 {
			return g
		}
	}
	return filterGroupsFromCMSFilter(c.Filter)
}
```

`tvbox.go` 的 `Home` 与 `BrowseFiltered`：

```go
// Home：sections 组装处
sections = append(sections, provider.Section{ID: id, Title: name, Filters: class.filterGroups()})
// 注意第二个分支（videolist 回退）没有 class 结构，Filters 留空即可。

// BrowseFiltered：原 Browse 体 + filters 生效
func (p *Provider) BrowseFiltered(ctx context.Context, cat string, page int, filters map[string]string) (provider.Page, error) {
	filterJSON := ""
	if len(filters) > 0 {
		b, err := json.Marshal(filters)
		if err != nil {
			return provider.Page{}, fmt.Errorf("筛选条件序列化失败: %w", err)
		}
		filterJSON = string(b)
	}
	resp, err := p.c.videolistFilteredPage(ctx, cat, "", page, filterJSON)
	…原函数体其余不变…
}
```

`tvbox.go` 需新增 `encoding/json` import。

- [ ] **Step 4: 验证绿**：`go test ./internal/provider/tvbox/ -count=1 && go vet ./internal/provider/tvbox/ && gofmt -l internal/provider/tvbox/`

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/cms.go internal/provider/tvbox/tvbox.go
git commit -m "feat(tvbox): CMS 解析 class 筛选定义并透传 filter 参数"
```

---

### Task 6: Spider BrowseFiltered 接 extend + Home 挂 Filters

**Files:**
- Modify: `internal/provider/tvbox/spider.go`

**Interfaces:**
- Consumes: `Engine.VodCategoryExtend`（Task 3）、`filterGroupsFromFilterDef`（Task 4）。
- Produces: `Spider.Home` 的 Section 携带 Filters；`Spider.BrowseFiltered` 走筛选项。

- [ ] **Step 1: 写失败测试**（`internal/provider/tvbox/spider_test.go`，用现有 `testdata/crawler/*.js` 基建；断言 BrowseFiltered 后 items 的 remarks 反映了 extend）

```go
func TestSpiderBrowseFilteredPassesExtend(t *testing.T) {
	site := config.Site{Type: 3, API: filepath.Join("testdata", "filter_site.js"), Key: "sp", Name: "筛选站"}
	if !filepath.IsAbs(site.API) {
		abs, _ := filepath.Abs(site.API)
		site.API = abs
	}
	p, err := NewSpider(site)
	if err != nil {
		t.Fatal(err)
	}
	page, err := p.BrowseFiltered(context.Background(), "1", 1, map[string]string{"area": "欧美"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Group != "欧美" {
		t.Fatalf("extend 未生效: %+v", page.Items)
	}
}
```

配套 fixture `internal/provider/tvbox/testdata/filter_site.js`：

```js
var rule = {
  host: "https://example.com",
  url: "/list/fyclass-fypage.html",
  filter_url: "&area={{fl.area}}",
  filter_def: {"1": {"area": {"name": "地区", "options": [{"n": "全部", "v": ""}, {"n": "欧美", "v": "欧美"}], "value": ""}}},
  class_name: "电影",
  class_url: "1",
  vod_selector: ".item",
  id_selector: "a",
  name_selector: "h3",
};
function categoryContent(name, tid, pg, filter, extend) {
  var area = extend && extend.area ? extend.area : "默认";
  return JSON.stringify({list: [{vod_id: "1", vod_name: "热片", type_name: area}]});
}
```

- [ ] **Step 2: 验证红**：`go test ./internal/provider/tvbox/ -run TestSpiderBrowseFiltered -count=1` → FAIL（BrowseFiltered 未生效 / fixture 缺失）

- [ ] **Step 3: 实现**（`spider.go`）

```go
func (p *Spider) Home(ctx context.Context) ([]provider.Section, error) {
	…现有三分支不变；在每个返回 sections 的 return 前统一做：
	sections = p.attachFilters(sections)
	return sections, nil
}

// attachFilters 从 rule.filter_def 为每个分类挂载筛选组；无 Rule 或该分类无
// 定义时保持原样。
func (p *Spider) attachFilters(sections []provider.Section) []provider.Section {
	rule, err := p.engine.Rule()
	if err != nil || len(rule.FilterDef) == 0 {
		return sections
	}
	for i := range sections {
		raw, ok := rule.FilterDef[sections[i].ID].(map[string]any)
		if !ok {
			continue
		}
		sections[i].Filters = filterGroupsFromFilterDef(raw)
	}
	return sections
}

func (p *Spider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}

func (p *Spider) BrowseFiltered(ctx context.Context, cat string, page int, filters map[string]string) (provider.Page, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Page{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.engine.Init(); err != nil {
		return provider.Page{}, err
	}
	items, err := p.engine.VodCategoryExtend(cat, page, filters)
	if err != nil {
		return provider.Page{}, err
	}
	return provider.Page{Items: spiderItems(items), Page: page, HasMore: len(items) > 0}, nil
}
```

（`Browse` 原实现删除，改委托。）

- [ ] **Step 4: 验证绿**：`go test ./internal/provider/tvbox/ -count=1 && go vet ./internal/provider/tvbox/ && gofmt -l internal/provider/tvbox/`

- [ ] **Step 5: 提交**

```bash
git add internal/provider/tvbox/spider.go internal/provider/tvbox/spider_test.go internal/provider/tvbox/testdata/filter_site.js
git commit -m "feat(tvbox): Spider 挂载 filter_def 筛选组并将选择值传入 extend"
```

---

### Task 7: shell 绑定（`VodListFiltered`、Section.Filters 透传）+ 绑定重生成

**Files:**
- Modify: `internal/shell/service.go`
- Modify: `internal/shell/models.go`（Section 结构，若存在）
- Run: `wails3 task generate:bindings`（带 `-ts`，Taskfile 已配置）

**Interfaces:**
- Consumes: `Section.Filters`、`Provider.BrowseFiltered`。
- Produces: shell 暴露 `Section.Filters` 到前端模型；新增 `VodListFiltered(site, cat string, page int, filters map[string]string) (VodListPage, error)`。

- [ ] **Step 1: 写失败测试**（`internal/shell/service_test.go`）

```go
func TestVodListFilteredRoutesFilters(t *testing.T) {
	svc := NewShellService(nil, nil, nil)
	type probe struct{ got map[string]string }
	pr := &probe{}
	svc.vods = map[string]provider.Provider{
		"f": filterProbeProvider{pr: pr},
	}
	if _, err := svc.VodListFiltered("f", "1", 1, map[string]string{"area": "欧美"}); err != nil {
		t.Fatal(err)
	}
	if pr.got["area"] != "欧美" {
		t.Fatalf("VodListFiltered 未透传筛选值: %+v", pr.got)
	}
}
```

配套桩（复用现有 `stubSearchProvider` 风格，需实现 Task 1 后的完整接口）：

```go
type filterProbeProvider struct {
	provider.Provider
	pr *struct{ got map[string]string }
}
func (p filterProbeProvider) BrowseFiltered(ctx context.Context, _ string, _ int, filters map[string]string) (provider.Page, error) {
	if p.pr != nil {
		p.pr.got = filters
	}
	return provider.Page{}, nil
}
// Browse 由内嵌 provider.Provider 提供；这里补一个显式实现避免 nil 引用：
func (p filterProbeProvider) Browse(ctx context.Context, cat string, page int) (provider.Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}
```

- [ ] **Step 2: 验证红**：`go test ./internal/shell/ -run TestVodListFiltered -count=1` → FAIL

- [ ] **Step 3: 实现**（`service.go` 紧邻 `VodList`）

```go
// VodListFiltered 带筛选值浏览分类；filters 可为 nil（等价 VodList）。
func (s *ShellService) VodListFiltered(site, cat string, page int, filters map[string]string) (VodListPage, error) {
	pv, err := s.vodOf(site)
	if err != nil {
		return VodListPage{}, err
	}
	pg, err := pv.BrowseFiltered(context.Background(), cat, page, filters)
	if err != nil {
		return VodListPage{}, err
	}
	return VodListPage{
		Items:     toVodItems(pg.Items),
		Page:      pg.Page,
		PageCount: pg.PageCount,
		Total:     pg.Total,
		HasMore:   pg.HasMore,
	}, nil
}
```

`VodCategories` 的 Section 映射带 Filters（`internal/shell` 的 `Section` 结构加字段）：

```go
// shell.Section 增加：
Filters []provider.FilterGroup `json:"filters,omitempty"`
// VodCategories 映射处：
out[i] = Section{ID: sc.ID, Title: sc.Title, Filters: sc.Filters}
```

- [ ] **Step 4: 验证绿**：`go test ./internal/shell/ -count=1 && go vet ./internal/shell/ && gofmt -l internal/shell/`，然后：

```bash
wails3 task generate:bindings
```

确认 `frontend/bindings/github.com/unbox/unbox/internal/shell/models.ts` 的 `Section` 含 `"filters"?: FilterGroup[]`、`FilterGroup`/`FilterOption` 类型生成，且 `ShellService` 含 `VodListFiltered`。若 bindings 未更新，手动补 `-ts` 跑：

```bash
wails3 generate bindings -ts -i
```

- [ ] **Step 5: 提交**

```bash
git add internal/shell/service.go internal/shell/models.go 2>/dev/null; git add -u frontend/bindings 2>/dev/null
git commit -m "feat(shell): VodListFiltered 绑定与 Section.Filters 透传"
```

---

### Task 8: 前端筛选状态机 `vodFilters.ts`（TDD 纯函数）

**Files:**
- Create: `frontend/src/vodFilters.ts`
- Test: `frontend/src/vodFilters.test.ts`

**Interfaces:**
- Consumes: Task 7 生成的 `Section.Filters`/`FilterGroup`/`FilterOption` 类型。
- Produces:
  - `export function availableFilterGroups(cat?: { Filters?: FilterGroup[] } | null): FilterGroup[]`
  - `export function initialSelections(groups: FilterGroup[]): Record<string, string>`
  - `export function nextSelections(current: Record<string, string>, key: string, value: string): Record<string, string>`
  - `export function hasActiveFilter(active: Record<string, string> | null): boolean`

- [ ] **Step 1: 写失败测试**

```ts
import { describe, expect, it } from 'vitest'
import {
  availableFilterGroups,
  hasActiveFilter,
  initialSelections,
  nextSelections,
} from './vodFilters'
import type { FilterGroup } from '../bindings/github.com/unbox/unbox/internal/shell/models'

const groups: FilterGroup[] = [
  { key: 'area', name: '地区', options: [{ name: '全部', value: '' }, { name: '内地', value: '内地' }] },
  { key: 'year', name: '年份', options: [{ name: '全部', value: '' }, { name: '2023', value: '2023' }] },
]

describe('availableFilterGroups', () => {
  it('有非空筛选项的分类返回组列表', () => {
    expect(availableFilterGroups({ Filters: groups })).toHaveLength(2)
  })
  it('无筛选项 / 空数组 / undefined 都返回空', () => {
    expect(availableFilterGroups({ Filters: [] })).toEqual([])
    expect(availableFilterGroups({})).toEqual([])
    expect(availableFilterGroups(null)).toEqual([])
    expect(availableFilterGroups()).toEqual([])
  })
})

describe('initialSelections', () => {
  it('默认取每维度的第一个选项值', () => {
    expect(initialSelections(groups)).toEqual({ area: '', year: '' })
  })
  it('空组返回空对象', () => {
    expect(initialSelections([])).toEqual({})
  })
})

describe('nextSelections', () => {
  it('单选替换、不污染其它维度', () => {
    const s = nextSelections({ area: '', year: '' }, 'area', '内地')
    expect(s).toEqual({ area: '内地', year: '' })
  })
})

describe('hasActiveFilter', () => {
  it('有任一非空选择才算激活', () => {
    expect(hasActiveFilter({ area: '内地', year: '' })).toBe(true)
    expect(hasActiveFilter({ area: '', year: '' })).toBe(false)
    expect(hasActiveFilter(null)).toBe(false)
  })
})
```

- [ ] **Step 2: 验证红**：`cd frontend && npx vitest run src/vodFilters.test.ts` → FAIL（模块不存在）

- [ ] **Step 3: 实现**

```ts
// vodFilters.ts — 点播筛选的纯逻辑状态机。
// 有筛选项才暴露按钮（availableFilterGroups 非空），选择值经
// ShellService.VodListFiltered 透传给站点。
import type { FilterGroup } from '../bindings/github.com/unbox/unbox/internal/shell/models'

export function availableFilterGroups(cat?: { Filters?: FilterGroup[] } | null): FilterGroup[] {
  return cat?.Filters?.filter(g => g.options.length > 0) ?? []
}

export function initialSelections(groups: FilterGroup[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const g of groups) {
    const first = g.options[0]
    out[g.key] = first ? first.value : ''
  }
  return out
}

export function nextSelections(current: Record<string, string>, key: string, value: string): Record<string, string> {
  return { ...current, [key]: value }
}

export function hasActiveFilter(active: Record<string, string> | null): boolean {
  return !!active && Object.values(active).some(v => v !== '')
}
```

- [ ] **Step 4: 验证绿**：`npx vitest run src/vodFilters.test.ts && npx vue-tsc --noEmit`

- [ ] **Step 5: 提交**

```bash
git add frontend/src/vodFilters.ts frontend/src/vodFilters.test.ts
git commit -m "feat(frontend): vodFilters 筛选状态机纯函数"
```

---

### Task 9: 前端筛选抽屉/弹层 + 接入 App.vue

**Files:**
- Create: `frontend/src/components/VodFilterPanel.vue`
- Modify: `frontend/src/App.vue`

**Interfaces:**
- Consumes: Task 8 的纯函数、Task 7 的 `VodListFiltered` 绑定、`Section.Filters` 数据。
- Produces: 点播页筛选按钮（v-if 有筛选项）+ 弹层面板 + 重拉第 1 页。

- [ ] **Step 1: 写失败测试**（组件渲染断言；`@vue/test-utils` 已可用）

```ts
// frontend/src/components/VodFilterPanel.test.ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VodFilterPanel from './VodFilterPanel.vue'
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/shell/models'

const groups: FilterGroup[] = [
  { key: 'area', name: '地区', options: [{ name: '全部', value: '' }, { name: '内地', value: '内地' }] },
]

it('渲染每个维度并提交选择', async () => {
  const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '' } } })
  expect(wrapper.text()).toContain('地区')
  const opts = wrapper.findAll('select')
  expect(opts).toHaveLength(1)
  await opts[0].setValue('内地')
  expect(wrapper.emitted('update:modelValue')![0]).toEqual([{ area: '内地' }])
})
```

- [ ] **Step 2: 验证红**：`npx vitest run src/components/VodFilterPanel.test.ts` → FAIL

- [ ] **Step 3: 实现 `VodFilterPanel.vue`**

```vue
<script setup lang="ts">
// VodFilterPanel.vue — 点播筛选抽屉内容：按维度渲染单选框，选择实时回传。
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/shell/models'
import { nextSelections } from '../vodFilters'

const props = defineProps<{
  groups: FilterGroup[]
  modelValue: Record<string, string>
}>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

function onSelect(key: string, value: string) {
  emit('update:modelValue', nextSelections(props.modelValue, key, value))
}
</script>

<template>
  <div class="vod-filter-panel">
    <label v-for="g in groups" :key="g.key" class="vod-filter-dim">
      <span class="vod-filter-name">{{ g.name }}</span>
      <select :value="modelValue[g.key] ?? g.options[0]?.value ?? ''" @change="onSelect(g.key, ($event.target as HTMLSelectElement).value)">
        <option v-for="opt in g.options" :key="opt.value" :value="opt.value">{{ opt.name }}</option>
      </select>
    </label>
  </div>
</template>
```

配套样式追加到 `frontend/public/style.css`（`layoutContracts.test.ts` 不强制该组件，直接加）：

```css
.vod-filter-panel { display: flex; flex-direction: column; gap: 0.6rem; padding: 0.75rem; }
.vod-filter-dim { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; }
.vod-filter-name { color: var(--text-dim); flex: 0 0 auto; }
.vod-filter-dim select {
  flex: 1; min-width: 0; padding: 0.3rem 0.5rem;
  background: var(--btn-bg); color: var(--text);
  border: 1px solid var(--panel-border); border-radius: 0.35rem;
}
```

- [ ] **Step 4: 接入 `App.vue`**

状态与请求（在既有 `vodList*` 状态区）：

```ts
// 点播筛选：draft 是弹层内未提交的选择；active 是已应用、当前列表正使用的。
const vodFilterDraft = ref<Record<string, string>>({})
const vodFilterActive = ref<Record<string, string> | null>(null)
const vodFilterOpen = ref(false)

const activeVodCatObj = computed(() => vodCategories.value.find(c => c.ID === vodActiveCat.value))
const vodFilterGroups = computed(() => availableFilterGroups(activeVodCatObj.value))

function openVodFilter() {
  if (!vodFilterGroups.value.length) return
  vodFilterDraft.value = initialSelections(vodFilterGroups.value)
  if (vodFilterActive.value) {
    // 回显已应用值：优先当前选择，缺维度补默认
    vodFilterDraft.value = { ...vodFilterDraft.value, ...vodFilterActive.value }
  }
  vodFilterOpen.value = true
}

function applyVodFilter() {
  vodFilterActive.value = hasActiveFilter(vodFilterDraft.value) ? vodFilterDraft.value : null
  vodFilterOpen.value = false
  resetVodListState()
  void reloadVodList()
}
```

`reloadVodList` 换请求（`reset` 路径与续拉都带 active）：

```ts
const result = await ShellService.VodListFiltered(activeSite.value, vodActiveCat.value, page, vodFilterActive.value) as VodListPage
```

`selectVodCategory` 与 `selectSite` 里追加：

```ts
vodFilterActive.value = null
vodFilterOpen.value = false
```

模板（点播页标题栏 `vod-detail-top` 前、`vod-layout` 侧栏区，放筛选按钮 + 弹层；置顶遮罩）：

```vue
<button v-if="vodFilterGroups.length" type="button" class="vod-filter-toggle"
        :class="{ 'has-filter': hasActiveFilter(vodFilterActive.value) }"
        @click="vodFilterOpen = !vodFilterOpen">筛选{{ hasActiveFilter(vodFilterActive.value) ? ' ·' : '' }}</button>

<div v-if="vodFilterOpen" class="vod-filter-mask" @click.self="vodFilterOpen = false">
  <div class="vod-filter-card">
    <VodFilterPanel v-model="vodFilterDraft" :groups="vodFilterGroups" />
    <div class="vod-filter-actions">
      <button type="button" @click="vodFilterOpen = false">取消</button>
      <button type="button" class="primary" @click="applyVodFilter">确定</button>
    </div>
  </div>
</div>
```

遮罩/卡片样式追加 `frontend/public/style.css`，参考现有弹层（`mpv-install`、`content-card-delete-mask` 的遮罩观感；固定定位 + 半透明底 + 居中最上层）。

- [ ] **Step 5: 验证**：`npx vitest run && npx vue-tsc --noEmit` 全绿；`go test ./... -count=1` 保持绿。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/VodFilterPanel.vue frontend/src/components/VodFilterPanel.test.ts frontend/src/App.vue frontend/public/style.css
git commit -m "feat(frontend): 点播筛选抽屉/弹层，有筛选项才显示入口"
```

---

### Task 10: 验收收尾（全链路 + 文档 + 回归）

**Files:**
- Modify: `docs/2026-10-17-m7-acceptance.md`（补筛选 UI 验收记录）
- Modify: `AGENTS.md`（M7 剩余缺口更新：筛选 UI 已实现）

- [ ] **Step 1: 全量回归**：`go test ./... -count=1 && go vet ./... && gofmt -l . && cd frontend && npx vitest run && npx vue-tsc --noEmit`

- [ ] **Step 2: 手工冒烟（若有 dev 环境）**：`mise run dev`，导入一个带 filter_def 的 drpy2 站点与一个带 filters 的 maccms 站点，验证：筛选按钮按分类显示、弹层选择 → 重拉列表带筛选值、分页续拉沿用、切分类/站点复位、无筛选项时按钮隐藏。

- [ ] **Step 3: 更新文档**：`AGENTS.md` 的 M7 段把「剩余缺口是筛选 UI」改为「筛选 UI 已实现（drpy2 规则站 + CMS）」，`docs/2026-10-17-m7-acceptance.md` 补一段筛选验收结果。

- [ ] **Step 4: 提交**

```bash
git add AGENTS.md docs/2026-10-17-m7-acceptance.md
git commit -m "docs: 点播筛选 UI 验收与项目状态更新"
```