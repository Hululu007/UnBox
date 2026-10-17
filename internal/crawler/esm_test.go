package crawler

import (
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

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
