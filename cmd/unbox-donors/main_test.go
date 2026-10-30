package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeDonorsSortsByAmountAndRedactsAnonymous(t *testing.T) {
	got := normalizeDonors([]rawSponsor{
		{User: rawUser{UserID: "u2", Name: "乙", Avatar: "a2"}, AllSumAmount: "10.5"},
		{User: rawUser{UserID: "u1", Name: "甲", Avatar: "a1"}, AllSumAmount: "30"},
		{User: rawUser{UserID: "u3", Name: "丙", Avatar: "a3"}, AllSumAmount: "10.5", Anonymous: true},
	})
	if len(got) != 3 {
		t.Fatalf("归一化后条数 = %d，期望 3", len(got))
	}
	// 金额降序；同额按导出昵称稳定排序（乙 与匿名的「热心网友」同为 10.5）
	if got[0].Name != "甲" || got[1].Name != "乙" || got[2].Name != "热心网友" {
		t.Fatalf("排序错误: %#v", got)
	}
	// 匿名条目不得泄漏 ID、头像与真实昵称
	if got[2].Name != "热心网友" {
		t.Fatalf("匿名条目昵称未脱敏: %#v", got[2])
	}
	if got[2].ID != "" || got[2].Avatar != "" {
		t.Fatalf("匿名条目 ID 或头像未脱敏: %#v", got[2])
	}
}

func TestNormalizeDonorsSupportsSponsorAndUserNestedVariants(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantID     string
		wantName   string
		wantAvatar string
		wantAmount float64
	}{
		{
			name: "sponsor 嵌套",
			payload: `{"data":{"list":[{` +
				`"sponsor":{"user_id":"sponsor-id","name":"赞助者甲","avatar":"sponsor.png"},` +
				`"all_sum_amount":"18.5"}]}}`,
			wantID:     "sponsor-id",
			wantName:   "赞助者甲",
			wantAvatar: "sponsor.png",
			wantAmount: 18.5,
		},
		{
			name: "user 嵌套兼容旧字段",
			payload: `{"data":{"list":[{` +
				`"user":{"user_id":"legacy-id","user_name":"赞助者乙","user_avatar":"legacy.png"},` +
				`"all_sum_amount":"8.25"}]}}`,
			wantID:     "legacy-id",
			wantName:   "赞助者乙",
			wantAvatar: "legacy.png",
			wantAmount: 8.25,
		},
		{
			name: "user 嵌套兼容别名且优先 user_id",
			payload: `{"data":{"list":[{` +
				`"user":{"user_id_str":"fallback-id","user_id":"preferred-id","nickname":"赞助者丙","avatar_url":"alias.png"},` +
				`"all_sum_amount":"3"}]}}`,
			wantID:     "preferred-id",
			wantName:   "赞助者丙",
			wantAvatar: "alias.png",
			wantAmount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response querySponsorResponse
			if err := json.Unmarshal([]byte(tt.payload), &response); err != nil {
				t.Fatalf("解析响应: %v", err)
			}

			got := normalizeDonors(response.Data.List)
			if len(got) != 1 {
				t.Fatalf("归一化后条数 = %d，期望 1", len(got))
			}
			if got[0].ID != tt.wantID || got[0].Name != tt.wantName ||
				got[0].Avatar != tt.wantAvatar || got[0].Amount != tt.wantAmount {
				t.Fatalf("归一化结果 = %#v，期望 id=%q name=%q avatar=%q amount=%v",
					got[0], tt.wantID, tt.wantName, tt.wantAvatar, tt.wantAmount)
			}
		})
	}
}

func TestNormalizeDonorsWarnsMissingNameWithKeysOnly(t *testing.T) {
	const (
		secretID     = "private-user-id"
		secretAvatar = "https://example.com/private-avatar.png"
	)
	payload := `{"data":{"list":[{` +
		`"user":{"user_id":"` + secretID + `","avatar":"` + secretAvatar + `"},` +
		`"all_sum_amount":"5"}]}}`

	var response querySponsorResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("解析响应: %v", err)
	}

	var stderr bytes.Buffer
	got := normalizeDonorsWithDiagnostics(response.Data.List, &stderr)
	if len(got) != 1 || got[0].Name != "" {
		t.Fatalf("归一化结果 = %#v，期望一条空姓名记录", got)
	}

	const wantWarning = "警告: 第1条赞助者缺少姓名字段，实际键: user{user_id,avatar}\n"
	if stderr.String() != wantWarning {
		t.Fatalf("stderr = %q，期望 %q", stderr.String(), wantWarning)
	}
	for _, secret := range []string{secretID, secretAvatar} {
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("stderr 泄漏字段值 %q: %q", secret, stderr.String())
		}
	}
}

func TestFetchSponsorPageHTTPErrorDoesNotLeakRequestFields(t *testing.T) {
	const (
		userID = "private-user-id"
		token  = "private-token"
	)
	t.Setenv("AFDIAN_USER_ID", userID)
	t.Setenv("AFDIAN_TOKEN", token)

	requestFields := make(chan querySponsorRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req querySponsorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requestFields <- req

		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ec":      401,
			"em":      "凭证无效",
			"token":   token,
			"user_id": req.UserID,
			"ts":      req.TS,
			"sign":    req.Sign,
		})
	}))
	defer server.Close()

	_, _, _, err := fetchSponsorPage(context.Background(), server.Client(), server.URL, token, userID, 1)
	if err == nil {
		t.Fatal("fetchSponsorPage() 应返回 HTTP 错误")
	}
	var stderr bytes.Buffer
	_ = reportError(&stderr, err)

	var req querySponsorRequest
	select {
	case req = <-requestFields:
	case <-time.After(time.Second):
		t.Fatal("未收到请求字段，无法验证脱敏")
	}

	outputs := []struct {
		name string
		text string
	}{
		{name: "stderr", text: stderr.String()},
		{name: "error", text: err.Error()},
	}
	for _, output := range outputs {
		if !strings.Contains(output.text, "HTTP 401") {
			t.Fatalf("%s = %q，缺少 HTTP 状态码", output.name, output.text)
		}
		if !strings.Contains(output.text, "em=凭证无效") {
			t.Fatalf("%s = %q，缺少清理后的 em", output.name, output.text)
		}
		for _, sensitive := range []string{token, userID, req.TS, req.Sign, "token", "user_id", "sign", "ts"} {
			if strings.Contains(output.text, sensitive) {
				t.Fatalf("%s 泄漏敏感请求字段 %q: %q", output.name, sensitive, output.text)
			}
		}
	}
}

func TestFetchSponsorPageBusinessErrorDoesNotLeakRequestFields(t *testing.T) {
	const (
		userID = "private-user-id"
		token  = "private-token"
		sign   = "private-sign"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ec": 401,
			"em": "凭证无效 token=" + token + "&sign=" + sign,
		})
	}))
	defer server.Close()

	_, _, _, err := fetchSponsorPage(context.Background(), server.Client(), server.URL, token, userID, 1)
	if err == nil {
		t.Fatal("fetchSponsorPage() 应返回业务错误")
	}
	var stderr bytes.Buffer
	_ = reportError(&stderr, err)

	for _, output := range []struct {
		name string
		text string
	}{
		{name: "stderr", text: stderr.String()},
		{name: "error", text: err.Error()},
	} {
		for _, sensitive := range []string{token, sign} {
			if strings.Contains(output.text, sensitive) {
				t.Fatalf("%s 泄漏敏感请求字段 %q: %q", output.name, sensitive, output.text)
			}
		}
	}
}

func TestFetchSponsorPageSendsBrowserLikeHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Mozilla/5.0") {
			t.Errorf("User-Agent 不是浏览器指纹: %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Referer") != "https://afdian.com/" {
			t.Errorf("Referer 异常: %q", r.Header.Get("Referer"))
		}
		if r.Header.Get("Accept-Language") == "" {
			t.Error("缺少 Accept-Language")
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(querySponsorResponse{EC: http.StatusOK, Data: querySponsorData{TotalPage: 1}})
	}))
	defer server.Close()

	_, _, _, err := fetchSponsorPage(context.Background(), server.Client(), server.URL, "token", "user-id", 1)
	if err != nil {
		t.Fatalf("fetchSponsorPage() 失败: %v", err)
	}
}

func TestFormatSponsorHTTPErrorHints403WAF(t *testing.T) {
	err := formatSponsorHTTPError(http.StatusForbidden, nil)
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("错误应包含 HTTP 403: %v", err)
	}
	if !strings.Contains(err.Error(), "风控") {
		t.Fatalf("403 错误应提示风控可能拦截: %v", err)
	}
}

func TestSignParamsMatchesAfdianScheme(t *testing.T) {
	params, ts, userID, token := `{"page":1}`, "1700000000", "u1", "tok"
	want := md5Hex(token + "params" + params + "ts" + ts + "user_id" + userID)
	if got := signParams(token, params, ts, userID); got != want {
		t.Fatalf("签名 = %s，期望 %s", got, want)
	}
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
