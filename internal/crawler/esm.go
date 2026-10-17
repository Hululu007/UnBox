package crawler

import (
	"regexp"
	"strings"
)

// maskNonCode 返回与 src 等长的副本：字符串、模板、行注释、块注释内容替换为空格。
// 保留换行符，便于错误定位。仅用于语法定位，不参与执行。
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
	//   import x / import { / import * / import ( / import "x" / import.meta
	reImportKeyword = regexp.MustCompile(`\bimport\s+([\p{L}_$][\p{L}\p{N}_$]*|[\(\{\*])|\bimport\s*["'\.]`)
	//   export default / export function / export {x} / export{x}
	reExportKeyword = regexp.MustCompile(`\bexport\s+([\p{L}_$][\p{L}\p{N}_$]*|\{|\*)|\bexport\s*[\{\*]`)
)

// needsModuleLoader 判断是否需要 ESM 加载器：
// 有 import，或有 export default 之外的 export。
func needsModuleLoader(src string) bool {
	masked := maskNonCode(src)
	if reImportKeyword.MatchString(masked) {
		return true
	}
	for _, m := range reExportKeyword.FindAllStringSubmatch(masked, -1) {
		if !strings.EqualFold(m[1], "default") {
			return true
		}
	}
	return false
}
