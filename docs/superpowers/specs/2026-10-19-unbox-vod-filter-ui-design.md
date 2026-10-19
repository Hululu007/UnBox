# UnBox 点播筛选 UI — 设计文档

> Spec：2026-10-19，M7 剩余缺口①（前端不展示 `filter` 选项）。
> 配套验收：`docs/2026-10-17-m7-acceptance.md`（引擎侧 `fyfilter`/`filter_url` 已展开）。

## 1. 背景与目标

UnBox 的点播引擎与 CMS 客户端已经把筛选数据解析到内部：
drpy2 规则站有 `rule.filter_def`/`filter_url`，CMS（maccms 系）的 `ac=list`
响应里每个分类带 `filter`/`filters` 字典。但**前端没有任何筛选 UI**，用户无法选择
地区 / 年份 / 语言等维度，实际拿到的一直是站点默认值。

目标：在点播页提供「筛选」入口（抽屉/弹层），让有筛选项的分类可以按维度选择并
重新拉取列表；**只有该分类存在筛选项时才显示筛选按钮**。

范围：
- 覆盖 drpy2 规则站（`Spider`）与 CMS（maccms 系，`Provider.Browse` 的 vods）。
- drpy 原生 http 服务（`/api/category`，`Drpy`）首版不接，Section.Filters 留空。
- 直播（live / M3U）不受影响，`BrowseFiltered` 实现为忽略筛选参数。
- 不做：筛选值持久化（只会话内记忆）、筛选历史的 URL 记忆、多选维度（单选下拉）。

## 2. 数据模型（Go 侧）

`internal/provider/` 新增 `filters.go`：

```go
// FilterOption 是单个可选项（如「内地」）。
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

`Section`（`internal/provider/provider.go`）增加字段并保持向后兼容：

```go
type Section struct {
    ID      string
    Title   string
    Filters []FilterGroup // 该分类的筛选定义；无筛选时为空
}
```

### Provider 接口扩展

```go
// BrowseFiltered 带筛选值浏览分类。filters 为「维度 key → 选中的 value」，
// 键值不足（未选维度）由实现方填站点默认值。filters 为 nil 等价 Browse。
BrowseFiltered(ctx context.Context, cat string, page int, filters map[string]string) (Page, error)
```

- `Browse` 保留原签名，统一委托 `BrowseFiltered(ctx, cat, page, nil)`。
- 各 Provider 的现有分类逻辑平移到 `BrowseFiltered`；直播等不支持的实现忽略
  `filters`（等价空参），不做额外动作。

理由（方案取舍）：改 `Browse` 签名会破坏所有 Provider 实现与调用方；新增方法
让关注点集中、迁移成本最低。`filters` 用 `map[string]string` 而非结构体，因为
各站点的维度键/值的形态差异大，透传比强类型建模更稳。

## 3. 各源的数据接入

### 3.1 drpy2 规则站（`Spider`，`internal/provider/tvbox/spider.go`）

**筛选项定义**：`crawler.Rule.FilterDef`（`map[tid]any`），条目形如
`{"area": {"name":"地区","options":[{"n":"内地","v":"内地"}...]}}`。
- 新增 `crawler` 辅助：`FilterGroups(rule, tid) []provider-ish FilterGroup`？
  注意：crawler 不能 import provider（依赖方向），故该组装器放 `tvbox` 包，
  由 `spider.go` 读取 `engine.Rule()` 后本地组装。
- `Spider.Home` 返回 Sections 前，对每个分类用 `rule.FilterDef[tid]` 组装
  `Filters`。Home 已加载引擎（`ensureLoaded`），无额外请求。

**筛选值应用**：
- `categoryContent(name, tid, pg, filter, extend)`：把 `filters` 合并站点默认值
  后转成 goja `extend` 对象传入（现状传空对象，`VodCategory` 的首参不变）。
- 声明式路径（`categoryContent` 缺失、走 `rule.URL` + `applyFilterTemplate`）：
  `renderFilterTemplate` 以「选择值覆盖默认值」后的 map 渲染 `{{fl.xxx}}`。
- `Engine` 扩展：`VodCategoryExtend(tid string, pg int, extend map[string]string) ([]Vod, error)`，
  `VodCategory` 委托默认空 map（保持现有行为与既有测试绿）。

**兼容细节**：`filter_def` 里 options 的 `n`（显示名）/`v`（提交值）两种 key 都要认；
条目缺 `options` 或为空时该维度不暴露（不生成空组）。

### 3.2 CMS（maccms 系，`internal/provider/tvbox/cms.go`）

**筛选项定义**：`ac=list` 的 `class[]` 条目可能带 `filter` 或 `filters`（站点差异），
形如：

```json
{"type_id": "1", "type_name": "电影", "filters": {
  "area": {"name": "地区", "options": [{"n": "全部", "v": ""}, {"n": "内地", "v": "内地"}]},
  "year": {"name": "年份", "options": [{"n": "全部", "v": ""}]}
}}
```

- `cmsClass` 增加 `Filter  json.RawMessage` + `Filters json.RawMessage`，解析时
  优先 `filters`、回退 `filter`；解析失败或为空 → `nil`（不影响分类浏览，
  与现状一致）。组装器把字典转 `[]FilterGroup`（同规则站：`n`/`v` 双 key 兼容，
  空 options 的维度剔除）。
- 需要真实站点样本验证 maccms 具体 key（设计上字段名双兼容 + 失败静默降级，
  给足容错空间）。

**筛选值应用**：`videolistPage` 增加可空 `filter string` 参数 — 拼
`q.Set("filter", jsonString)`（`url.Values.Encode` 自动 URL 编码）。值形如
`{"area":"内地","year":"2023"}`（仅含用户选中的维度）。首版 GET；上线后如有
站点拒收再按站点探测 POST（设计范围内不做）。

- `Provider.BrowseFiltered`：`videolistPage(ctx, t, "", page, filterJSON)`。
- 现有 `videolist(ctx, t, wd, page)` 保留为搜索/无筛选路径，委托填空 filter。

### 3.3 drpy http 服务（`Drpy`）

`/api/category?tid&pg` 一般不支持筛选。`Drpy.Home` 的 Section 不填 Filters、
`BrowseFiltered` 忽略 filters（走原 category 请求）。实现上零成本。

### 3.4 直播（`provider/live`）

`BrowseFiltered` 忽略 filters（委托现有 Browse 逻辑），签名对齐即可，行为不变。

## 4. 前端（Vue3，`frontend/src/App.vue` + 新模块）

### 4.1 数据接入

- Go 绑定重新生成后，`Section.Filters` / 新增 `VodListFiltered` 进 `models.ts`。
- 新增 `frontend/src/vodFilters.ts`（纯逻辑，可单测）：
  - `availableFilterGroups(cat: Section): FilterGroup[]` — 分类有 Filters 且
    至少一个非空组才返回；这是「有筛选项才显示按钮」的判定。
  - `defaultFilterSelections(groups): Record<string,string>` — 每维度取
    options[0].Value（「全部」）作为初始选择。
  - `applyFilterChange(state, key, value)` / `resetFilterState(state)` —
    单选替换/整体复位。
- 状态：`const vodFilters = ref<Record<string, string>>({})` +
  `const vodActiveFilters = ref<Record<string, string> | null>(null)`（已应用的），
  切分类/切站点调用 `resetFilterState` 清空并置 `vodActiveFilters = null`。

### 4.2 交互（抽屉/弹层）

- 点播页标题栏右侧、该分类**有筛选项才显示**「筛选」按钮（用分类的
  `availableFilterGroups` 判定，`v-if` 包住）。
- 点击 → 浮层面板（覆盖在点播区之上，半透明遮罩 + 居中/侧滑卡片）：
  - 每个筛选维度一组单选下拉（或可点标签列表，按 `ContentCardStyle` 语义做下拉更省空间）；每维度带「全部」默认项。
  - 底部「确定 / 取消」。「确定」提交当前 choices → `vodActiveFilters`、重拉第 1 页；
    「取消」丢弃未提交的选择，面板关闭。
  - 面板关闭不自动重拉，只有「确定」触发。
- 已应用筛选时按钮带激活态（角标/高亮），再次打开面板回显当前已应用值。
- 分页续拉（`loadNextVodPage`）沿用 `vodActiveFilters`（请求同一个 site+cat+filters）。

### 4.3 列表请求

- `reloadVodList` 改走 `ShellService.VodListFiltered(site, cat, page, vodActiveFilters)`；
  request-id 消歧逻辑不变（`vodListRequest` 递增）。
- 筛选变更 → `resetVodListState()` + 第 1 页。

## 5. 错误处理与回退

- 带 filter 的请求失败：沿用现有 `handleError`（弹错、列表清空或保留旧内容），
  不区分是否带筛选。
- 站点对未知维度/值返回空列表：属于站点行为，前端不做二次猜测，如实展示
  「暂无结果」。筛选值透传不增删。
- 解析 `filter_def`/`class.filters` 失败：一律降级为无筛选（不阻断分类/浏览）。

## 6. 测试策略（TDD）

Go：
- `tvbox.FilterGroupsFromFilterDef(dict)` / `cmsFilterGroups(raw)` 单测：
  `n`/`v` 双 key、空 options 剔除、`filter`/`filters` 字段回退、畸形输入降级为空。
- `cms.videolistPage` 带 filter 参数断言 URL query 正确（含 URL 编码）。
- `Spider.BrowseFiltered`：`extend` 传入 goja 的 smoke 断言（用现有 M7 测试基建）。
- 既有 `Browse` 路径测试保持绿（委托不改变行为）。

前端：
- `vodFilters.ts` 纯函数单测：默认选择、单选替换、复位、「有筛选项才显示」判定。
- `App.vue` 的筛选按钮显隐与确定/取消流：若抽取纯状态机则单测；组件渲染
  依赖点按 App.playback.test 既有模式补桩。

## 7. 不做什么（YAGNI）

- 不做跨站点筛选记忆落盘、URL 记忆；不做多选/区间筛选。
- 不做 `Drpy` http 服务的筛选（站点不支持）。
- 不在 CMS 层做 POST 探测（首版 GET；站点拒收属外部行为，后续按站适配）。

## 8. 交付物清单

- `internal/provider/filters.go`（FilterOption/FilterGroup）、`provider.go` 的
  `Section.Filters` 与 `BrowseFiltered` 接口。
- `internal/crawler`：`VodCategoryExtend`（+ 现有 `VodCategory` 委托）。
- `internal/provider/tvbox`：spider 组装 FilterDef→Groups、cms filters 解析 +
  videolist filter 参数 + `BrowseFiltered`。
- `internal/provider/live`：`BrowseFiltered` 忽略实现。
- 前端：`vodFilters.ts` + 测试、`App.vue` 筛选按钮/弹层、绑定重生成。
- 文档：本 spec + 实施计划（writing-plans）。