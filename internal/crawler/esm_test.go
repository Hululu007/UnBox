package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestMaskNonCode(t *testing.T) {
	src := "const a = 'import x from \"y\"';\n// import z\n/* export const q */\nconst b = `export default`;\nimport real from './r.js';"
	masked := maskNonCode(src)
	if len(masked) != len(src) {
		t.Fatalf("长度变化: %d != %d", len(masked), len(src))
	}
	if contains(masked, "import x") || contains(masked, "import z") || contains(masked, "export const q") || contains(masked, "export default") {
		t.Fatalf("字符串/注释未掩码:\n%s", masked)
	}
	if !contains(masked, "import real") {
		t.Fatalf("真实 import 被误掩码:\n%s", masked)
	}
}

func TestNeedsModuleLoader(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"var rule = {host:'x'};", false},
		{"export default { home(){} };", false},
		{"import x from './x.js';", true},
		{"import './x.js';", true},
		{"import{a}from'./m.js';", true},
		{"import*as n from'./m.js';", true},
		{"import('./m.js');", true},
		{"obj.import('./m.js');", false},
		{"export function __jsEvalReturn(){ return {} };", true},
		{"export const a = 1;", true},
		{"export { a };", true},
		{"export{a};", true},
		{"const s = 'import x';", false},
		{"const important = 1;", false},
	}
	for _, c := range cases {
		if got := needsModuleLoader(c.src); got != c.want {
			t.Errorf("needsModuleLoader(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

func TestRewriteModuleImports(t *testing.T) {
	vm := goja.New()
	src := `
import def from "./a.js";
import { x, y as z } from "./b.js";
import * as ns from "./c.js";
import "./d.js";
`
	got, err := rewriteModule(vm, src, "https://x/app/main.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`__module("https://x/app/a.js")`,
		`__module("https://x/app/b.js")`,
		`__module("https://x/app/c.js")`,
		`__module("https://x/app/d.js")`,
	} {
		if !contains(got, want) {
			t.Fatalf("缺少 %q:\n%s", want, got)
		}
	}
	if contains(got, "import ") {
		t.Fatalf("import 残留:\n%s", got)
	}
}

func TestRewriteModuleExports(t *testing.T) {
	vm := goja.New()
	src := `export function __jsEvalReturn(){ return { home(){} } }
export const a = 1;
const b = 2;
export { b as c };`
	got, err := rewriteModule(vm, src, "https://x/m.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"function __jsEvalReturn", "__exports.__jsEvalReturn = __jsEvalReturn;",
		"const a = 1;", "__exports.a = a;", "__exports.c = b;",
	} {
		if !contains(got, want) {
			t.Fatalf("缺少 %q:\n%s", want, got)
		}
	}
	if contains(got, "export ") {
		t.Fatalf("export 残留:\n%s", got)
	}
}

// 真实脚本常见 `export async function home(){...}`；只匹配 `export function`
// 会漏改，goja 报 `Unexpected reserved word`。
func TestRewriteModuleExportAsyncFunction(t *testing.T) {
	vm := goja.New()
	got, err := rewriteModule(vm, `export async function home(){ return 1 }`, "https://x/m.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"function home", "__exports.home = home;"} {
		if !contains(got, want) {
			t.Fatalf("缺少 %q:\n%s", want, got)
		}
	}
	if contains(got, "export ") || contains(got, "export{") {
		t.Fatalf("export 残留:\n%s", got)
	}
}

// 端到端验证 `export async function` 能被 goja 编译并调用。
func TestLoadESMExportAsyncFunction(t *testing.T) {
	e := New()
	if err := e.Load(`export async function home(){ return 1 }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil || v.ToInteger() != 1 {
		t.Fatalf("home() = %v, %v", v, err)
	}
}

func TestRewriteModuleExportDefault(t *testing.T) {
	vm := goja.New()
	got, err := rewriteModule(vm, `export default { home(){} }`, "https://x/m.js")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "__exports.default =") {
		t.Fatalf("缺少 __exports.default 赋值:\n%s", got)
	}
	if contains(got, "export ") {
		t.Fatalf("export 残留:\n%s", got)
	}
}

// 压缩脚本里 export default 后无空格（export default{...}），须同样改写。
func TestRewriteModuleExportDefaultNoSpace(t *testing.T) {
	vm := goja.New()
	got, err := rewriteModule(vm, `export default{'init':1,'home':2}`, "https://x/m.js")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "__exports.default =") || contains(got, "export default") {
		t.Fatalf("无空格的 export default 未改写:\n%s", got)
	}
}

func TestRewriteModuleRejections(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"export star", `export * from "https://x/y.js";`},
		{"export star as", `export * as ns from "https://x/y.js";`},
		{"export list from", `export { a } from "https://x/y.js";`},
		{"export class", `export class X {}`},
		{"import meta", `const u = import.meta.url;`},
		{"dynamic import", `import("./x.js");`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vm := goja.New()
			if _, err := rewriteModule(vm, c.src, "https://x/m.js"); !errors.Is(err, ErrESModuleUnsupported) {
				t.Fatalf("rewriteModule(%q) err = %v, want ErrESModuleUnsupported", c.src, err)
			}
		})
	}
}

func TestRewriteModuleSpecifierPassthrough(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`import x from "assets://js/lib/x.js"`, `__module("assets://js/lib/x.js")`},
		{`import x from "https://cdn/x.js"`, `__module("https://cdn/x.js")`},
	}
	for _, c := range cases {
		vm := goja.New()
		got, err := rewriteModule(vm, c.src, "https://x/m.js")
		if err != nil {
			t.Fatalf("rewriteModule(%q): %v", c.src, err)
		}
		if !contains(got, c.want) {
			t.Fatalf("缺少 %q:\n%s", c.want, got)
		}
	}
}

func TestRewriteModuleRelativeWithoutBaseURL(t *testing.T) {
	vm := goja.New()
	if _, err := rewriteModule(vm, `import x from "./x.js";`, ""); err == nil {
		t.Fatal("相对导入缺少 base URL 时应报错")
	}
}

func TestRewriteModuleExportVarKinds(t *testing.T) {
	vm := goja.New()
	got, err := rewriteModule(vm, `export let a = 1; export var b = 2;`, "https://x/m.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"let a = 1;", "var b = 2;", "__exports.a = a;", "__exports.b = b;"} {
		if !contains(got, want) {
			t.Fatalf("缺少 %q:\n%s", want, got)
		}
	}
	if contains(got, "export ") {
		t.Fatalf("export 残留:\n%s", got)
	}
}

func TestAssetSource(t *testing.T) {
	for _, u := range []string{
		"assets://js/lib/cheerio.min.js",
		"https://x/lib/cheerio.min.js",
		"https://x/js/crypto-js.js",
	} {
		if src, ok := assetSource(u); !ok || src == "" {
			t.Errorf("assetSource(%q) 未命中", u)
		}
	}
	if _, ok := assetSource("assets://js/lib/cat.js"); ok {
		t.Fatal("cat.js 不应内置")
	}
}

func TestLoadESMWithRelativeModule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dep.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`export default "dep";`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import dep from "./dep.js";
export function __jsEvalReturn(){ return { home(){ return dep } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "dep" {
		t.Fatalf("home() = %q", v.String())
	}
}

func TestLoadESMIIFEIsolation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`const C = "a"; export default C;`))
	})
	mux.HandleFunc("/b.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`const C = "b"; export default C;`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import a from "./a.js";
import b from "./b.js";
export function __jsEvalReturn(){ return { home(){ return a+b } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "ab" {
		t.Fatalf("home() = %q", v.String())
	}
}

func TestLoadESMUmdModule(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/umd.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`(function(root,factory){
  if (typeof exports === "object") { module.exports = factory(); }
  else { root.UmdValue = factory(); }
})(this, function(){ return { v: 42 }; });`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import u from "./umd.js";
export function __jsEvalReturn(){ return { home(){ return String(u.v) } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "42" {
		t.Fatalf("home() = %q", v.String())
	}
}

// 同一 UMD/CJS 模块被多个模块 import 时，缓存必须保存执行后的最终导出；
// 否则第二次 import 拿到的是执行前的空 exports 对象。
func TestLoadESMRepeatedUMDImport(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/umd.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`(function(root,factory){
  if (typeof exports === "object") { module.exports = factory(); }
  else { root.UmdValue = factory(); }
})(this, function(){ return { v: 42 }; });`))
	})
	mux.HandleFunc("/mid.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import u from "./umd.js";
export default { getU(){ return String(u.v) } }`))
	})
	mux.HandleFunc("/main.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`import mid from "./mid.js";
import u from "./umd.js";
export function __jsEvalReturn(){ return { home(){ return u.v+"|"+mid.getU() } } }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := New()
	if err := e.LoadFromURL(context.Background(), srv.URL+"/main.js"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "42|42" {
		t.Fatalf("home() = %q，重复 import 的 UMD 模块缓存有误", v.String())
	}
}

// drpyS 风格脚本把 `__jsEvalReturn` 放在 `export default` 对象里。
// 非 ESM 路径（normalizeModuleSource）已支持；ESM 路径也须调用它，
// 否则动作全部静默丢失。
func TestLoadESMNestedJsEvalReturn(t *testing.T) {
	e := New()
	src := `export default { __jsEvalReturn(){ return { home(){ return "ok" } } } }`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil || v.String() != "ok" {
		t.Fatalf("home() = %v, %v", v, err)
	}
}

// 带命名导出时走 ESM 改写路径，嵌套的 `__jsEvalReturn` 同样要提升。
func TestLoadESMNestedJsEvalReturnWithNamedExport(t *testing.T) {
	e := New()
	src := `export const marker = 1;
export default { __jsEvalReturn(){ return { home(){ return "ok"+marker } } } }`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil || v.String() != "ok1" {
		t.Fatalf("home() = %v, %v", v, err)
	}
}

func TestMaskNonCodeRegexLiteral(t *testing.T) {
	src := `import dep from "./dep.js"; const re = /^\/\//m; const x = "import fake"; export default { home(){} };`
	masked := maskNonCode(src)
	if !contains(masked, "export default") {
		t.Fatalf("正则里的 // 被当成注释，export 被掩掉:\n%s", masked)
	}
	if contains(masked, "import fake") {
		t.Fatalf("字符串内容未掩码:\n%s", masked)
	}
	if !needsModuleLoader(src) {
		t.Fatal("含 import + 正则 + export default 的脚本应走模块路径")
	}
}

func TestRewriteModuleRegexLiteral(t *testing.T) {
	vm := goja.New()
	src := `const re = /^\/\//m;
export default { home(){ return re.test("//x") } };`
	got, err := rewriteModule(vm, src, "https://x/m.js")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "__exports.default =") || contains(got, "export default") {
		t.Fatalf("正则后的 export default 未改写:\n%s", got)
	}
}

func TestMaskNonCodeDivision(t *testing.T) {
	src := `const a = b / c / d; export default { home(){} };`
	if !contains(maskNonCode(src), "export default") {
		t.Fatalf("除法被误判为正则:\n%s", maskNonCode(src))
	}
}

// 嵌套模板字面量：${} 插值内的反引号不能被当成外层模板的闭合；
// 否则插值后的 `</${n}>` 会被当代码、`/` 被误判为正则，掩掉末尾 export。
func TestMaskNonCodeNestedTemplate(t *testing.T) {
	src := "`${function(e,t){return t?\"\":`${f()}</${n}>`}(e,t)}`;\nexport{mh as contains};\n"
	masked := maskNonCode(src)
	if !contains(masked, "export{") {
		t.Fatalf("嵌套模板把末尾 export 掩掉:\n%s", masked)
	}
	if contains(masked, "import ") {
		t.Fatalf("模板内容未掩码:\n%s", masked)
	}
}

// 主模块没有 URL 时，错误信息用 "<main>" 占位，避免「模块  改写失败」双空格。
func TestLoadMainModuleErrorLabel(t *testing.T) {
	e := New()
	err := e.Load(`export * from "https://x/y.js";`)
	if err == nil {
		t.Fatal("不支持的 ESM 语法应报错")
	}
	if !strings.Contains(err.Error(), "模块 <main> 改写失败") {
		t.Fatalf("主模块错误标签不是 <main>: %v", err)
	}
	if strings.Contains(err.Error(), "模块  ") {
		t.Fatalf("错误信息出现双空格: %v", err)
	}
}

// rewriteModule / RunString 失败时，预执行缓存的占位 exports 必须删除，
// 否则同一 URL 再次加载会拿到残缺对象而不是重试。
func TestModuleLoaderCacheCleanupOnFailure(t *testing.T) {
	e := New()
	l := &moduleLoader{e: e, ctx: context.Background(), cache: map[string]goja.Value{}}

	if _, err := l.loadSource(`export * from "https://x/y.js";`, "https://x/rewrite-bad.js"); err == nil {
		t.Fatal("rewrite 失败应报错")
	}
	if _, ok := l.cache["https://x/rewrite-bad.js"]; ok {
		t.Fatal("rewrite 失败后缓存未清理")
	}

	if _, err := l.loadSource(`const = ;`, "https://x/compile-bad.js"); err == nil {
		t.Fatal("编译失败应报错")
	}
	if _, ok := l.cache["https://x/compile-bad.js"]; ok {
		t.Fatal("编译失败后缓存未清理")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
