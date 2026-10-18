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
	default:
		return "", false
	}
}

// assetMissingError 为「脚本 import 了 assets:// 下未内置的文件」构造可定位的错误。
//
// assets:// 协议本身是支持的，走到这里只说明资源没打包进来。已知缺口是 cat.js：
// 上游为 GPL-3.0，不随应用分发，M7.3 计划用自有 shim 替代。
func assetMissingError(rawURL string) error {
	if path.Base(rawURL) == "cat.js" {
		return fmt.Errorf(
			"站点依赖 %s，但 cat.js 上游为 GPL-3.0、未随应用分发，当前版本暂不支持该站点（M7.3 待实现自有 shim）",
			rawURL)
	}
	return fmt.Errorf("站点引用了应用未内置的资源 %s", rawURL)
}
