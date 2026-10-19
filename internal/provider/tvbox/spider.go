package tvbox

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/crawler"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
)

// Spider 是 type=3 且 api 指向 .js 文件的爬虫 Provider。
// 爬虫脚本在第一次动作时加载，避免导入源阶段阻塞网络请求。
type Spider struct {
	site config.Site

	mu      sync.Mutex
	engine  *crawler.Engine
	loaded  bool
	loadErr error
	cache   map[string]provider.Episode
}

// NewSpider 用站点定义构造 JS 爬虫 Provider。
func NewSpider(site config.Site) (*Spider, error) {
	if strings.TrimSpace(site.API) == "" {
		return nil, fmt.Errorf("JS 爬虫地址为空")
	}
	engine := crawler.New()
	engine.SetExt(scriptExt(site))
	engine.SetSiteMeta(site.Key, int(site.Type))
	return &Spider{site: site, engine: engine, cache: make(map[string]provider.Episode)}, nil
}

// scriptExt 返回传给脚本 init 的 ext 文本。
//
// 真实配置里 ext 既可能是字符串，也可能是 JSON 对象（如 bili_open 的
// {"categories": …, "cookie": …}）。Site.ExtString() 只取字符串，对象会变成
// 空串，于是脚本里 cfg.ext.hasOwnProperty(...) 直接报 TypeError。
func scriptExt(site config.Site) string {
	if text := site.ExtString(); text != "" {
		return text
	}
	if site.Ext == nil {
		return ""
	}
	raw, err := json.Marshal(site.Ext)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (p *Spider) ID() string { return p.site.Key }

// ScriptURL 返回该站点实际要加载的脚本地址（按 spiderScriptURL 的选源规则得出）。
func (p *Spider) ScriptURL() string { return spiderScriptURL(p.site) }

// spiderScriptURL 选择真正要加载的脚本：
// drpy2 站点的 api 指向引擎库，站点规则在 ext 里；其余情况用 api。
// ext 的 query/fragment 不参与后缀判断，但返回时保留原始 ext。
func spiderScriptURL(site config.Site) string {
	api := strings.TrimSpace(site.API)
	ext := site.ExtString()
	if !isJSScriptURL(ext) {
		return api
	}
	if isDrpyEngineLib(api) {
		return ext
	}
	return api
}

// isJSScriptURL 判断 ext 是否指向 .js（忽略 ?query / #fragment）。
func isJSScriptURL(raw string) bool {
	base := strings.TrimSpace(raw)
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	return strings.HasSuffix(strings.ToLower(base), ".js")
}

// isDrpyEngineLib 判断地址是否指向 drpy/drpy2 引擎库。
// 真实配置里除 drpy2.min.js / drpy.min.js 外，还有 drpy2.1.min.js、
// drpy2.min.js?v=1 等形态，统一按「drpy 前缀 + .js 后缀」识别。
func isDrpyEngineLib(raw string) bool {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	base := strings.ToLower(path.Base(raw))
	return strings.HasPrefix(base, "drpy") && strings.HasSuffix(base, ".js")
}

func (p *Spider) ensureLoaded(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return p.loadErr
	}
	p.loadErr = p.engine.LoadFromURL(ctx, spiderScriptURL(p.site))
	p.loaded = true
	return p.loadErr
}

func (p *Spider) Home(ctx context.Context) ([]provider.Section, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.engine.Init(); err != nil {
		return nil, err
	}
	classes, classesErr := p.engine.VodClasses()
	if classesErr == nil && len(classes) > 0 {
		sections := make([]provider.Section, 0, len(classes))
		for _, class := range classes {
			if strings.TrimSpace(class.TypeID) == "" || strings.TrimSpace(class.TypeName) == "" {
				continue
			}
			sections = append(sections, provider.Section{ID: class.TypeID, Title: class.TypeName})
		}
		if len(sections) > 0 {
			return p.attachFilters(sections), nil
		}
	}
	rule, err := p.engine.Rule()
	if err == nil && strings.TrimSpace(rule.ClassName) != "" {
		names := strings.Split(rule.ClassName, "&")
		ids := strings.Split(rule.ClassURL, "&")
		sections := make([]provider.Section, 0, len(names))
		for i, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id := name
			if i < len(ids) && strings.TrimSpace(ids[i]) != "" {
				id = strings.TrimSpace(ids[i])
			}
			sections = append(sections, provider.Section{ID: id, Title: name})
		}
		return p.attachFilters(sections), nil
	}
	vods, err := p.engine.VodHome()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	sections := make([]provider.Section, 0)
	for _, vod := range vods {
		name := strings.TrimSpace(vod.TypeName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		sections = append(sections, provider.Section{ID: name, Title: name})
	}
	// 分类接口失败且 rule/home 回退都没产出可用分类时，把原始错误抛给上层；
	// 否则调用方只会看到空分类，误以为站点没有分类。
	if len(sections) == 0 && classesErr != nil {
		return nil, classesErr
	}
	return p.attachFilters(sections), nil
}

// attachFilters 从 rule.filter / rule.filter_def 为每个分类挂载筛选组。
// 真实脚本两种方言并存：rule.filter（FongMi 数组形态，带可选列表）优先，
// filter_def 的字典形态作回退；都没有时该分类无筛选。
func (p *Spider) attachFilters(sections []provider.Section) []provider.Section {
	rule, err := p.engine.Rule()
	if err != nil {
		return sections
	}
	for i := range sections {
		sections[i].Filters = filterGroupsForTid(rule, sections[i].ID)
	}
	return sections
}

// filterGroupsForTid 取某个分类的筛选组：rule.Filter[tid]（数组或字典）优先，
// rule.FilterDef[tid] 字典回退；无定义返回 nil。
func filterGroupsForTid(rule *crawler.Rule, tid string) []provider.FilterGroup {
	if rule != nil {
		if raw, ok := rule.Filter[tid]; ok {
			switch v := raw.(type) {
			case []any:
				if g := filterGroupsFromFongmi(v); len(g) > 0 {
					return g
				}
			case map[string]any:
				if g := filterGroupsFromFilterDef(v); len(g) > 0 {
					return g
				}
			}
		}
		if raw, ok := rule.FilterDef[tid].(map[string]any); ok {
			if g := filterGroupsFromFilterDef(raw); len(g) > 0 {
				return g
			}
		}
	}
	return nil
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

func (p *Spider) Search(ctx context.Context, q string) ([]provider.Item, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.engine.Init(); err != nil {
		return nil, err
	}
	items, err := p.engine.VodSearch(q)
	if err != nil {
		return nil, err
	}
	return spiderItems(items), nil
}

func (p *Spider) Detail(ctx context.Context, id string) (provider.Media, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return provider.Media{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.engine.Init(); err != nil {
		return provider.Media{}, err
	}
	detail, err := p.engine.VodDetail(id)
	if err != nil {
		return provider.Media{}, err
	}
	sources := splitSources(detail.VodPlayFrom)
	episodes := parseEpisodes(id, detail.VodPlayURL, sources)
	for _, episode := range episodes {
		p.cache[episode.ID] = episode
	}
	return provider.Media{
		ID: id, Title: detail.VodName, Logo: detail.VodPic, Group: detail.TypeName,
		Description: detail.VodContent, Year: detail.VodYear, Area: detail.VodArea,
		Type: detail.TypeName, Remarks: detail.VodRemarks, Sources: sources, Episodes: episodes,
	}, nil
}

func (p *Spider) Resolve(ctx context.Context, epID string) (player.Stream, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return player.Stream{}, err
	}
	p.mu.Lock()
	episode, ok := p.cache[epID]
	p.mu.Unlock()
	if !ok {
		vodID, _, _, err := parseEpID(epID)
		if err != nil {
			return player.Stream{}, err
		}
		if _, err := p.Detail(ctx, vodID); err != nil {
			return player.Stream{}, err
		}
		p.mu.Lock()
		episode, ok = p.cache[epID]
		p.mu.Unlock()
		if !ok {
			return player.Stream{}, fmt.Errorf("剧集不存在: %s", epID)
		}
	}
	p.mu.Lock()
	playURL, err := p.engine.VodPlay(episode.Source, episode.URL)
	p.mu.Unlock()
	if err == nil && strings.TrimSpace(playURL) != "" {
		episode.URL = playURL
	}
	return player.Stream{URL: episode.URL, Kind: kindForURL(episode.URL), Headers: map[string]string{"Referer": originOf(spiderScriptURL(p.site))}}, nil
}

func spiderItems(vods []crawler.Vod) []provider.Item {
	items := make([]provider.Item, 0, len(vods))
	for _, vod := range vods {
		items = append(items, provider.Item{ID: vod.VodID.String(), Title: vod.VodName, Logo: vod.VodPic, Group: firstNonEmpty(vod.TypeName, vod.VodRemarks)})
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
