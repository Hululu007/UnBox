package crawler

import (
	"context"
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
