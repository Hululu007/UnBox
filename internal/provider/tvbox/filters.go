package tvbox

import (
	"encoding/json"

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
// 只认「name + options」的嵌套对象形态；options 为空或解析失败则该维度剔除。
// 标量默认值形态（如 {"area":"内地"}）无可选项，返回空。
func filterGroupsFromFilterDef(raw map[string]any) []provider.FilterGroup {
	return filterGroupsFromDict(raw)
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

// filterGroupsFromDict 把任意 {key:{name, options:[…]}, …} 字典组装成筛选组。
// 单个 option 无法取出显示名时跳过该选项；维度没有任何可用选项时整个剔除。
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
