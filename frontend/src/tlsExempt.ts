// tlsExempt.ts —— 从后端错误里识别「证书过期/不受信」并取出站点主机名，
// 供点播页的一键豁免使用，避免用户复制网址再切到设置页。
//
// Go 的 *url.Error 会把请求地址写进错误串（Get "https://host/path": ...），
// x509 的错误文案固定是英文，据此判定即可；普通 HTTP 错误不触发。

const CERT_HINTS = [
  /x509:/i,
  /certificate has expired/i,
  /certificate is not valid/i,
  /failed to verify certificate/i,
  /unknown authority/i,
]

const URL_RE = /https?:\/\/[^\s"'）)，,]+/i

/** certErrorHost 返回证书错误中的主机名；不是证书错误或取不到主机时返回空串。 */
export function certErrorHost(message: string): string {
  if (!message || !CERT_HINTS.some((re) => re.test(message))) return ''
  const match = message.match(URL_RE)
  if (!match) return ''
  const raw = match[0].replace(/[)\].,;]+$/, '')
  try {
    // URL.hostname 保留 IPv6 的方括号，后端 normalizeTLSHost 会去掉，这里对齐。
    return new URL(raw).hostname.replace(/^\[|\]$/g, '')
  } catch {
    return ''
  }
}
