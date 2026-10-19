package provider

import (
	"context"
	"testing"

	"github.com/unbox/unbox/internal/player"
)

// 编译期接口完整性：实现方必须提供 BrowseFiltered，Browse 行为保持。
type stubProvider struct{}

func (stubProvider) ID() string                              { return "" }
func (stubProvider) Home(context.Context) ([]Section, error) { return nil, nil }
func (p stubProvider) Browse(ctx context.Context, cat string, page int) (Page, error) {
	return p.BrowseFiltered(ctx, cat, page, nil)
}
func (stubProvider) BrowseFiltered(ctx context.Context, cat string, page int, _ map[string]string) (Page, error) {
	return Page{}, nil
}
func (stubProvider) Search(context.Context, string) ([]Item, error) { return nil, nil }
func (stubProvider) Detail(context.Context, string) (Media, error)  { return Media{}, nil }
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
