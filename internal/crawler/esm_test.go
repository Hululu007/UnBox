package crawler

import (
	"errors"
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

func TestRewriteModuleRejections(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"export star", `export * from "https://x/y.js";`},
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

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
