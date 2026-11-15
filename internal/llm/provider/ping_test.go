package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hi-tools/internal/llm/provider"
)

// --- Client.Headers: gửi kèm mọi request ---

func TestClientHeadersSentOnListModels(t *testing.T) {
	var gotSession, gotExtra string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get("x-opencode-session")
		gotExtra = r.Header.Get("X-Extra")
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	c := provider.NewClient()
	c.Headers = map[string]string{
		"x-opencode-session": "3f2c-uuid",
		"X-Extra":            "v1",
	}
	if _, err := c.ListModels(context.Background(), srv.URL+"/v1", "sk-key"); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotSession != "3f2c-uuid" {
		t.Fatalf("header x-opencode-session = %q, muốn 3f2c-uuid", gotSession)
	}
	if gotExtra != "v1" {
		t.Fatalf("header X-Extra = %q, muốn v1", gotExtra)
	}
}

func TestClientHeadersSentOnChat(t *testing.T) {
	var gotSession, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get("x-opencode-session")
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null}}]}`))
	}))
	defer srv.Close()

	c := provider.NewClient()
	c.Headers = map[string]string{"x-opencode-session": "sess-1"}
	if err := c.Chat(context.Background(), srv.URL+"/v1", "sk-chat", "m"); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotSession != "sess-1" {
		t.Fatalf("header x-opencode-session trên chat = %q, muốn sess-1", gotSession)
	}
	if gotAuth != "Bearer sk-chat" {
		t.Fatalf("Authorization = %q, muốn Bearer sk-chat", gotAuth)
	}
}

// --- Client.Chat ---

func TestChatSendsMethodPathBody(t *testing.T) {
	var (
		gotMethod, gotPath, gotAuth, gotCT string
		gotBody                            map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null}}]}`))
	}))
	defer srv.Close()

	c := provider.NewClient()
	if err := c.Chat(context.Background(), srv.URL+"/v1", "sk-chat", "deepseek-v4.1-flash"); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, muốn POST", gotMethod)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, muốn /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-chat" {
		t.Fatalf("Authorization = %q, muốn Bearer sk-chat", gotAuth)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Fatalf("Content-Type = %q, muốn application/json", gotCT)
	}
	if gotBody == nil {
		t.Fatal("body không phải JSON")
	}
	if got, _ := gotBody["model"].(string); got != "deepseek-v4.1-flash" {
		t.Fatalf("body.model = %q, muốn deepseek-v4.1-flash", got)
	}
	if got, _ := gotBody["max_tokens"].(float64); got != 16 {
		t.Fatalf("body.max_tokens = %v, muốn 16", gotBody["max_tokens"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("body.messages = %v, muốn đúng một message", gotBody["messages"])
	}
	msg, _ := msgs[0].(map[string]any)
	if role, _ := msg["role"].(string); role != "user" {
		t.Fatalf("messages[0].role = %q, muốn user", role)
	}
	if content, _ := msg["content"].(string); content != "ping" {
		t.Fatalf("messages[0].content = %q, muốn ping", content)
	}
}

func TestChatSuccessIgnoresBodyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Model reasoning có thể trả content: null; không được đọc/parse nội dung.
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null}}]}`))
	}))
	defer srv.Close()

	if err := provider.NewClient().Chat(context.Background(), srv.URL+"/v1", "k", "m"); err != nil {
		t.Fatalf("Chat HTTP 200 với content null trả lỗi: %v", err)
	}
}

func TestChatNon200ReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	err := provider.NewClient().Chat(context.Background(), srv.URL+"/v1", "sk-chat", "m")
	if err == nil {
		t.Fatal("Chat status 429 trả nil error")
	}
	var httpErr *provider.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("lỗi %v không phải *HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, muốn 429", httpErr.StatusCode)
	}
}

// --- Classify ---

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401", &provider.HTTPError{StatusCode: 401, Status: "401 Unauthorized"}, "auth"},
		{"403", &provider.HTTPError{StatusCode: 403, Status: "403 Forbidden"}, "auth"},
		{"402", &provider.HTTPError{StatusCode: 402, Status: "402 Payment Required"}, "quota"},
		{"429", &provider.HTTPError{StatusCode: 429, Status: "429 Too Many Requests"}, "quota"},
		{"404", &provider.HTTPError{StatusCode: 404, Status: "404 Not Found"}, "http"},
		{"500", &provider.HTTPError{StatusCode: 500, Status: "500 Internal Server Error"}, "http"},
		{"dns", &net.DNSError{Err: "no such host", Name: "x.example"}, "net"},
		{"url", &url.Error{Op: "Get", URL: "http://x.example/v1/models", Err: errors.New("connection refused")}, "net"},
		{"response", errors.New("response /models không hợp lệ: EOF"), "response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := provider.Classify(tc.err); got != tc.want {
				t.Fatalf("Classify(%v) = %q, muốn %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyWrappedNetError(t *testing.T) {
	base := &net.DNSError{Err: "no such host", Name: "khong-ton-tai.example", IsNotFound: true}
	wrapped := fmt.Errorf("gọi /models: %w", &url.Error{Op: "Get", URL: "http://khong-ton-tai.example/v1/models", Err: base})
	if got := provider.Classify(wrapped); got != "net" {
		t.Fatalf("Classify(lỗi mạng bọc nhiều lớp) = %q, muốn net", got)
	}
}

func TestClassifyRealNetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	base := srv.URL + "/v1"
	srv.Close() // tắt server để kết nối bị từ chối

	_, err := provider.NewClient().ListModels(context.Background(), base, "k")
	if err == nil {
		t.Fatal("ListModels tới server đã tắt trả nil error")
	}
	if got := provider.Classify(err); got != "net" {
		t.Fatalf("Classify(lỗi kết nối thật %v) = %q, muốn net", err, got)
	}
}

func TestClassifyResponseFromBadBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"nope":true}`))
	}))
	defer srv.Close()

	_, err := provider.NewClient().ListModels(context.Background(), srv.URL+"/v1", "k")
	if err == nil {
		t.Fatal("ListModels body thiếu data trả nil error")
	}
	if got := provider.Classify(err); got != "response" {
		t.Fatalf("Classify(body không parse được) = %q, muốn response", got)
	}
}

func TestClassifyHTTPErrorKeepsStatus(t *testing.T) {
	// 200 là thành công, không có lỗi; mọi status khác 200 phải rơi vào auth/quota/http.
	for _, code := range []int{400, 404, 500, 502} {
		if got := provider.Classify(&provider.HTTPError{StatusCode: code}); got != "http" {
			t.Fatalf("Classify(HTTP %d) = %q, muốn http", code, got)
		}
	}
}

// --- LoadPings / SavePings ---

func TestLoadPingsMissingFile(t *testing.T) {
	got := provider.LoadPings(t.TempDir())
	if len(got) != 0 {
		t.Fatalf("LoadPings khi thiếu file = %v, muốn rỗng", got)
	}
}

func TestLoadPingsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_ping.json"), []byte("{khong-phai-json"), 0o600); err != nil {
		t.Fatalf("ghi file hỏng: %v", err)
	}
	got := provider.LoadPings(dir)
	if len(got) != 0 {
		t.Fatalf("LoadPings file hỏng = %v, muốn rỗng", got)
	}
}

func TestSavePingsMergesAndPreservesOtherKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	now := time.Now().UTC().Truncate(time.Second)

	if err := provider.SavePings(dir, map[string]provider.PingResult{
		"alpha": {At: now, OK: true, LatencyMS: 12},
	}); err != nil {
		t.Fatalf("SavePings lần 1: %v", err)
	}
	// Ghi đè beta nhưng phải giữ alpha.
	if err := provider.SavePings(dir, map[string]provider.PingResult{
		"beta": {At: now, OK: false, Kind: "auth", Error: "HTTP 401 Unauthorized"},
	}); err != nil {
		t.Fatalf("SavePings lần 2: %v", err)
	}

	got := provider.LoadPings(dir)
	if len(got) != 2 {
		t.Fatalf("LoadPings = %v, muốn 2 key (merge)", got)
	}
	if !got["alpha"].OK || got["alpha"].LatencyMS != 12 {
		t.Fatalf("alpha mất/không đúng: %+v", got["alpha"])
	}
	if got["beta"].OK || got["beta"].Kind != "auth" {
		t.Fatalf("beta không đúng: %+v", got["beta"])
	}
}

func TestSavePingsOverwritesSameKey(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	if err := provider.SavePings(dir, map[string]provider.PingResult{
		"p": {At: now, OK: true, LatencyMS: 5},
	}); err != nil {
		t.Fatalf("SavePings lần 1: %v", err)
	}
	if err := provider.SavePings(dir, map[string]provider.PingResult{
		"p": {At: now, OK: false, Kind: "quota", Error: "HTTP 429 Too Many Requests"},
	}); err != nil {
		t.Fatalf("SavePings lần 2: %v", err)
	}

	got := provider.LoadPings(dir)
	if len(got) != 1 {
		t.Fatalf("LoadPings = %v, muốn đúng 1 key", got)
	}
	if got["p"].OK || got["p"].Kind != "quota" {
		t.Fatalf("key p không được ghi đè: %+v", got["p"])
	}
}

func TestSavePingsCreatesFileAndDirWith0600(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chua", "co")
	if err := provider.SavePings(dir, map[string]provider.PingResult{
		"p": {At: time.Now(), OK: true},
	}); err != nil {
		t.Fatalf("SavePings: %v", err)
	}

	path := filepath.Join(dir, "_ping.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat _ping.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("quyền _ping.json = %o, muốn 600", perm)
	}
}

func TestPingResultJSONTags(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	data, err := json.Marshal(provider.PingResult{At: now, OK: true, LatencyMS: 320})
	if err != nil {
		t.Fatalf("marshal PingResult: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal PingResult: %v", err)
	}
	for _, key := range []string{"at", "ok", "latency_ms"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("JSON thiếu key %q: %s", key, data)
		}
	}
	if _, ok := m["kind"]; ok {
		t.Fatalf("JSON có kind khi rỗng (thiếu omitempty): %s", data)
	}
	if _, ok := m["error"]; ok {
		t.Fatalf("JSON có error khi rỗng (thiếu omitempty): %s", data)
	}
}
