package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hi-tools/internal/llm/provider"
)

// hookWriter là io.Writer an toàn goroutine; hook được gọi ngay trước mỗi lần ghi
// để test phát hiện việc ghi stderr khi provider khác còn đang ping.
type hookWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	hook func()
}

func (w *hookWriter) Write(p []byte) (int, error) {
	if w.hook != nil {
		w.hook()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *hookWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// pingLines trả các dòng của bảng kết quả (bỏ dòng header).
func pingLines(t *testing.T, stdout string) []string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NAME") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func TestPingTwoProvidersTableExitAndCache(t *testing.T) {
	e := newCLIEnv(t)

	modelsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, muốn /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"},{"id":"m3"}]}`))
	}))
	defer modelsSrv.Close()

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer authSrv.Close()

	e.writeRegistry(t, `providers:
  zeta:
    kind: openai
    base_url: `+modelsSrv.URL+`/v1
    api_key: env:ZETA_KEY
  alpha:
    kind: openai
    base_url: `+authSrv.URL+`/v1
    api_key: env:ALPHA_KEY
subscriptions:
  claude:
    harness: claude
`)
	t.Setenv("ZETA_KEY", "sk-zeta")
	t.Setenv("ALPHA_KEY", "sk-alpha")

	code, stdout, stderr := runCLI(t, "", "ping")
	if code != 1 {
		t.Fatalf("exit code = %d, muốn 1 vì có provider lỗi (stdout=%q stderr=%q)", code, stdout, stderr)
	}

	lines := pingLines(t, stdout)
	if len(lines) != 2 {
		t.Fatalf("bảng có %d dòng dữ liệu, muốn 2 (bỏ qua subscription):\n%s", len(lines), stdout)
	}
	if strings.Contains(stdout, "claude") {
		t.Fatalf("bảng chứa subscription claude (phải bỏ qua):\n%s", stdout)
	}
	// In theo thứ tự tên: alpha trước zeta.
	if !strings.HasPrefix(lines[0], "alpha") || !strings.HasPrefix(lines[1], "zeta") {
		t.Fatalf("bảng không in theo thứ tự tên:\n%s", stdout)
	}
	if !strings.Contains(lines[0], "❌") || !strings.Contains(lines[0], "auth") {
		t.Fatalf("dòng alpha = %q, muốn ❌ auth", lines[0])
	}
	if !strings.Contains(lines[1], "✅") || !strings.Contains(lines[1], "3 models") {
		t.Fatalf("dòng zeta = %q, muốn ✅ 3 models", lines[1])
	}
	for _, col := range []string{"NAME", "STATUS", "LATENCY", "DETAIL"} {
		if !strings.Contains(stdout, col) {
			t.Fatalf("bảng thiếu cột %s:\n%s", col, stdout)
		}
	}

	// _ping.json có cả hai kết quả (merge).
	pings := provider.LoadPings(e.cacheDir)
	if len(pings) != 2 {
		t.Fatalf("_ping.json có %d entry, muốn 2: %+v", len(pings), pings)
	}
	if pings["alpha"].OK || pings["alpha"].Kind != "auth" {
		t.Fatalf("ping alpha = %+v, muốn OK=false kind=auth", pings["alpha"])
	}
	if !pings["zeta"].OK || pings["zeta"].LatencyMS < 0 {
		t.Fatalf("ping zeta = %+v, muốn OK=true", pings["zeta"])
	}

	// Provider ok phải được ghi cache model.
	c, err := provider.LoadCache(e.cacheDir, "zeta")
	if err != nil {
		t.Fatalf("cache model zeta không được ghi: %v", err)
	}
	if len(c.Models) != 3 {
		t.Fatalf("cache model zeta = %v, muốn 3 model", c.Models)
	}
}

func TestPingEnvKeyUnsetIsKeyErrorNoRequest(t *testing.T) {
	e := newCLIEnv(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:HI_LLM_PING_UNSET_KEY
`)
	t.Setenv("HI_LLM_PING_UNSET_KEY", "")

	code, stdout, stderr := runCLI(t, "", "ping")
	if code != 1 {
		t.Fatalf("exit code = %d, muốn 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("provider thiếu key vẫn gửi %d request", got)
	}

	line := ""
	for _, l := range pingLines(t, stdout) {
		if strings.HasPrefix(l, "p") {
			line = l
		}
	}
	if !strings.Contains(line, "❌") || !strings.Contains(line, "key") {
		t.Fatalf("dòng p = %q, muốn ❌ key", line)
	}
	// Lỗi trước khi gửi request → LATENCY "-": cột thứ 3 của bảng.
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[2] != "-" {
		t.Fatalf("dòng p = %q, muốn cột LATENCY = - khi không gửi request", line)
	}

	pings := provider.LoadPings(e.cacheDir)
	if pings["p"].Kind != "key" {
		t.Fatalf("ping p = %+v, muốn kind=key", pings["p"])
	}
}

func TestPingUnknownProviderExit2(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "ping", "khong-co")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 với provider lạ (stdout=%q stderr=%q)", code, stdout, stderr)
	}
}

func TestPingSubscriptionExit2(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "ping", "claude")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 với subscription (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "subscription") {
		t.Fatalf("lỗi không nêu subscription: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPingChatRequiresExactlyOneProvider(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p1:
    kind: openai
    base_url: https://p1.example/v1
    api_key: env:P1_KEY
  p2:
    kind: openai
    base_url: https://p2.example/v1
    api_key: env:P2_KEY
`)
	t.Setenv("P1_KEY", "sk-1")
	t.Setenv("P2_KEY", "sk-2")

	// Không chỉ định provider.
	code, stdout, stderr := runCLI(t, "", "ping", "--chat", "m")
	if code != 2 {
		t.Fatalf("--chat không provider exit %d, muốn 2 (stdout=%q stderr=%q)", code, stdout, stderr)
	}

	// Hai provider.
	code, stdout, stderr = runCLI(t, "", "ping", "p1", "p2", "--chat", "m")
	if code != 2 {
		t.Fatalf("--chat với 2 provider exit %d, muốn 2 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
}

func TestPingChatSendsModelAndSkipsModelCache(t *testing.T) {
	e := newCLIEnv(t)

	var (
		gotPath string
		gotBody map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null}}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "ping", "p", "--chat", "deepseek-v4.1-flash")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, muốn /v1/chat/completions", gotPath)
	}
	if got, _ := gotBody["model"].(string); got != "deepseek-v4.1-flash" {
		t.Fatalf("body.model = %q, muốn deepseek-v4.1-flash", got)
	}

	lines := pingLines(t, stdout)
	if len(lines) != 1 || !strings.Contains(lines[0], "✅") || !strings.Contains(lines[0], "chat ok") {
		t.Fatalf("bảng = %q, muốn một dòng ✅ chat ok", stdout)
	}

	// Chế độ --chat không ghi cache model.
	if _, err := provider.LoadCache(e.cacheDir, "p"); err == nil {
		t.Fatal("--chat đã ghi cache model (không được)")
	}
}

func TestPingSendsProviderHeaders(t *testing.T) {
	e := newCLIEnv(t)

	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-opencode-session")
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
    headers:
      x-opencode-session: 3f2c-uuid
`)
	t.Setenv("P_KEY", "sk-p")

	code, _, stderr := runCLI(t, "", "ping", "p")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	if gotHeader != "3f2c-uuid" {
		t.Fatalf("header tới server = %q, muốn 3f2c-uuid", gotHeader)
	}
}

func TestPingOnlyNamedProviders(t *testing.T) {
	e := newCLIEnv(t)

	var otherCalls int32
	otherSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&otherCalls, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer otherSrv.Close()

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer okSrv.Close()

	e.writeRegistry(t, `providers:
  other:
    kind: openai
    base_url: `+otherSrv.URL+`/v1
    api_key: env:OTHER_KEY
  p:
    kind: openai
    base_url: `+okSrv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("OTHER_KEY", "sk-o")
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "ping", "p")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if got := atomic.LoadInt32(&otherCalls); got != 0 {
		t.Fatalf("ping p nhưng provider other bị gọi %d lần", got)
	}
	if strings.Contains(stdout, "other") {
		t.Fatalf("bảng chứa provider không được chỉ định:\n%s", stdout)
	}

	// _ping.json chỉ có p.
	pings := provider.LoadPings(e.cacheDir)
	if len(pings) != 1 {
		t.Fatalf("_ping.json = %+v, muốn chỉ có p", pings)
	}
	if _, ok := pings["p"]; !ok {
		t.Fatalf("_ping.json thiếu p: %+v", pings)
	}
}

func TestPingEmptyRegistryExit0WithGuidance(t *testing.T) {
	newCLIEnv(t)

	code, stdout, stderr := runCLI(t, "", "ping")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 với registry rỗng (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "hi-llm add") {
		t.Fatalf("registry rỗng không in hướng dẫn hi-llm add: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPingDoesNotLeakKey(t *testing.T) {
	e := newCLIEnv(t)

	const secret = "sk-MUST-NOT-LEAK-IN-PING"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", secret)

	code, stdout, stderr := runCLI(t, "", "ping")
	if code != 1 {
		t.Fatalf("exit code = %d, muốn 1", code)
	}
	if strings.Contains(stdout+stderr, secret) {
		t.Fatalf("output làm lộ api key: stdout=%q stderr=%q", stdout, stderr)
	}
	pings := provider.LoadPings(e.cacheDir)
	if strings.Contains(pings["p"].Error, secret) {
		t.Fatalf("_ping.json làm lộ api key: %q", pings["p"].Error)
	}
}

// --- Kiểm tham số phải xảy ra trước khi xét registry rỗng ---

func TestPingUnknownProviderExit2BeforeEmptyRegistryCheck(t *testing.T) {
	newCLIEnv(t) // registry rỗng

	code, stdout, stderr := runCLI(t, "", "ping", "khong-co")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 với provider lạ ngay cả khi registry rỗng (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "khong-co") {
		t.Fatalf("lỗi không nêu tên provider lạ: stdout=%q stderr=%q", stdout, stderr)
	}
	if strings.Contains(stdout, "hi-llm add") {
		t.Fatalf("registry rỗng đã in hướng dẫn thay vì báo lỗi tham số: stdout=%q", stdout)
	}
}

func TestPingSubscriptionExit2BeforeEmptyRegistryCheck(t *testing.T) {
	e := newCLIEnv(t)
	// Chỉ có subscription, không có provider nào.
	e.writeRegistry(t, `subscriptions:
  claude:
    harness: claude
`)

	code, stdout, stderr := runCLI(t, "", "ping", "claude")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 với subscription dù registry không có provider (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "subscription") {
		t.Fatalf("lỗi không nêu subscription: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPingChatWithoutProviderExit2BeforeEmptyRegistryCheck(t *testing.T) {
	newCLIEnv(t) // registry rỗng

	code, stdout, stderr := runCLI(t, "", "ping", "--chat", "m")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 khi --chat không chỉ định provider (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "--chat") {
		t.Fatalf("lỗi không nêu --chat: stdout=%q stderr=%q", stdout, stderr)
	}
}

// --- Bỏ trùng tên provider trong tham số ---

func TestPingDuplicateNamesPrintOneRow(t *testing.T) {
	e := newCLIEnv(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  p1:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: env:P1_KEY
`)
	t.Setenv("P1_KEY", "sk-1")

	code, stdout, stderr := runCLI(t, "", "ping", "p1", "p1")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	lines := pingLines(t, stdout)
	if len(lines) != 1 {
		t.Fatalf("bảng có %d dòng cho `ping p1 p1`, muốn 1:\n%s", len(lines), stdout)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("ping p1 p1 gửi %d request, muốn 1", got)
	}
	if len(provider.LoadPings(e.cacheDir)) != 1 {
		t.Fatalf("_ping.json = %+v, muốn chỉ có p1", provider.LoadPings(e.cacheDir))
	}
}

// --- Không ghi stderr trong goroutine ping ---

// TestPingWritesSaveCacheErrorAfterWait kiểm tra lỗi SaveCache được gom lại và in
// sau wg.Wait(): khi một provider còn đang ping thì stderr phải chưa có gì.
func TestPingWritesSaveCacheErrorAfterWait(t *testing.T) {
	e := newCLIEnv(t)

	fastDone := make(chan struct{})
	var fastOnce sync.Once
	fastSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
		fastOnce.Do(func() { close(fastDone) })
	}))
	defer fastSrv.Close()

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	var slowDone atomic.Bool
	var once sync.Once
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(slowStarted) })
		<-releaseSlow
		slowDone.Store(true)
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer slowSrv.Close()

	e.writeRegistry(t, `providers:
  bad:
    kind: openai
    base_url: `+fastSrv.URL+`/v1
    api_key: env:BAD_KEY
  slow:
    kind: openai
    base_url: `+slowSrv.URL+`/v1
    api_key: env:SLOW_KEY
`)
	t.Setenv("BAD_KEY", "sk-bad")
	t.Setenv("SLOW_KEY", "sk-slow")

	// HI_LLM_CACHE_DIR là một file thường → SaveCache của provider "bad" luôn lỗi.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("tạo file chặn cache dir: %v", err)
	}
	t.Setenv("HI_LLM_CACHE_DIR", blocker)

	var wroteEarly atomic.Bool
	errOut := &hookWriter{hook: func() {
		if !slowDone.Load() {
			wroteEarly.Store(true)
		}
	}}

	done := make(chan int, 1)
	go func() {
		done <- run([]string{"ping"}, strings.NewReader(""), io.Discard, errOut)
	}()

	select {
	case <-slowStarted:
	case <-time.After(5 * time.Second):
		close(releaseSlow)
		t.Fatal("provider slow không được ping (ping không chạy song song?)")
	}
	select {
	case <-fastDone:
	case <-time.After(5 * time.Second):
		close(releaseSlow)
		t.Fatal("provider bad không được ping")
	}

	// Provider bad đã xong request; cho nó thời gian gọi SaveCache (sẽ lỗi).
	time.Sleep(500 * time.Millisecond)
	early := wroteEarly.Load()
	close(releaseSlow)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ping không kết thúc sau khi provider slow trả lời")
	}

	if early || wroteEarly.Load() {
		t.Fatalf("ping ghi stderr trong goroutine khi provider khác còn đang chạy; stderr=%q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "bad") && !strings.Contains(errOut.String(), "cache") {
		t.Fatalf("lỗi ghi cache của provider bad không được in sau wg.Wait(): stderr=%q", errOut.String())
	}
}
