package crawler

import (
	_ "embed"
	"path"
	"strings"
)

//go:embed assets/cheerio.min.js
var cheerioAsset string

//go:embed assets/crypto-js.js
var cryptoJSAsset string

// assetSource 按 basename 映射内置资源。
// 已知 basename 优先于远端请求，避免源站把 ./lib/crypto-js.js 返回成 404 HTML。
func assetSource(rawURL string) (string, bool) {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	switch path.Base(rawURL) {
	case "cheerio.min.js":
		return cheerioAsset, true
	case "crypto-js.js":
		return cryptoJSAsset, true
	default:
		return "", false
	}
}
