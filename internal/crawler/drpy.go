package crawler

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func fillURL(tmpl, tid string, pg int) string {
	tmpl = strings.ReplaceAll(tmpl, "fyclass", tid)
	tmpl = strings.ReplaceAll(tmpl, "fypage", strconv.Itoa(pg))
	return strings.ReplaceAll(tmpl, "**", url.QueryEscape(tid))
}

// flPlaceholderRe 匹配 drpy2 筛选模板里的 {{fl.key}} / {{ fl.key }}。
var flPlaceholderRe = regexp.MustCompile(`\{\{\s*fl\.([\p{L}\p{N}_]+)\s*\}\}`)

// applyFilterTemplate 复刻 drpy2 categoryParse 的 filter_url 展开：
//
//	if (rule.filter_url) {
//	  url 含 fyfilter ? 用 filter_url 替换 : 追加 filter_url
//	  url = jinja2(url, {fl: filter_def[tid] 合并用户选择})
//	}
//
// 未实现前端筛选选择，因此这里只用 filter_def 里该分类的默认值渲染；
// 缺少的键按 jinja2 语义渲染成空串。不做这一步，规则会把字面量
// "fyfilter" 直接发到站点（例如 params=fyfilter），请求必然失败。
func applyFilterTemplate(path, tid string, rule *Rule) string {
	if rule == nil {
		return path
	}
	filterURL := strings.TrimSpace(rule.FilterURL)
	if filterURL == "" {
		return path
	}
	if strings.Contains(path, "fyfilter") {
		path = strings.Replace(path, "fyfilter", filterURL, 1)
	} else {
		if !strings.HasSuffix(path, "&") && !strings.HasPrefix(filterURL, "&") {
			path += "&"
		}
		path += filterURL
	}
	path = strings.ReplaceAll(path, "fyclass", tid)
	return renderFilterTemplate(path, filterDefaults(rule, tid))
}

// filterDefaults 取出某个分类的筛选默认值。
func filterDefaults(rule *Rule, tid string) map[string]string {
	out := map[string]string{}
	if rule == nil || len(rule.FilterDef) == 0 {
		return out
	}
	raw, ok := rule.FilterDef[tid]
	if !ok {
		return out
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for key, value := range values {
		out[key] = filterValueString(value)
	}
	return out
}

// filterValueString 把筛选默认值转成查询串里可用的文本。
func filterValueString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

// renderFilterTemplate 把 {{fl.key}} 替换成对应的筛选值，缺失的键渲染为空串。
func renderFilterTemplate(tmpl string, values map[string]string) string {
	if !strings.Contains(tmpl, "{{") {
		return tmpl
	}
	return flPlaceholderRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := flPlaceholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		return values[sub[1]]
	})
}
