package crawler

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// maskNonCode 返回与 src 等长的副本：字符串、模板、行注释、块注释内容替换为空格。
// 保留换行符，便于错误定位。仅用于语法定位，不参与执行。
// 已知局限：不识别正则字面量（/.../），其中的 import/引号可能被误判；
// 当前爬虫脚本样本未触发，后续如遇到再升级为带 prev-token 的词法扫描。
func maskNonCode(src string) string {
	out := []byte(src)
	n := len(src)
	blank := func(i int) {
		if out[i] != '\n' && out[i] != '\r' {
			out[i] = ' '
		}
	}
	for i := 0; i < n; {
		switch src[i] {
		case '\'', '"', '`':
			quote := src[i]
			// 保留引号本身（import 正则需要匹配 "..."），只清空内容。
			i++
			for i < n {
				c := src[i]
				if c == '\\' {
					blank(i)
					if i+1 < n {
						blank(i + 1)
					}
					i += 2
					continue
				}
				if c == quote {
					i++
					break
				}
				blank(i)
				i++
			}
		case '/':
			if i+1 < n && src[i+1] == '/' {
				for i < n && src[i] != '\n' {
					blank(i)
					i++
				}
			} else if i+1 < n && src[i+1] == '*' {
				blank(i)
				blank(i + 1)
				i += 2
				for i < n {
					if src[i] == '*' && i+1 < n && src[i+1] == '/' {
						blank(i)
						blank(i + 1)
						i += 2
						break
					}
					blank(i)
					i++
				}
			} else {
				i++
			}
		default:
			i++
		}
	}
	return string(out)
}

var (
	// 注意：Go regexp 不支持 lookahead，用两组分支覆盖：
	//   import x / import{a} / import*as / import "x" / import.meta
	// 压缩器会输出 import{a}from... / import*as n from...（无空格），必须匹配。
	reImportKeyword = regexp.MustCompile(`\bimport\s+[\p{L}_$][\p{L}\p{N}_$]*|\bimport\s*[{\*"'\.]`)
	// 动态 import：前面不能是 . 或标识符字符，避免把 obj.import(...) 误判。
	reDynamicImport = regexp.MustCompile(`(?:^|[^.\p{L}\p{N}_$])import\s*\(`)
	//   export default / export function / export {x} / export{x}
	reExportKeyword = regexp.MustCompile(`\bexport\s+([\p{L}_$][\p{L}\p{N}_$]*|\{|\*)|\bexport\s*[\{\*]`)
)

// needsModuleLoader 判断是否需要 ESM 加载器：
// 有 import（含无空格 named/namespace/dynamic），或有 export default 之外的 export。
func needsModuleLoader(src string) bool {
	masked := maskNonCode(src)
	if reImportKeyword.MatchString(masked) || reDynamicImport.MatchString(masked) {
		return true
	}
	for _, m := range reExportKeyword.FindAllStringSubmatch(masked, -1) {
		if !strings.EqualFold(m[1], "default") {
			return true
		}
	}
	return false
}

type moduleEdit struct {
	start, end int
	text       string
}

func addModuleEdit(edits *[]moduleEdit, start, end int, text string) bool {
	for _, e := range *edits {
		if start < e.end && e.start < end {
			return false
		}
	}
	*edits = append(*edits, moduleEdit{start: start, end: end, text: text})
	return true
}

var (
	reImpDefault = regexp.MustCompile(`import\s+([\p{L}_$][\p{L}\p{N}_$]*)\s*(?:,\s*\{([^}]*)\})?\s+from\s*(["'][^"']*["'])`)
	reImpNamed   = regexp.MustCompile(`import\s*\{([^}]*)\}\s*from\s*(["'][^"']*["'])`)
	reImpNS      = regexp.MustCompile(`import\s*\*\s*as\s+([\p{L}_$][\p{L}\p{N}_$]*)\s+from\s*(["'][^"']*["'])`)
	reImpSide    = regexp.MustCompile(`import\s*(["'][^"']*["'])`)
	reExpDefault = regexp.MustCompile(`export\s+default\s`)
	reExpFunc    = regexp.MustCompile(`export\s+function\s+([\p{L}_$][\p{L}\p{N}_$]*)`)
	reExpVar     = regexp.MustCompile(`export\s+(const|let|var)\s+([\p{L}_$][\p{L}\p{N}_$]*)`)
	reExpList    = regexp.MustCompile(`export\s*\{([^}]*)\}`)
	reExpStar    = regexp.MustCompile(`export\s*\*\s*from`)
)

// resolveImportSpec 把 import 的字符串字面量解析成绝对地址。
// assets:// / http(s):// 原样返回，相对路径按 baseURL 解析。
func resolveImportSpec(vm *goja.Runtime, spec, baseURL string) (string, error) {
	v, err := vm.RunString(spec)
	if err != nil {
		return "", fmt.Errorf("导入地址不是字符串字面量: %s", spec)
	}
	ref := v.String()
	if strings.HasPrefix(ref, "assets://") ||
		strings.HasPrefix(ref, "http://") ||
		strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	if baseURL == "" {
		return "", fmt.Errorf("相对导入 %q 缺少 base URL", ref)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(r).String(), nil
}

// rewriteModule 把 ES Module 语法改写成 goja 可直接执行的 CommonJS 形态：
// import → __module() 调用，export → __exports 赋值。暂不支持 export * from
// 与 import.meta / 动态 import()。
func rewriteModule(vm *goja.Runtime, src, baseURL string) (string, error) {
	masked := maskNonCode(src)
	if reExpStar.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 export * from", ErrESModuleUnsupported)
	}
	if strings.Contains(masked, "import.meta") || reDynamicImport.MatchString(masked) {
		return "", fmt.Errorf("%w: 暂不支持 import.meta / 动态 import()", ErrESModuleUnsupported)
	}

	var edits []moduleEdit
	var exports []string

	// imports
	for _, idx := range reImpDefault.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpDefault.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[3], baseURL)
		if err != nil {
			return "", err
		}
		repl := fmt.Sprintf("const %s = (function(m){return m&&m.default!==undefined?m.default:m;})(__module(%s));", m[1], strconv.Quote(abs))
		if m[2] != "" {
			repl += fmt.Sprintf(" const {%s} = __module(%s);", strings.ReplaceAll(m[2], " as ", ": "), strconv.Quote(abs))
		}
		if !addModuleEdit(&edits, idx[0], idx[1], repl) {
			return "", fmt.Errorf("import 语句重叠: %s", region)
		}
	}
	for _, idx := range reImpNamed.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpNamed.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[2], baseURL)
		if err != nil {
			return "", err
		}
		repl := fmt.Sprintf("const {%s} = __module(%s);", strings.ReplaceAll(m[1], " as ", ": "), strconv.Quote(abs))
		if !addModuleEdit(&edits, idx[0], idx[1], repl) {
			continue
		}
	}
	for _, idx := range reImpNS.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpNS.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[2], baseURL)
		if err != nil {
			return "", err
		}
		if !addModuleEdit(&edits, idx[0], idx[1], fmt.Sprintf("const %s = __module(%s);", m[1], strconv.Quote(abs))) {
			continue
		}
	}
	for _, idx := range reImpSide.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reImpSide.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		abs, err := resolveImportSpec(vm, m[1], baseURL)
		if err != nil {
			return "", err
		}
		if !addModuleEdit(&edits, idx[0], idx[1], fmt.Sprintf("__module(%s);", strconv.Quote(abs))) {
			continue
		}
	}

	// exports
	for _, idx := range reExpDefault.FindAllStringIndex(masked, -1) {
		if !addModuleEdit(&edits, idx[0], idx[1], "__exports.default = ") {
			return "", fmt.Errorf("重复的 export default")
		}
	}
	for _, idx := range reExpFunc.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpFunc.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		if !addModuleEdit(&edits, idx[0], idx[1], "function "+m[1]) {
			continue
		}
		exports = append(exports, m[1])
	}
	for _, idx := range reExpVar.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpVar.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		if !addModuleEdit(&edits, idx[0], idx[1], m[1]+" "+m[2]) {
			continue
		}
		exports = append(exports, m[2])
	}
	for _, idx := range reExpList.FindAllStringSubmatchIndex(masked, -1) {
		region := src[idx[0]:idx[1]]
		m := reExpList.FindStringSubmatch(region)
		if m == nil {
			continue
		}
		if !addModuleEdit(&edits, idx[0], idx[1], "") {
			continue
		}
		for _, item := range strings.Split(m[1], ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			local, exported := item, item
			if parts := strings.Split(item, " as "); len(parts) == 2 {
				local, exported = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			}
			exports = append(exports, exported+" = "+local)
		}
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		src = src[:e.start] + e.text + src[e.end:]
	}
	for _, item := range exports {
		if parts := strings.SplitN(item, " = ", 2); len(parts) == 2 {
			src += "\n__exports." + parts[0] + " = " + parts[1] + ";"
		} else {
			src += "\n__exports." + item + " = " + item + ";"
		}
	}
	return stripAsyncAwait(src), nil
}
