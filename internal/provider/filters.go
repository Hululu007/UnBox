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
