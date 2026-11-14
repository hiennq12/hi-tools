package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hi-tools/internal/llm/provider"
)

func (e *cliEnv) seedCache(t *testing.T, name string, models []string, age time.Duration) {
	t.Helper()
	if err := provider.SaveCache(e.cacheDir, name, &provider.Cache{
		FetchedAt: time.Now().Add(-age),
		Models:    models,
	}); err != nil {
		t.Fatalf("seed cache %s: %v", name, err)
	}
}

func TestModelsSingleProviderUsesCache(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://api.deeprelay.example/v1
    api_key: env:DEEPRELAY_KEY
`)
	t.Setenv("DEEPRELAY_KEY", "sk-x")
	e.seedCache(t, "deeprelay", []string{"b", "a"}, 5*time.Minute)

	// base_url trong registry trỏ mạng thật, nhưng cache còn hạn nên không gọi.
	code, stdout, stderr := runCLI(t, "", "models", "deeprelay")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "deeprelay/a\ndeeprelay/b" {
		t.Fatalf("stdout = %q, muốn hai dòng deeprelay/a và deeprelay/b", stdout)
	}
}

func TestModelsRefreshFlagAfterPositional(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
	}))
	defer srv.Close()
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-p")

	// Flag đặt SAU tham số vị trí phải được nhận.
	code, stdout, stderr := runCLI(t, "", "models", "p", "--refresh")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	got := strings.TrimSpace(stdout)
	if got != "p/m1\np/m2" {
		t.Fatalf("stdout = %q, muốn p/m1 và p/m2", got)
	}
}

func TestModelsJSONOutput(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: https://api.example.com/v1
    api_key: env:P_KEY
`)
	e.seedCache(t, "p", []string{"x", "y"}, time.Minute)
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "models", "p", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}

	var got []struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout không phải JSON hợp lệ: %v\n%s", err, stdout)
	}
	if len(got) != 2 {
		t.Fatalf("JSON có %d phần tử, muốn 2:\n%s", len(got), stdout)
	}
	if got[0].Provider != "p" || got[0].ID != "x" {
		t.Fatalf("phần tử 0 = %+v, muốn {p x}", got[0])
	}
	if got[1].ID != "y" {
		t.Fatalf("phần tử 1 = %+v, muốn id y", got[1])
	}
}

func TestModelsTTLExpiredFetches(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"net"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	e.seedCache(t, "p", []string{"old"}, 2*time.Hour)
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "models", "p")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "p/net" {
		t.Fatalf("stdout = %q, muốn p/net (cache hết hạn phải fetch)", got)
	}

	cache, err := provider.LoadCache(e.cacheDir, "p")
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if len(cache.Models) != 1 || cache.Models[0] != "net" {
		t.Fatalf("cache không được cập nhật: %+v", cache)
	}
}

func TestModelsCustomTTLFlag(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"net"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	e.seedCache(t, "p", []string{"cached"}, 20*time.Minute)
	t.Setenv("P_KEY", "sk-p")

	// TTL 10m < tuổi cache 20m => phải fetch lại.
	code, stdout, stderr := runCLI(t, "", "models", "p", "--ttl", "10m")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "p/net" {
		t.Fatalf("stdout = %q, muốn p/net", got)
	}
}

func TestModelsCustomTTLKeepsFreshCache(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://api.deeprelay.example/v1
    api_key: env:DEEPRELAY_KEY
`)
	t.Setenv("DEEPRELAY_KEY", "sk-x")
	e.seedCache(t, "deeprelay", []string{"cached"}, 20*time.Minute)

	code, stdout, stderr := runCLI(t, "", "models", "deeprelay", "--ttl", "1h")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "deeprelay/cached" {
		t.Fatalf("stdout = %q, muốn deeprelay/cached", got)
	}
}

func TestModelsAllProvidersFromCache(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  zed:
    kind: openai
    base_url: https://z.example/v1
    api_key: env:Z_KEY
  alpha:
    kind: openai
    base_url: https://a.example/v1
    api_key: env:A_KEY
subscriptions:
  claude:
    harness: claude
`)
	e.seedCache(t, "zed", []string{"z1"}, time.Minute)
	e.seedCache(t, "alpha", []string{"a2", "a1"}, time.Minute)

	code, stdout, stderr := runCLI(t, "", "models")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"alpha/a1", "alpha/a2", "zed/z1"}
	if len(lines) != len(want) {
		t.Fatalf("stdout = %q, muốn %v", stdout, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("stdout = %q, muốn %v (đã sort)", stdout, want)
		}
	}
	// Subscription không được xuất hiện.
	if strings.Contains(stdout, "claude") {
		t.Fatalf("models gộp có subscription: %q", stdout)
	}
}

func TestModelsAllProvidersMissingCacheWarnsAndExitsZero(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: https://p.example/v1
    api_key: env:P_KEY
`)
	code, stdout, stderr := runCLI(t, "", "models")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 khi mọi provider thiếu cache", code)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, muốn rỗng", stdout)
	}
	if !strings.Contains(stderr, "p") {
		t.Fatalf("stderr không cảnh báo provider p thiếu cache: %q", stderr)
	}
}

func TestModelsAllProvidersIgnoresTTL(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: https://p.example/v1
    api_key: env:P_KEY
`)
	// Cache cũ hơn TTL mặc định nhưng vẫn phải được dùng khi không chỉ định provider.
	e.seedCache(t, "p", []string{"old"}, 48*time.Hour)

	code, stdout, stderr := runCLI(t, "", "models")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "p/old" {
		t.Fatalf("stdout = %q, muốn p/old", got)
	}
}

func TestModelsRefreshWithoutProviderIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	e.seedCache(t, "deeprelay", []string{"m"}, time.Minute)

	code, stdout, stderr := runCLI(t, "", "models", "--refresh")
	if code == 0 {
		t.Fatalf("models --refresh không có provider exit 0 (stdout=%q)", stdout)
	}
	if !strings.Contains(stderr, "--refresh") {
		t.Fatalf("lỗi không nhắc --refresh: %q", stderr)
	}
}

func TestModelsTTLWithoutProviderIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	e.seedCache(t, "deeprelay", []string{"m"}, time.Minute)

	code, stdout, stderr := runCLI(t, "", "models", "--ttl", "5m")
	if code == 0 {
		t.Fatalf("models --ttl không có provider exit 0 (stdout=%q)", stdout)
	}
	if !strings.Contains(stderr, "--ttl") {
		t.Fatalf("lỗi không nhắc --ttl: %q", stderr)
	}
}

func TestModelsUnknownProviderIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, _ := runCLI(t, "", "models", "nonexistent")
	if code == 0 {
		t.Fatalf("models provider lạ exit 0 (stdout=%q)", stdout)
	}
}

func TestModelsSubscriptionIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, _ := runCLI(t, "", "models", "claude")
	if code == 0 {
		t.Fatalf("models subscription exit 0 (stdout=%q)", stdout)
	}
}

func TestModelsCacheDirCreated(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-p")
	if e.exists(e.cacheDir) {
		t.Fatal("cache dir đã tồn tại trước test")
	}

	code, stdout, stderr := runCLI(t, "", "models", "p", "--refresh")
	if code != 0 {
		t.Fatalf("exit code = %d (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(e.cacheDir, "p.json")); err != nil {
		t.Fatalf("cache p.json không được tạo trong HI_LLM_CACHE_DIR: %v", err)
	}
}

func TestModelsNetworkErrorExitsNonZero(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusUnauthorized)
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "models", "p", "--refresh")
	if code == 0 {
		t.Fatalf("models gặp 401 exit 0 (stdout=%q)", stdout)
	}
	if !strings.Contains(stderr, "401") {
		t.Fatalf("stderr không nêu status 401: %q", stderr)
	}
}

func TestModelsMissingCacheSingleProviderFetches(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"one"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "models", "p")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "p/one" {
		t.Fatalf("stdout = %q, muốn p/one", got)
	}
}
