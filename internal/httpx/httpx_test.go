package httpx

import (
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func okResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// 瞬时错误（如 TLS 抖动导致的 EOF）应重试一次，并重建请求体。
func TestDoRetriesTransientError(t *testing.T) {
	attempts := 0
	var bodies []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(b))
		}
		if attempts == 1 {
			return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: io.ErrUnexpectedEOF}
		}
		return okResponse("ok"), nil
	})}

	req, err := http.NewRequest(http.MethodPost, "http://example.com/x", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(client, req)
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	defer resp.Body.Close()
	if attempts != 2 {
		t.Fatalf("应尝试 2 次，实际 %d", attempts)
	}
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("重试必须重建请求体，实际 %q", bodies)
	}
}

// 证书错误是确定性的，重试只会浪费时间。
func TestDoDoesNotRetryCertificateError(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: x509.UnknownAuthorityError{}}
	})}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Do(client, req); err == nil {
		t.Fatal("应返回证书错误")
	}
	if attempts != 1 {
		t.Fatalf("证书错误不应重试，实际尝试 %d 次", attempts)
	}
}

// 无请求体的 GET 也必须能重试（GetBody 为 nil 是 GET 的常态）。
func TestDoRetriesBodylessGET(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: io.EOF}
		}
		return okResponse("ok"), nil
	})}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(client, req)
	if err != nil {
		t.Fatalf("GET 重试后应成功: %v", err)
	}
	defer resp.Body.Close()
	if attempts != 2 {
		t.Fatalf("GET 应重试一次，实际尝试 %d 次", attempts)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }
func (r failingReader) Close() error             { return nil }

// HTTP/2 的 PROTOCOL_ERROR 往往在读取响应体时才暴露，必须整体重试。
func TestDoReadAllRetriesBodyReadError(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       failingReader{err: ioError("stream error: stream ID 1; PROTOCOL_ERROR")},
			}, nil
		}
		return okResponse("payload"), nil
	})}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, body, err := DoReadAll(client, req, 0)
	if err != nil {
		t.Fatalf("读响应体失败应重试: %v", err)
	}
	if string(body) != "payload" || attempts != 2 {
		t.Fatalf("body=%q attempts=%d", body, attempts)
	}
}

// 超过体积上限是确定性错误，不应重试。
func TestDoReadAllBodyTooLarge(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		return okResponse("0123456789"), nil
	})}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := DoReadAll(client, req, 4); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("应返回 ErrBodyTooLarge，实际 %v", err)
	}
	if attempts != 1 {
		t.Fatalf("超限不应重试，实际尝试 %d 次", attempts)
	}
}

// 传输层的 TLS 握手超时必须跟随客户端超时，否则 30 秒的客户端实际 10 秒就失败。
func TestNewClientTLSHandshakeFollowsTimeout(t *testing.T) {
	client := NewClient(30 * time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型 = %T", client.Transport)
	}
	if transport.TLSHandshakeTimeout != 30*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v，期望 30s", transport.TLSHandshakeTimeout)
	}
}

func TestIsTransient(t *testing.T) {
	cases := map[string]bool{
		"tls: bad record MAC":                true,
		"stream error: stream ID 1":          true,
		"read tcp: connection reset by peer": true,
		"x509: certificate has expired":      false,
		"EOF":                                true,
	}
	for msg, want := range cases {
		if got := IsTransient(ioError(msg)); got != want {
			t.Errorf("IsTransient(%q) = %v，期望 %v", msg, got, want)
		}
	}
	if IsTransient(nil) {
		t.Error("nil 不应视为瞬时错误")
	}
}

// 证书校验失败时，只有被显式许可的主机才降级重试。
func TestInsecureFallbackOnlyForAllowlistedHost(t *testing.T) {
	defer SetInsecureHosts(nil)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("insecure-ok"))
	}))
	defer srv.Close()

	// 未加入白名单：保持原有行为，证书错误直接失败。
	SetInsecureHosts(nil)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Do(NewClient(5*time.Second), req); !isCertificateError(err) {
		t.Fatalf("未授权主机应返回证书错误，实际 %v", err)
	}

	// 加入白名单后：跳过校验并成功拿到响应。
	SetInsecureHosts([]string{"127.0.0.1"})
	req, err = http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, body, err := DoReadAll(NewClient(5*time.Second), req, 0)
	if err != nil {
		t.Fatalf("授权主机应成功: %v", err)
	}
	defer resp.Body.Close()
	if string(body) != "insecure-ok" {
		t.Fatalf("body = %q", body)
	}
}

// 豁免后仍然失败时必须说明已经跳过了证书校验，否则用户会以为豁免没生效。
func TestInsecureRetryErrorWording(t *testing.T) {
	err := insecureRetryError(ioError("bad record mac"))
	if !strings.Contains(err.Error(), "已按主机豁免跳过证书校验") {
		t.Fatalf("错误应说明已豁免，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "bad record mac") {
		t.Fatalf("应保留原始错误，实际: %v", err)
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"Example.COM":      "example.com",
		"example.com:8443": "example.com",
		"  example.com  ":  "example.com",
		"":                 "",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q，期望 %q", in, got, want)
		}
	}
	SetInsecureHosts([]string{"Example.COM:443"})
	defer SetInsecureHosts(nil)
	if !InsecureHost("example.com") {
		t.Fatal("带端口/大写的主机应能匹配")
	}
	if InsecureHost("other.com") {
		t.Fatal("未授权主机不应匹配")
	}
}

type ioError string

func (e ioError) Error() string { return string(e) }
