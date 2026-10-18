// gbk.js — UnBox 自有 shim（不使用上游 FongMi 的 GPL-3.0 实现）。
//
// drpy2 的 gbk 工具：把中文转成 GBK 百分号编码，以及反向还原。
// 实际编解码由 Go 侧 __gbkEncode / __gbkDecode 完成（golang.org/x/text GBK）。
export function gbkTool() {
	return {
		encode: function (str) {
			return __gbkEncode(String(str));
		},
		decode: function (str) {
			return __gbkDecode(String(str));
		},
	};
}
