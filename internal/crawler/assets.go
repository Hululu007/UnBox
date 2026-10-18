package crawler

import (
	_ "embed"
	"fmt"
	"path"
	"strings"
)

//go:embed assets/cheerio.min.js
var cheerioAsset string

//go:embed assets/crypto-js.js
var cryptoJSAsset string

//go:embed assets/gbk.js
var gbkAsset string

//go:embed assets/cat.js
var catAsset string

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
	case "gbk.js":
		return gbkAsset, true
	case "cat.js":
		return catAsset, true
	default:
		return "", false
	}
}

// assetMissingError 为「脚本 import 了 assets:// 下未内置的文件」构造可定位的错误。
//
// assets:// 协议本身是支持的，走到这里只说明资源没打包进来。目前内置的是
// cheerio / crypto-js / gbk.js / cat.js 四个自有或 MIT/Apache 友好资源，
// 其余（如模板.js）仍不随应用分发。
func assetMissingError(rawURL string) error {
	return fmt.Errorf("站点引用了应用未内置的资源 %s", rawURL)
}
