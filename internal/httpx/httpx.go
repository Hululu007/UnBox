// Package httpx 提供带显式传输层超时与瞬时错误重试的 HTTP 客户端。
//
// 真实源站里 TLS 握手超时、连接被重置、HTTP/2 PROTOCOL_ERROR 这类错误很常见，
// 它们多为瞬时故障，重试一次通常就能成功；而证书错误是确定性的，不能重试。
package httpx

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// maxAttempts 是瞬时错误下的总尝试次数（首次 + 一次重试）。
const maxAttempts = 2

// ErrBodyTooLarge 表示响应体超过调用方给定的上限。
var ErrBodyTooLarge = errors.New("响应体超过上限")

// NewClient 返回带显式传输层配置的客户端。
//
// 直接写 &http.Client{Timeout: d} 会用 http.DefaultTransport，它的
// TLSHandshakeTimeout 固定为 10 秒，于是「客户端超时 30 秒」实际在 TLS
// 握手阶段 10 秒就失败了。这里让握手超时跟随客户端超时，避免跨境线路
// 上的慢握手被提前掐断。
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   timeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// Do 执行请求；遇到瞬时网络错误时重试一次。
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, lastErr
		}
		resp, err := client.Do(attemptReq)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldRetry(req, err) {
			return resp, err
		}
	}
	return nil, lastErr
}

// DoReadAll 执行请求并读取整个响应体，遇到瞬时错误（含读取阶段的
// HTTP/2 PROTOCOL_ERROR、EOF）时整体重试一次。limit > 0 时超过上限返回
// ErrBodyTooLarge（不重试）。
func DoReadAll(client *http.Client, req *http.Request, limit int64) (*http.Response, []byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, nil, lastErr
		}
		resp, err := client.Do(attemptReq)
		if err != nil {
			lastErr = err
			if !shouldRetry(req, err) {
				return resp, nil, err
			}
			continue
		}
		body, readErr := readAllLimit(resp.Body, limit)
		if readErr != nil {
			_ = resp.Body.Close()
			lastErr = readErr
			if !shouldRetry(req, readErr) {
				return resp, nil, readErr
			}
			continue
		}
		return resp, body, nil
	}
	return nil, nil, lastErr
}

func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(r)
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

// requestForAttempt 返回本次尝试要用的请求；重试时重建请求体。
func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 {
		return req, nil
	}
	if req.GetBody == nil {
		if req.Body != nil {
			return nil, errors.New("请求体无法重建")
		}
		return req.Clone(req.Context()), nil
	}
	clone := req.Clone(req.Context())
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}

// shouldRetry 判断错误是否值得重试：瞬时错误、调用方未放弃、请求体可重建。
func shouldRetry(req *http.Request, err error) bool {
	if !IsTransient(err) {
		return false
	}
	if req.Context().Err() != nil {
		return false
	}
	return req.Body == nil || req.GetBody != nil
}

// IsTransient 判断错误是否值得重试。
//
// 证书类错误（x509）是确定性的，不重试；DNS 解析失败通常也重试不出结果，
// 但超时可能只是抖动，因此统一按 net.Error.Timeout() 归入可重试。
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"eof",
		"tls: bad record mac",
		"protocol_error",
		"connection reset by peer",
		"broken pipe",
		"server closed idle connection",
		"stream error",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
