package tvbox

import (
	"encoding/json"
	"sort"

	"github.com/unbox/unbox/internal/provider"
)

// optionText 兼容 option 字段的两种写法：显示名优先 n/name，提交值优先 v/value。
func optionText(opt map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := opt[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// filterGroupsFromFilterDef 组装 drpy2 rule.filter_def 某分类的筛选组。
// options 为空或解析失败则该维度剔除；标量默认值形态（如 {"area":"内地"}）
// 无可选项，返回空。
func filterGroupsFromFilterDef(raw map[string]any) []provider.FilterGroup {
	return filterGroupsFromDict(raw)
}

// filterGroupsFromCMSFilter 解析 maccms class 条目里的 filter/filters 字段。
// 畸形 / 空内容一律返回 nil（不阻断分类浏览）。
// 兼容两种形态：{"fid":{name,options}} 字典（filterGroupsFromDict），
// 以及 [{key,name,value:[{n,v}]}] 数组（filterGroupsFromFongmi）。
func filterGroupsFromCMSFilter(raw json.RawMessage) []provider.FilterGroup {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" || string(raw) == "{}" {
		return nil
	}
	if raw[0] == '[' {
		var entries []any
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil
		}
		return filterGroupsFromFongmi(entries)
	}
	var dict map[string]any
	if err := json.Unmarshal(raw, &dict); err != nil {
		return nil
	}
	return filterGroupsFromDict(dict)
}

// filterGroupsFromFongmi 组装 FongMi 数组方言筛选组：每条 {key,name,value:[{n,v}]}。
// 真实 drpy2 脚本（如 9udy / 苹果CMS 类站点）的 rule.filter 就是这种形态，
// 只有带可选列表的维度才暴露；畸形条目跳过。
func filterGroupsFromFongmi(entries []any) []provider.FilterGroup {
	var out []provider.FilterGroup
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key := optionText(entry, "key")
		if key == "" {
			continue
		}
		name := optionText(entry, "name", "n")
		rawOpts, ok := entry["value"].([]any)
		if !ok || len(rawOpts) == 0 {
			continue
		}
		opts := filterOptions(rawOpts)
		if len(opts) == 0 {
			continue
		}
		if name == "" {
			name = key
		}
		out = append(out, provider.FilterGroup{Key: key, Name: name, Options: opts})
	}
	return out
}

// filterGroupsFromDict 把 {维度 key: 定义} 字典组装成筛选组。
//
// 字典里的定义有三种真实形态，都要认：
//
//	{"area": {"name":"地区","options":[{"n","v"}…]}}   // 最常见
//	{"area": {"n":"地区","value":[{"n","v"}…]}}        // FongMi 条目的字典写法
//	{"area": [{"n","v"}…]}                             // maccms 常见：value 直接是数组
//
// 键顺序按字典 key 排序：Go map 的遍历顺序是随机的，不排序会让前端每次
// 重载筛选项的顺序都变。没有维度名时回退到 key，避免渲染出空标签。
func filterGroupsFromDict(raw map[string]any) []provider.FilterGroup {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]provider.FilterGroup, 0, len(keys))
	for _, key := range keys {
		group, ok := filterGroupFromDictEntry(key, raw[key])
		if ok {
			out = append(out, group)
		}
	}
	return out
}

// filterGroupFromDictEntry 解析单个维度的定义。
func filterGroupFromDictEntry(key string, value any) (provider.FilterGroup, bool) {
	var name string
	var rawOpts []any
	switch entry := value.(type) {
	case map[string]any:
		name = optionText(entry, "name", "n")
		if opts, ok := entry["options"].([]any); ok {
			rawOpts = opts
		} else if opts, ok := entry["value"].([]any); ok {
			rawOpts = opts
		}
	case []any:
		rawOpts = entry
	default:
		// 标量默认值（{"area":"内地"}）没有可选列表，不暴露筛选。
		return provider.FilterGroup{}, false
	}

	opts := filterOptions(rawOpts)
	if len(opts) == 0 {
		return provider.FilterGroup{}, false
	}
	if name == "" {
		name = key
	}
	return provider.FilterGroup{Key: key, Name: name, Options: opts}, true
}

// filterOptions 把选项数组转成统一的 FilterOption。
// 单个选项既可以是 {n,v} 对象，也可以是纯字符串（显示名与提交值相同）。
func filterOptions(rawOpts []any) []provider.FilterOption {
	opts := make([]provider.FilterOption, 0, len(rawOpts))
	for _, raw := range rawOpts {
		switch opt := raw.(type) {
		case map[string]any:
			display := optionText(opt, "n", "name")
			value := optionText(opt, "v", "value")
			if display == "" {
				if value == "" {
					continue
				}
				display = value
			}
			if value == "" {
				value = display
			}
			opts = append(opts, provider.FilterOption{Name: display, Value: value})
		case string:
			if opt == "" {
				continue
			}
			opts = append(opts, provider.FilterOption{Name: opt, Value: opt})
		}
	}
	return opts
}
