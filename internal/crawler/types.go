package crawler

import (
	"encoding/json"
	"strings"
)

// Vod 是爬虫返回的影片条目（drpy vod_* 字段对齐）。
type Vod struct {
	VodID      flexID `json:"vod_id"`
	VodName    string `json:"vod_name"`
	VodPic     string `json:"vod_pic"`
	TypeName   string `json:"type_name"`
	VodRemarks string `json:"vod_remarks"`
	VodContent string `json:"vod_content"`
}

// flexID 兼容爬虫返回数字或字符串形式的 id。
//
// 真实脚本里 vod_id 两种形态都常见；用 string 硬解时只要有一个数字，
// 整个列表就会报 `json: cannot unmarshal number into Go struct field
// Vod.list.vod_id of type string`，同批其它条目一起丢掉。
type flexID string

// String 返回 id 文本。
func (f flexID) String() string { return string(f) }

// UnmarshalJSON 同时接受 JSON 数字与字符串；null / 缺失视为空。
func (f *flexID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = ""
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*f = flexID(strings.TrimSpace(str))
		return nil
	}
	*f = flexID(s)
	return nil
}

// Class 是爬虫首页返回的分类。
type Class struct {
	TypeID   string `json:"type_id"`
	TypeName string `json:"type_name"`
}

// Detail 是爬虫返回的详情。
type Detail struct {
	Vod
	VodYear     string `json:"vod_year"`
	VodArea     string `json:"vod_area"`
	VodPlayFrom string `json:"vod_play_from"`
	VodPlayURL  string `json:"vod_play_url"`
}

// Rule 是声明式 drpy rule 的常用字段。
type Rule struct {
	Title      string `json:"title"`
	Host       string `json:"host"`
	HomeURL    string `json:"homeUrl"`
	SearchURL  string `json:"searchUrl"`
	DetailURL  string `json:"detailUrl"`
	PlayURL    string `json:"playUrl"`
	URL        string `json:"url"`
	ClassParse string `json:"class_parse"`
	// FilterURL 是 drpy2 的筛选查询片段模板（如 "&area={{fl.area}}&year={{fl.year}}"）。
	// 分类 URL 里的 fyfilter 占位会被它替换。真实配置里大量存在，不展开就会把
	// 字面量 "fyfilter" 发到站点（params=fyfilter）。
	FilterURL string `json:"filter_url"`
	// FilterDef 是按分类 id 给出的筛选默认值，用于渲染 FilterURL 里的 {{fl.xxx}}。
	FilterDef             map[string]any    `json:"filter_def"`
	Lazy                  string            `json:"lazy"`
	Headers               map[string]string `json:"headers"`
	Timeout               int64             `json:"timeout"`
	ClassName             string            `json:"class_name"`
	ClassURL              string            `json:"class_url"`
	VodSelector           string            `json:"vod_selector"`
	NameSelector          string            `json:"name_selector"`
	PicSelector           string            `json:"pic_selector"`
	TypeSelector          string            `json:"type_selector"`
	RemarksSelector       string            `json:"remarks_selector"`
	IDSelector            string            `json:"id_selector"`
	DetailNameSelector    string            `json:"detail_name_selector"`
	DetailContentSelector string            `json:"detail_content_selector"`
	DetailYearSelector    string            `json:"detail_year_selector"`
	DetailAreaSelector    string            `json:"detail_area_selector"`
	PlaySelector          string            `json:"play_selector"`
	PlayNameSelector      string            `json:"play_name_selector"`
	PlayURLSelector       string            `json:"play_url_selector"`
}
