package crawler

import (
	"net/url"
	"testing"
)

func TestFillURL(t *testing.T) {
	if got := fillURL("/list/fyclass--------fypage---.html", "movie", 2); got != "/list/movie--------2---.html" {
		t.Fatalf("fillURL=%q", got)
	}
	if got := fillURL("/s?wd=**&page=fypage", "斗罗", 1); got != "/s?wd="+url.QueryEscape("斗罗")+"&page=1" {
		t.Fatalf("fillURL search=%q", got)
	}
}

// drpy2 用 rule.filter_url 替换分类 URL 里的 fyfilter 占位，再用 filter_def
// 里该分类的默认值渲染 {{fl.xxx}}。不展开就会把字面量 "fyfilter" 发到站点，
// 例如 params=fyfilter、zhonglei=其他fyfilter。
func TestApplyFilterTemplate(t *testing.T) {
	rule := &Rule{
		FilterURL: "&area={{fl.area}}&year={{fl.year}}",
		FilterDef: map[string]any{
			"17": map[string]any{"area": "全部", "year": "2024"},
		},
	}

	got := applyFilterTemplate("/category/data?params=fyfilter", "17", rule)
	want := "/category/data?params=&area=全部&year=2024"
	if got != want {
		t.Fatalf("fyfilter 展开 = %q，期望 %q", got, want)
	}

	// 占位紧跟分类名（zhonglei=其他fyfilter）时替换后要拼成合法的查询串。
	got = applyFilterTemplate("/category.html?zhonglei=其他fyfilter", "17", rule)
	want = "/category.html?zhonglei=其他&area=全部&year=2024"
	if got != want {
		t.Fatalf("紧邻占位 = %q，期望 %q", got, want)
	}
}

// 缺少默认值的键按 jinja2 语义渲染成空串，而不是残留 {{fl.x}}。
func TestApplyFilterTemplateMissingDefault(t *testing.T) {
	rule := &Rule{FilterURL: "&area={{ fl.area }}"}
	if got := applyFilterTemplate("/list?fyfilter", "1", rule); got != "/list?&area=" {
		t.Fatalf("缺省值展开 = %q", got)
	}
}

// 没有 filter_url 时不做任何改写（与 drpy2 的 if(rule.filter_url) 守卫一致）。
func TestApplyFilterTemplateWithoutFilterURL(t *testing.T) {
	rule := &Rule{}
	if got := applyFilterTemplate("/list?fyfilter", "1", rule); got != "/list?fyfilter" {
		t.Fatalf("无 filter_url 不应改写，得 %q", got)
	}
	if got := applyFilterTemplate("/list", "1", nil); got != "/list" {
		t.Fatalf("rule 为 nil 时应原样返回，得 %q", got)
	}
}
