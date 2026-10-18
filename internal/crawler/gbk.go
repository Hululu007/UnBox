package crawler

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// installGBK 注入 GBK 编解码原语，供内置 gbk.js shim 使用。
//
// 上游 FongMi 的 gbk.js 是 GPL-3.0（与 cat.js 同样不随应用分发），这里用
// golang.org/x/text 的 GBK 编码表实现同等语义：
//   - encode：ASCII 走 encodeURIComponent，其余字符转成 GBK 字节的 %XX%YY；
//   - decode：把连续的 %XX 还原成 GBK 文本。
func (e *Engine) installGBK() {
	_ = e.vm.Set("__gbkEncode", func(call goja.FunctionCall) goja.Value {
		return e.vm.ToValue(gbkEncodeString(call.Argument(0).String()))
	})
	_ = e.vm.Set("__gbkDecode", func(call goja.FunctionCall) goja.Value {
		return e.vm.ToValue(gbkDecodeString(call.Argument(0).String()))
	})
}

// gbkEncodeString 把字符串编码成 GBK 百分号形式（如「你好」→ %C4%E3%BA%C3）。
func gbkEncodeString(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		if r < 0x80 {
			b.WriteString(encodeURIComponentRune(r))
			continue
		}
		raw, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(string(r)))
		if err != nil || len(raw) == 0 {
			b.WriteRune(r)
			continue
		}
		for _, by := range raw {
			fmt.Fprintf(&b, "%%%02X", by)
		}
	}
	return b.String()
}

// encodeURIComponentRune 复刻 encodeURIComponent 对单个字符的处理。
func encodeURIComponentRune(r rune) string {
	if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
		return string(r)
	}
	switch r {
	case '-', '_', '.', '!', '~', '*', '\'', '(', ')':
		return string(r)
	}
	var b strings.Builder
	for _, by := range []byte(string(r)) {
		fmt.Fprintf(&b, "%%%02X", by)
	}
	return b.String()
}

// gbkDecodeString 把连续的 %XX 还原成文本；字节里有高位时按 GBK 解码。
func gbkDecodeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) && isHexByte(s[i+1]) && isHexByte(s[i+2]) {
			var raw []byte
			for i+2 < len(s) && s[i] == '%' && isHexByte(s[i+1]) && isHexByte(s[i+2]) {
				raw = append(raw, hexByte(s[i+1])<<4|hexByte(s[i+2]))
				i += 3
			}
			b.WriteString(decodeGBKBytes(raw))
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func decodeGBKBytes(raw []byte) string {
	high := false
	for _, by := range raw {
		if by >= 0x80 {
			high = true
			break
		}
	}
	if !high {
		return string(raw)
	}
	if out, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw); err == nil {
		return string(out)
	}
	return string(raw)
}

func isHexByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexByte(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}
