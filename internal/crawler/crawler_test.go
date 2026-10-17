package crawler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	if New() == nil {
		t.Fatal("New() 返回 nil")
	}
}

func TestConsoleGlobal(t *testing.T) {
	e := New()
	if err := e.Load(`function probe(){ console.error("boom", 1); return "ok" }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("probe")
	if err != nil || v.String() != "ok" {
		t.Fatalf("probe = %v, %v", v, err)
	}
}

func TestLoadAndCall(t *testing.T) {
	e := New()
	if err := e.Load(`function home(){ return "ok" }`); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("home")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "ok" {
		t.Fatalf("home() = %q, want %q", got, "ok")
	}
}

func TestCallUndefinedFunction(t *testing.T) {
	_, err := New().Call("missing")
	if err == nil {
		t.Fatal("Call 应返回未定义函数错误")
	}
}

func TestNormalizeModuleSourceStripsAsyncAwaitAcrossWhitespace(t *testing.T) {
	src := "async function home(){\n  return await\n    req('https://example.com')\n}\n"
	got := normalizeModuleSource(src)
	for _, forbidden := range []string{"async", "await"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("normalize 后仍含 %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "function home()") {
		t.Fatalf("async function 未归约为 function:\n%s", got)
	}
}

func TestNormalizeModuleSourceRewritesExportDefault(t *testing.T) {
	src := "function home(){ return 1 }\nexport default { home }"
	got := normalizeModuleSource(src)
	if strings.Contains(got, "export default") {
		t.Fatalf("export default 残留:\n%s", got)
	}
	if !strings.Contains(got, "var __crawler_exports =") {
		t.Fatalf("export default 未改写为 __crawler_exports:\n%s", got)
	}
}

func TestStripAsyncAwait(t *testing.T) {
	got := stripAsyncAwait("async function home(){\n return await req('x')\n}")
	if strings.Contains(got, "async") || strings.Contains(got, "await") {
		t.Fatalf("async/await 未剥离:\n%s", got)
	}
	if !strings.Contains(got, "function home()") {
		t.Fatalf("函数声明被误伤:\n%s", got)
	}

	arrow := stripAsyncAwait("const f = async (x) => await g(x); const h = async x => await g(x);")
	if strings.Contains(arrow, "async") || strings.Contains(arrow, "await") {
		t.Fatalf("async 箭头函数未剥离:\n%s", arrow)
	}
}

func TestNormalizeModuleSourcePreservesIdentifiers(t *testing.T) {
	src := "const awaitable = 1; const asyncTask = 2;"
	got := normalizeModuleSource(src)
	for _, want := range []string{"awaitable", "asyncTask"} {
		if !strings.Contains(got, want) {
			t.Fatalf("标识符 %q 被误伤:\n%s", want, got)
		}
	}
}

func TestLoadReportsESModuleUnsupported(t *testing.T) {
	e := New()
	err := e.Load(`export * from "https://x/y.js";`)
	if err == nil || !errors.Is(err, ErrESModuleUnsupported) {
		t.Fatalf("不支持的 ESM 语法应报错: %v", err)
	}
}

func TestLoadStillRunsJS0Script(t *testing.T) {
	e := New()
	src := "var rule = {};\nasync function home() { return 'ok' }\nexport default { home: home }\n"
	if err := e.Load(src); err != nil {
		t.Fatalf("FongMi js0 形态脚本应能加载: %v", err)
	}
}

func TestLoadFromURLRejectsRelativePath(t *testing.T) {
	e := New()
	err := e.LoadFromURL(context.Background(), "./lib/drpy2.min.js")
	if err == nil || !strings.Contains(err.Error(), "相对路径") {
		t.Fatalf("相对路径应给出明确提示，实际: %v", err)
	}
}

func TestLoadFromURLRejectsAssetsScheme(t *testing.T) {
	e := New()
	err := e.LoadFromURL(context.Background(), "assets://js/lib/cheerio.min.js")
	if err == nil || !strings.Contains(err.Error(), "协议不支持") {
		t.Fatalf("assets:// 应给出协议提示，实际: %v", err)
	}
}

// console 参数应按空格连接，而不是依赖 fmt.Sprint 对相邻字符串不加空格的规则。
func TestConsoleArgsJoinedWithSpaces(t *testing.T) {
	buf := captureLog(t)
	e := New()
	if err := e.Load(`function probe(){ console.log("a", 1, "b"); return "ok" }`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Call("probe"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "a 1 b") {
		t.Fatalf("console 参数未按空格连接: %q", buf.String())
	}
}

// 缺少 console.time/group/trace/assert/table 等方法时，脚本会在日志路径中断；
// 这些方法应是可调用的 no-op。
func TestConsoleExtraMethodsAreNoOps(t *testing.T) {
	e := New()
	src := `function probe(){
  console.time("t"); console.timeEnd("t");
  console.group("g"); console.groupEnd(); console.groupCollapsed("c");
  console.trace("t"); console.assert(true, "ok"); console.assert(false, "fail");
  console.table([1, 2]); console.dir({a: 1}); console.count("c"); console.countReset("c");
  return "ok";
}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("probe")
	if err != nil || v.String() != "ok" {
		t.Fatalf("probe = %v, %v", v, err)
	}
}

func TestSetTimeoutCallbackErrorLogged(t *testing.T) {
	buf := captureLog(t)
	e := New()
	src := `function probe(){
  setTimeout(function(){ throw new Error("boom") }, 0);
  return "ok";
}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("probe")
	if err != nil || v.String() != "ok" {
		t.Fatalf("probe = %v, %v", v, err)
	}
	if !strings.Contains(buf.String(), "setTimeout") || !strings.Contains(buf.String(), "boom") {
		t.Fatalf("setTimeout 回调错误未记录: %q", buf.String())
	}
}

// captureLog 临时把标准日志重定向到缓冲区，返回其指针；测试结束自动还原。
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return &buf
}

func TestSetTimeoutShim(t *testing.T) {
	e := New()
	src := `function probe(){
  var x = 0;
  setTimeout(function(){ x = 1; }, 0);
  return x;
}`
	if err := e.Load(src); err != nil {
		t.Fatal(err)
	}
	v, err := e.Call("probe")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1" {
		t.Fatalf("setTimeout 未同步执行: %q", v.String())
	}
	if err := e.Load(`function probe2(){ clearTimeout(1); return "ok" }`); err != nil {
		t.Fatal(err)
	}
	v, err = e.Call("probe2")
	if err != nil || v.String() != "ok" {
		t.Fatalf("clearTimeout = %v, %v", v, err)
	}
}
