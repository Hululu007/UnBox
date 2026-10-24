import { describe, expect, it } from 'vitest'
import { certErrorHost } from './tlsExempt'

describe('certErrorHost', () => {
  it('extracts the host from a Go certificate expiry error', () => {
    const msg = '请求失败: Get "https://bad.example.com/api.php/provide/vod?ac=list": tls: failed to verify certificate: x509: certificate has expired or is not yet valid: current time 2026-10-24T12:00:00Z is after 2025-01-01T00:00:00Z'
    expect(certErrorHost(msg)).toBe('bad.example.com')
  })

  it('keeps the port-free host for IP addresses and custom ports', () => {
    const msg = '请求失败: Get "https://1.2.3.4:8443/api.php": x509: certificate is not valid for any names'
    expect(certErrorHost(msg)).toBe('1.2.3.4')
  })

  it('strips the IPv6 brackets so it matches the backend host list', () => {
    const msg = '请求失败: Get "https://[2001:db8::1]:8443/api.php": x509: certificate has expired'
    expect(certErrorHost(msg)).toBe('2001:db8::1')
  })

  it('ignores non-certificate errors', () => {
    expect(certErrorHost('站点返回 HTTP 500')).toBe('')
    expect(certErrorHost('解析分类响应失败: unexpected end of JSON input')).toBe('')
  })

  it('returns nothing when a certificate error carries no URL', () => {
    expect(certErrorHost('x509: certificate has expired or is not yet valid')).toBe('')
  })
})
