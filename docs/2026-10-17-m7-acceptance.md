# M7 drpy2 / ESM 真实站点验收（2026-10-17）

验收对象：M7.1（drpy2 选源）与 M7.2（ES Module 加载器）。
代码基线：`feat/m7-drpy2-esm`，验收前 HEAD `3c810c98`。
环境：WSL2 Linux 6.18.54.1（microsoft-standard-WSL2），Go 1.26.3 linux/amd64，
CGO 开启；宿主机**缺少 GTK4 / WebKitGTK 开发包**（`pkg-config --exists gtk4`、
`pkg-config --exists webkitgtk-6.0` 均为 false）。

结论：**M7.1/M7.2 的代码路径（选源、ESM 改写与加载）已实现并通过单元/端到端测试，
但三条真实站点验收均未完整跑通分类 → 列表 → 详情 → 播放**。三个失败各有明确根因，
均记录在下方，未修改验收测试、未伪造成功。

## 1. 验证命令

### 1.1 可运行包（排除 Wails 相关包）

因宿主缺少 GTK4/WebKitGTK，`internal/shell` 与 `cmd/unbox` 无法编译。对其余全部包执行：

```bash
go list ./... | grep -v '/internal/shell$' | grep -v '/cmd/unbox$' \
  | xargs go test -count=1
go list ./... | grep -v '/internal/shell$' | grep -v '/cmd/unbox$' | xargs go vet
gofmt -l .
go list ./... | grep -v '/internal/shell$' | grep -v '/cmd/unbox$' \
  | grep -v '/build/windows/nsis$' | xargs env CGO_ENABLED=1 go build
```

结果：

| 命令 | 结果 |
|---|---|
| `go test`（非 Wails 包） | 全部 `ok`：`config`、`crawler`、`library`、`library/thumb`、`playback`、`player`、`player/failover`、`player/mpvplugin`、`player/mpvproc`、`probe`、`provider/live`、`provider/tvbox`、`store`、`cmd/unbox-scan`、`cmd/unbox-donors`、`build/windows/nsis`；`internal/provider` 无测试文件 |
| `go vet`（非 Wails 包） | 无输出，退出码 0 |
| `gofmt -l .` | 无输出 |
| `CGO_ENABLED=1 go build`（非 Wails 包） | 退出码 0 |

### 1.2 全量命令的限制（如实记录）

```bash
go test ./... -count=1
CGO_ENABLED=1 go build ./...
```

两者均在 `internal/shell` 失败，`pkg-config` 报告：

```
Package 'glib-2.0' was not found in the pkg-config search path.
Package 'gobject-2.0' not found
Package 'gtk4' not found
Package 'webkitgtk-6.0' not found
Package 'gio-unix-2.0' not found
Package 'libsoup-3.0' not found
```

这是宿主环境限制（未安装 Wails 的 Linux cgo 依赖），不是 M7 代码回归；按要求未安装
系统包。

### 1.3 Provider 选源与 ESM 端到端单测

```bash
go test ./internal/provider/tvbox -run 'TestSpiderScriptURL|TestSpiderESMJsEvalReturn' -count=1 -v
```

结果：两项均 PASS。

- `TestSpiderScriptURL` 覆盖 `api=.../drpy2.min.js` + `ext=.../360影视.js` → 实际选择
  `ext`（含带 query 的 api、非 js ext、无 ext 等分支）。
- `TestSpiderESMJsEvalReturn` 用 `httptest` 覆盖 ESM `export function __jsEvalReturn`
  的加载与动作提升。

## 2. 真实站点验收结果

### 2.1 drpy2：`360影视.js`（`var rule`）

```bash
UNBOX_REAL_JS_PATH=/tmp/drpy-research/scripts/drpy2_360.js \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

实测日志要点：

```
脚本形态: rule=true init=false homeContent=false ...
分类(4):
  - id="2" name="电视剧"
  - id="1" name="电影"
  - id="3" name="综艺"
  - id="4" name="动漫"
VodCategory("2") 失败: json 列表路径不存在: data.movies
```

- 加载与分类：**成功**（`var rule` 解析成功，4 个分类）。
- 列表/详情/播放：**未执行**（列表即失败）。

根因（既有 drpy2 方言缺口，不是 M7 选源问题）：脚本的列表模板是

```
url:'https://api.web.360kan.com/v1/filter/list?catid=fyclassfyfilter'
```

`fillURL` 只替换 `fyclass`/`fypage`/`**`，不替换 `fyfilter`，生成
`catid=2fyfilter`。实测该请求返回：

```json
{"data":null,"errno":1,"msg":"catid参数错误"}
```

而同一接口 `catid=2` 返回 `data.movies` 列表。即：**引擎尚未实现 drpy2 的
`fyfilter` / 分类筛选占位**（`filter` / `filter_url` 也未实现），含该占位的站点
无法取到一级列表。这属于 M7 之后需补的 drpy2 方言能力。

选源本身已由 `TestSpiderScriptURL` 验证：drpy2 站点加载的是 `ext`（`360影视.js`），
不是 `api`（`drpy2.min.js`）。

### 2.2 ESM：`聚合短剧.js`

```bash
UNBOX_REAL_JS_URL='https://clun.top/js/聚合短剧.js' UNBOX_REAL_SEARCH=测试 \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

实测日志要点：

```
脚本形态: rule=false init=true homeContent=false ...
VodClasses 失败: ReferenceError: console is not defined at getFilterOptions (<eval>:243:3(15))
分类列表(0)（取前 3）:
搜索结果(0)（取前 3）:
分类列表为空，无法验证详情/播放
```

- 下载与 ESM 加载：**成功**（脚本被识别为模块，`init` 已提升为全局）。
- `./lib/crypto-js.js` 导入：**成功**（随主脚本下载/解析）。
- 分类：**失败**，未进入列表/详情/播放。

根因：脚本第 243 行在 `getFilterOptions` 的 `catch` 中调用 `console.error(...)`，
而 goja 运行时未注入 `console`。异常处理路径本身又抛 `ReferenceError`，导致
`homeContent` 无法返回分类。需要补一个最小 `console` shim（`log`/`warn`/`error`
映射到现有 `print`/日志），或从脚本侧容错。

### 2.3 ESM：`lf_search3_min.js`（混淆 import/export）

首选按 URL 运行：

```bash
UNBOX_REAL_JS_URL='https://raw.githubusercontent.com/gaotianliuyun/gao/master/lib/lf_search3_min.js' \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

首次及重试均为：

```
下载/加载爬虫失败: 下载爬虫失败: Get ".../lf_search3_min.js": net/http: TLS handshake timeout
```

该地址用 `curl` 可正常访问（HTTP 200，363687 字节）；Go 客户端在用 HTTP/2 时
TLS 握手超时，关闭 HTTP/2 后同一 URL 可下载：

```bash
GODEBUG=http2client=0 UNBOX_REAL_JS_URL='https://raw.githubusercontent.com/gaotianliuyun/gao/master/lib/lf_search3_min.js' \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

```
模块 https://raw.githubusercontent.com/gaotianliuyun/gao/master/lib/lf_search3_min.js 编译失败:
SyntaxError: SyntaxError: (anonymous): Line 1:363339 Unexpected reserved word (and 5 more errors)
```

用与上述 URL 内容完全一致的本地副本（SHA-256 均为
`3e13db194b3b62fd39c27eff7de82afe3719dc0b228e27f5f9fceb9e145c5b20`，
`/tmp/drpy-research/sample/070_api_lf_search3_min.js`）运行，得到同一错误：

```bash
UNBOX_REAL_JS_PATH=/tmp/drpy-research/sample/070_api_lf_search3_min.js \
  go test ./internal/crawler -run TestRealCrawlerAcceptance -count=1 -v
```

```
加载爬虫失败: 模块  编译失败: SyntaxError: SyntaxError: (anonymous): Line 1:363339 Unexpected reserved word (and 5 more errors)
```

根因：`maskNonCode` 不识别正则字面量。脚本在约 172 KB 处有正则 `/^\/\//m`，
其中的 `//` 被误判为行注释，掩码把该行（整个压缩脚本就是一行）直到文件末尾全部
清空，于是文件末尾的 `export default {...}` 未被 `rewriteModule` 改写；两条
`import` 正常剥离后，goja 仍看到 `export`，报 `Unexpected reserved word`。
这正是代码注释里已声明的「不识别正则字面量」限制，本脚本实际触发了它。

## 3. 验收汇总

| 脚本 | 加载 | 分类 | 列表 | 详情 | 播放 | 阻塞点 |
|---|---|---|---|---|---|---|
| `drpy2_360.js` | ✅ `rule` 解析 | ✅ 4 个 | ❌ | 未执行 | 未执行 | `fyfilter` 占位未实现 → `catid=2fyfilter` 被 API 拒绝 |
| `聚合短剧.js` | ✅ ESM + crypto-js | ❌ | 未执行 | 未执行 | 未执行 | 运行时缺 `console`，`getFilterOptions` 的 catch 再次抛错 |
| `lf_search3_min.js` | ❌ 编译失败 | 未执行 | 未执行 | 未执行 | 未执行 | `maskNonCode` 把正则里的 `//` 当注释，漏改末尾 `export default` |

## 4. 后续待办（按优先级）

1. **ESM 掩码升级**：把 `maskNonCode` 从「字符串/注释扫描」升级为带 prev-token
   判定的词法扫描，识别正则字面量（至少处理 `/^\/\//m` 这类含 `//` 的模式），
   否则混淆 ESM 脚本会继续漏改 `export`。
2. **`console` shim**：注入 `console.log/warn/error/debug`（映射到现有 `print`
   或日志缓冲），让脚本的异常处理路径不再因缺 API 而中断。
3. **drpy2 `fyfilter` / 筛选**：实现 `fyfilter` 占位与 `filter`/`filter_url`
   展开，至少支持「无筛选值 → 空串」，否则大量 drpy2 站点的列表模板不可用。
4. **M7.3 `cat.js` shim**：仍待办；当前不内置 GPL-3.0 的 cat.js，`bili_open.js`
   这类强依赖 `Crypto`/`jinja2`/`_` 的脚本需要自有 shim 才能完整运行。
5. 网络：Go 客户端访问 `raw.githubusercontent.com` 的 HTTP/2 TLS 握手在本次环境
   超时，`GODEBUG=http2client=0` 可绕过；属本机网络环境问题，不是爬虫代码问题。

## 5. 许可证

内置资源及其许可证见 `internal/crawler/assets/THIRD-PARTY-NOTICES.txt`：

- cheerio（MIT）— `LICENSE.cheerio.txt`；
- crypto-js（MIT）— `LICENSE.crypto-js.txt`；
- cheerio bundle 内的 css-select 家族（BSD-2-Clause）— `LICENSE.css-select.txt`
  （新增，来源 `https://raw.githubusercontent.com/fb55/css-select/master/LICENSE`）。

未内置：cat.js（GPL-3.0）、模板.js / gbk.js（未声明许可证）。
