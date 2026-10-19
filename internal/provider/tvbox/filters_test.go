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

func TestFilterGroupsFromCMSFilter(t *testing.T) {
	got := filterGroupsFromCMSFilter([]byte(`{"area":{"name":"地区","options":[{"n":"全部","v":""},{"n":"内地","v":"内地"}]}}`))
	if len(got) != 1 || got[0].Key != "area" || got[0].Options[1].Name != "内地" {
		t.Fatalf("CMS filter 解析错误: %+v", got)
	}
	for _, junk := range []string{"", "null", "[]", "{}", "not json"} {
		if g := filterGroupsFromCMSFilter([]byte(junk)); len(g) != 0 {
			t.Fatalf("畸形 filter %q 应返回空，实际 %+v", junk, g)
		}
	}
}

// option 的 n/v 出现异常（缺 v / 非字符串）时安全降级，不 panic。
func TestFilterGroupsTolerantOfOddOptions(t *testing.T) {
	raw := map[string]any{
		"bad": map[string]any{
			"name":    "怪维度",
			"options": []any{42, "x", map[string]any{"v": "only-value"}, map[string]any{"n": 7}},
		},
	}
	if got := filterGroupsFromFilterDef(raw); len(got) != 0 {
		t.Fatalf("畸形 options 应整体剔除，实际 %+v", got)
	}
}
