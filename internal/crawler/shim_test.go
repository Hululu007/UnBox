package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// drpy2 引擎库在自身顶部定义 PC_UA 等常量；UnBox 只加载站点规则（ext），
// 规则里直接引用 PC_UA 会报 ReferenceError，需要引擎侧补上。
func TestUAGlobalsAvailable(t *testing.T) {
	e := New()
	if err := e.Load(`function ua(){ return PC_UA }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("ua")
	if err != nil {
		t.Fatalf("PC_UA 应可用: %v", err)
	}
	if got := v.String(); got != uaPC {
		t.Fatalf("PC_UA = %q", got)
	}
	for _, name := range []string{"MOBILE_UA", "UC_UA", "IOS_UA", "UA"} {
		if e.vm.Get(name) == nil || e.vm.Get(name).String() == "" {
			t.Errorf("%s 未注入", name)
		}
	}
}

// 规则自己声明同名常量时不能被引擎注入值挡住。
func TestUAGlobalsDoNotShadowScriptDeclaration(t *testing.T) {
	e := New()
	if err := e.Load("const PC_UA = 'script-ua';\nfunction ua(){ return PC_UA }"); err != nil {
		t.Fatalf("脚本声明同名常量不应报错: %v", err)
	}
	v, err := e.Call("ua")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "script-ua" {
		t.Fatalf("脚本内声明应生效，得 %q", got)
	}
}

// gbk.js 上游是 GPL-3.0，UnBox 用 Go 的 GBK 实现提供同名 shim。
func TestGBKToolShim(t *testing.T) {
	e := New()
	src := `import { gbkTool } from "assets://js/lib/gbk.js";
export function enc(s){ return gbkTool().encode(s); }
export function dec(s){ return gbkTool().decode(s); }`
	if err := e.loadWithContext(context.Background(), src, "https://x/js/site.js"); err != nil {
		t.Fatalf("gbk.js 应可加载: %v", err)
	}

	enc, err := e.Call("enc", e.vm.ToValue("你好"))
	if err != nil {
		t.Fatal(err)
	}
	if got := enc.String(); got != "%C4%E3%BA%C3" {
		t.Fatalf("GBK 编码 = %q", got)
	}
	dec, err := e.Call("dec", e.vm.ToValue(enc.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := dec.String(); got != "你好" {
		t.Fatalf("GBK 解码 = %q", got)
	}
}

// 真实脚本会直接读 extend.area；传 null 会抛
// `Cannot read property 'area' of undefined`。
func TestVodCategoryPassesExtendObject(t *testing.T) {
	e := New()
	src := `function category(tid, pg, filter, extend){
  // 真实脚本会读 extend.area；这里先确认 extend 本身是对象而不是 null。
  var area = extend.area;
  return JSON.stringify({list:[{vod_id: tid, vod_name: typeof extend + ":" + typeof area}]});
}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	vods, err := e.VodCategory("movie", 1)
	if err != nil {
		t.Fatalf("category 不应因 extend 为 null 报错: %v", err)
	}
	if len(vods) != 1 || vods[0].VodName != "object:undefined" {
		t.Fatalf("vods = %+v", vods)
	}
}

// FongMi js0 的 init(ext) 需要一个参数；不传会让脚本里的
// ext.match(...) / ext.indexOf(...) 抛 TypeError。
func TestInitReceivesExt(t *testing.T) {
	e := New()
	e.SetExt("18+")
	src := `var seen = "";
function init(ext){ seen = typeof ext + ":" + (ext.match(/18\+/) ? "hit" : "miss"); }`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	if err := e.Init(); err != nil {
		t.Fatalf("init 不应因缺少参数报错: %v", err)
	}
	if got := e.vm.Get("seen").String(); got != "string:hit" {
		t.Fatalf("init 收到的 ext = %q", got)
	}
}

// cat 系脚本的 init(cfg) 读 cfg.skey / cfg.stype / cfg.ext.hasOwnProperty。
func TestInitAcceptsConfigObject(t *testing.T) {
	e := New()
	e.SetExt(`{"categories":"短剧#沙雕","cookie":"c=1"}`)
	e.SetSiteMeta("bili", 3)
	src := `var got = "";
function init(cfg){
  got = cfg.skey + "|" + cfg.stype + "|" + cfg.ext.categories + "|" + cfg.ext.hasOwnProperty("cookie");
}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	if err := e.Init(); err != nil {
		t.Fatalf("init(cfg) 不应报错: %v", err)
	}
	if got := e.vm.Get("got").String(); got != "bili|3|短剧#沙雕|true" {
		t.Fatalf("init(cfg) 收到 %q", got)
	}
}

// json: 规则拿到 HTML 时给出可定位提示，而不是 json 包的原始语法错误。
func TestExtractJSONVodsRejectsHTML(t *testing.T) {
	_, err := extractJSONVods("<!DOCTYPE html><html><body>404</body></html>", "json:data.list;vod_id")
	if err == nil {
		t.Fatal("HTML 响应应报错")
	}
	if !contains(err.Error(), "网页而不是 JSON") {
		t.Fatalf("错误应说明返回了网页，实际: %v", err)
	}
}

// 真实脚本会把 vod_id 返回成数字；用 string 硬解会让整批列表解析失败。
func TestVodAcceptsNumericID(t *testing.T) {
	var payload struct {
		List []Vod `json:"list"`
	}
	raw := `{"list":[{"vod_id":79337,"vod_name":"数字"},{"vod_id":"tv-1","vod_name":"字符串"},{"vod_id":null,"vod_name":"空"}]}`
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("数字 id 不应导致解析失败: %v", err)
	}
	if len(payload.List) != 3 {
		t.Fatalf("应解析 3 条，得 %d", len(payload.List))
	}
	if got := payload.List[0].VodID.String(); got != "79337" {
		t.Errorf("数字 id = %q", got)
	}
	if got := payload.List[1].VodID.String(); got != "tv-1" {
		t.Errorf("字符串 id = %q", got)
	}
	if got := payload.List[2].VodID.String(); got != "" {
		t.Errorf("null id = %q", got)
	}
}

// http(url, opts) 是 FongMi 脚本的取正文原语，直接返回 content 字符串。
func TestHTTPGlobalReturnsContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("via-http"))
	}))
	defer srv.Close()

	e := New()
	src := fmt.Sprintf(`function go(){ return http(%q) }`, srv.URL)
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("go")
	if err != nil {
		t.Fatalf("http 应可用: %v", err)
	}
	if got := v.String(); got != "via-http" {
		t.Fatalf("http() = %q", got)
	}
}

// 缺少 rule 且没有动作函数时，错误要说明是在哪个动作上缺的。
func TestNoRuleErrorNamesAction(t *testing.T) {
	e := New()
	if err := e.Load(`function unrelated(){} `); err != nil {
		t.Fatal(err)
	}
	_, err := e.VodCategory("movie", 1)
	if err == nil || !contains(err.Error(), "分类动作") {
		t.Fatalf("应指出缺少分类动作，实际: %v", err)
	}
}

// cat.js 上游是 GPL-3.0；自有 shim 提供 Crypto / _ / jinja2 三件套。
func TestCatJSSShim(t *testing.T) {
	e := New()
	src := `import { Crypto, _, jinja2 } from "assets://js/lib/cat.js";
export function b64(s){ return Crypto.enc.Base64.stringify(Crypto.enc.Utf8.parse(s)); }
export function empty(v){ return _.isEmpty(v); }
export function kbps(v){ return _.floor(v / 1024); }
export function sorted(json){ return JSON.stringify(_.sortBy(JSON.parse(json), "bit")); }
export function render(tpl, json){ return jinja2(tpl, JSON.parse(json)); }
export function loop(){ return jinja2("{% for x in list %}{{x}}/{% endfor %}", {list:[1,2,3]}); }
export function cond(){ return jinja2("{% if fl.on %}yes{% else %}no{% endif %}", {fl:{on:false}}); }`
	if err := e.loadWithContext(context.Background(), src, "https://x/cat/js/site.js"); err != nil {
		t.Fatalf("cat.js 应可加载: %v", err)
	}

	if v, err := e.Call("b64", e.vm.ToValue("你好")); err != nil || v.String() != "5L2g5aW9" {
		t.Fatalf("Crypto base64 = %v, %v", v, err)
	}
	if v, err := e.Call("empty", e.vm.ToValue([]any{})); err != nil || !v.ToBoolean() {
		t.Fatalf("_.isEmpty = %v, %v", v, err)
	}
	if v, err := e.Call("kbps", e.vm.ToValue(1500)); err != nil || v.ToInteger() != 1 {
		t.Fatalf("_.floor = %v, %v", v, err)
	}
	sorted, err := e.Call("sorted", e.vm.ToValue(`[{"bit":3},{"bit":1}]`))
	if err != nil || sorted.String() != `[{"bit":1},{"bit":3}]` {
		t.Fatalf("_.sortBy = %q, %v", sorted, err)
	}
	rendered, err := e.Call("render", e.vm.ToValue("{{fl.area}}"), e.vm.ToValue(`{"fl":{"area":"全部"}}`))
	if err != nil || rendered.String() != "全部" {
		t.Fatalf("jinja2 插值 = %q, %v", rendered, err)
	}
	if v, err := e.Call("loop"); err != nil || v.String() != "1/2/3/" {
		t.Fatalf("jinja2 for = %q, %v", v, err)
	}
	if v, err := e.Call("cond"); err != nil || v.String() != "no" {
		t.Fatalf("jinja2 if = %q, %v", v, err)
	}
}

// TVBox 脚本会拼出 127.0.0.1:<port> 的本地服务地址，报错要说清楚原因。
func TestExplainLoopbackRefused(t *testing.T) {
	err := explainRequestError("http://127.0.0.1:10079/c/3600/proxy/https://x/y.m3u",
		errors.New("dial tcp 127.0.0.1:10079: connect: connection refused"))
	if err == nil || !contains(err.Error(), "本地代理/文件服务") {
		t.Fatalf("应说明是本地服务，实际: %v", err)
	}

	// 外部地址不受影响。
	plain := errors.New("dial tcp 1.2.3.4:443: connect: connection refused")
	if got := explainRequestError("https://example.com/x", plain); got.Error() != plain.Error() {
		t.Fatalf("外部地址不应改写，实际: %v", got)
	}
}
