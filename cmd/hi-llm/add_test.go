package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/registry"
)

func TestAddMissingValuesNoTTY(t *testing.T) {
	e := newCLIEnv(t)

	cases := []struct {
		name    string
		args    []string
		wantArg string
	}{
		{
			name:    "thiếu name",
			args:    []string{"add", "--base-url", "https://api.example.com/v1", "--api-key-stdin"},
			wantArg: "--name",
		},
		{
			name:    "thiếu base-url",
			args:    []string{"add", "--name", "p", "--api-key-stdin"},
			wantArg: "--base-url",
		},
		{
			name:    "thiếu key",
			args:    []string{"add", "--name", "p", "--base-url", "https://api.example.com/v1"},
			wantArg: "--api-key",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, "sk-abc\n", tc.args...)
			if code == 0 {
				t.Fatalf("exit code = 0, muốn ≠ 0 (stdout=%q)", stdout)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, tc.wantArg) {
				t.Fatalf("lỗi không nêu %s: %q", tc.wantArg, combined)
			}
			if e.exists(e.providersPath()) {
				t.Fatalf("providers.yaml được ghi dù thiếu %s", tc.wantArg)
			}
		})
	}
}

func TestAddStdinAndEnvExclusive(t *testing.T) {
	e := newCLIEnv(t)
	code, stdout, stderr := runCLI(t, "sk-abc\n",
		"add", "--name", "p", "--base-url", "https://api.example.com/v1",
		"--api-key-stdin", "--api-key-env", "FOO", "--skip-verify",
	)
	if code == 0 {
		t.Fatalf("exit code = 0 khi dùng cả --api-key-stdin và --api-key-env (stdout=%q)", stdout)
	}
	if !strings.Contains(stdout+stderr, "--api-key") {
		t.Fatalf("lỗi không nêu xung đột flag: %q", stdout+stderr)
	}
	if e.exists(e.providersPath()) {
		t.Fatal("providers.yaml được ghi dù flag xung đột")
	}
}

func TestAddAPIKeyStdinStoresEnc(t *testing.T) {
	e := newCLIEnv(t)
	code, stdout, stderr := runCLI(t, "  sk-from-stdin \n",
		"add", "--name", "deeprelay", "--base-url", "https://api.example.com/v1",
		"--api-key-stdin", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}

	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load providers.yaml: %v", err)
	}
	p, ok := reg.Providers["deeprelay"]
	if !ok {
		t.Fatalf("thiếu provider deeprelay: %+v", reg.Providers)
	}
	if p.APIKey != "enc:deeprelay" {
		t.Fatalf("api_key = %q, muốn enc:deeprelay", p.APIKey)
	}
	if p.Kind != "openai" {
		t.Fatalf("kind = %q, muốn openai", p.Kind)
	}
	if p.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base_url = %q, muốn https://api.example.com/v1", p.BaseURL)
	}

	if !e.exists(e.keysPath()) {
		t.Fatal("--api-key-stdin không tạo keys.enc")
	}
	store := keystore.NewEncStore(e.keysPath(), keystore.EnvPassword())
	got, err := store.Get("deeprelay")
	if err != nil {
		t.Fatalf("Get key vừa lưu: %v", err)
	}
	if got != "sk-from-stdin" {
		t.Fatalf("key lưu = %q, muốn sk-from-stdin (phải trim)", got)
	}
}

func TestAddRegistryHasNoSecret(t *testing.T) {
	e := newCLIEnv(t)
	const secret = "sk-SHOULD-NOT-BE-IN-YAML"
	code, _, stderr := runCLI(t, secret+"\n",
		"add", "--name", "p", "--base-url", "https://api.example.com/v1",
		"--api-key-stdin", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr)
	}
	data := e.readFile(t, e.providersPath())
	if strings.Contains(data, secret) {
		t.Fatalf("providers.yaml chứa secret:\n%s", data)
	}
	if !strings.Contains(data, "enc:p") {
		t.Fatalf("providers.yaml thiếu con trỏ enc:p:\n%s", data)
	}
}

func TestAddAPIKeyEnvDoesNotCreateKeystore(t *testing.T) {
	e := newCLIEnv(t)
	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "deeprelay", "--base-url", "https://api.example.com/v1",
		"--api-key-env", "DEEPRELAY_KEY", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reg.Providers["deeprelay"].APIKey; got != "env:DEEPRELAY_KEY" {
		t.Fatalf("api_key = %q, muốn env:DEEPRELAY_KEY", got)
	}
	if e.exists(e.keysPath()) {
		t.Fatal("--api-key-env đã tạo keys.enc (không được ghi keystore)")
	}
}

func TestAddDuplicateNameNeedsForce(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "sk-new\n",
		"add", "--name", "deeprelay", "--base-url", "https://api.example.com/v1",
		"--api-key-env", "NEW_KEY", "--skip-verify",
	)
	if code == 0 {
		t.Fatalf("add trùng tên không --force exit 0 (stdout=%q)", stdout)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--force") {
		t.Fatalf("lỗi không nêu --force: %q", combined)
	}

	// Không được thay đổi registry cũ.
	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reg.Providers["deeprelay"].APIKey; got != "enc:deeprelay" {
		t.Fatalf("registry cũ bị thay đổi: api_key = %q", got)
	}
}

func TestAddDuplicateNameWithForceOverwrites(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "deeprelay", "--base-url", "https://api.example.com/v2",
		"--api-key-env", "NEW_KEY", "--force", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := reg.Providers["deeprelay"]
	if p.APIKey != "env:NEW_KEY" || p.BaseURL != "https://api.example.com/v2" {
		t.Fatalf("provider không được ghi đè: %+v", p)
	}
	// Subscription không bị mất.
	if !reg.IsSubscription("claude") {
		t.Fatalf("subscription claude bị mất: %+v", reg.Subscriptions)
	}
}

func TestAddSkipVerifyDoesNotCallNetwork(t *testing.T) {
	e := newCLIEnv(t)
	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "p", "--base-url", srv.URL+"/v1",
		"--api-key-env", "P_KEY", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if called != 0 {
		t.Fatalf("--skip-verify vẫn gọi mạng %d lần", called)
	}
	if !e.exists(e.providersPath()) {
		t.Fatal("--skip-verify không ghi providers.yaml")
	}
}

func TestAddVerifySuccessWrites(t *testing.T) {
	e := newCLIEnv(t)
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	code, stdout, stderr := runCLI(t, "sk-verify-ok\n",
		"add", "--name", "p", "--base-url", srv.URL+"/v1", "--api-key-stdin",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if gotAuth != "Bearer sk-verify-ok" {
		t.Fatalf("Authorization = %q, muốn Bearer sk-verify-ok", gotAuth)
	}
	if !e.exists(e.providersPath()) {
		t.Fatal("verify thành công nhưng không ghi providers.yaml")
	}
}

func TestAddVerifyFailWritesNothing(t *testing.T) {
	e := newCLIEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	code, stdout, stderr := runCLI(t, "sk-bad\n",
		"add", "--name", "p", "--base-url", srv.URL+"/v1", "--api-key-stdin",
	)
	if code == 0 {
		t.Fatalf("verify fail exit 0 (stdout=%q)", stdout)
	}
	combined := stderr + stdout
	if !strings.Contains(combined, "401") && !strings.Contains(strings.ToLower(combined), "unauthorized") {
		t.Fatalf("cảnh báo không nêu status/lỗi xác minh: %q", combined)
	}
	if strings.Contains(combined, "sk-bad") {
		t.Fatalf("output làm lộ key: %q", combined)
	}
	if e.exists(e.providersPath()) {
		t.Fatal("verify fail nhưng providers.yaml vẫn được ghi")
	}
	if e.exists(e.keysPath()) {
		t.Fatal("verify fail nhưng keys.enc vẫn được ghi")
	}
}

func TestAddNormalizesBaseURL(t *testing.T) {
	e := newCLIEnv(t)
	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "p", "--base-url", "https://api.example.com/",
		"--api-key-env", "P_KEY", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reg.Providers["p"].BaseURL; got != "https://api.example.com/v1" {
		t.Fatalf("base_url = %q, muốn https://api.example.com/v1", got)
	}
}

func TestAddRejectsBadName(t *testing.T) {
	e := newCLIEnv(t)
	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "Bad/Name", "--base-url", "https://api.example.com/v1",
		"--api-key-env", "P_KEY", "--skip-verify",
	)
	if code == 0 {
		t.Fatalf("add tên xấu exit 0 (stdout=%q)", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatalf("add tên xấu không in lỗi ra stderr (stdout=%q)", stdout)
	}
	if e.exists(e.providersPath()) {
		t.Fatal("providers.yaml được ghi dù tên xấu")
	}
}

func TestAddRejectsBadKind(t *testing.T) {
	e := newCLIEnv(t)
	code, stdout, _ := runCLI(t, "",
		"add", "--name", "p", "--base-url", "https://api.example.com/v1",
		"--kind", "anthropic", "--api-key-env", "P_KEY", "--skip-verify",
	)
	if code == 0 {
		t.Fatalf("add kind anthropic exit 0 (stdout=%q)", stdout)
	}
	if e.exists(e.providersPath()) {
		t.Fatal("providers.yaml được ghi dù kind lạ")
	}
}

func TestAddCreatesMissingHome(t *testing.T) {
	e := newCLIEnv(t)
	if e.exists(e.home) {
		t.Fatal("home đã tồn tại trước test")
	}
	code, stdout, stderr := runCLI(t, "sk-ok\n",
		"add", "--name", "p", "--base-url", "https://api.example.com/v1",
		"--api-key-stdin", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d với home chưa tồn tại (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !e.exists(e.providersPath()) {
		t.Fatal("providers.yaml không được tạo trong home mới")
	}
	if strings.Contains(strings.ToLower(stderr), "no such file") {
		t.Fatalf("lỗi ENOENT khi home chưa tồn tại: %q", stderr)
	}
}

func TestAddSubscriptionNameRejected(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, _ := runCLI(t, "",
		"add", "--name", "claude", "--base-url", "https://api.example.com/v1",
		"--api-key-env", "C_KEY", "--skip-verify",
	)
	if code == 0 {
		t.Fatalf("add trùng tên subscription exit 0 (stdout=%q)", stdout)
	}
}

func TestAddLsRejectExtraArgs(t *testing.T) {
	e := newCLIEnv(t)
	for _, args := range [][]string{
		{"add", "deeprelay", "--name", "p", "--base-url", "https://x.example", "--api-key-env", "X", "--skip-verify"},
		{"ls", "extra"},
	} {
		code, _, stderr := runCLI(t, "", args...)
		if code != 2 || !strings.Contains(stderr, "thừa") {
			t.Fatalf("%v: code=%d stderr=%q, muốn exit 2 báo tham số thừa", args, code, stderr)
		}
	}
	if e.exists(e.providersPath()) {
		t.Fatal("tham số thừa nhưng providers.yaml vẫn được ghi")
	}
}

func TestAddEnvUnsetWarnsNoTTY(t *testing.T) {
	e := newCLIEnv(t)
	t.Setenv("HI_LLM_TEST_UNSET", "")
	code, _, stderr := runCLI(t, "", "add", "--name", "p", "--base-url", "https://x.example", "--api-key-env", "HI_LLM_TEST_UNSET")
	if code != 1 || !strings.Contains(stderr, "cảnh báo") || !strings.Contains(stderr, "chưa đặt") {
		t.Fatalf("code=%d stderr=%q, muốn exit 1 + cảnh báo biến chưa đặt", code, stderr)
	}
	if e.exists(e.providersPath()) {
		t.Fatal("biến env chưa đặt (không TTY) nhưng providers.yaml vẫn được ghi")
	}
}

func TestAddForceKeepsHeaders(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  opencode-go:
    kind: openai
    base_url: https://old.example/v1
    api_key: enc:opencode-go
    headers:
      x-opencode-session: 3f2c-uuid
subscriptions:
  claude:
    harness: claude
`)

	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "opencode-go", "--base-url", "https://new.example/v1",
		"--api-key-env", "NEW_KEY", "--force", "--skip-verify",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}

	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := reg.Providers["opencode-go"]
	if p.BaseURL != "https://new.example/v1" || p.APIKey != "env:NEW_KEY" {
		t.Fatalf("provider không được ghi đè: %+v", p)
	}
	if got := p.Headers["x-opencode-session"]; got != "3f2c-uuid" {
		t.Fatalf("--force làm mất headers cũ: %+v", p.Headers)
	}
}

func TestAddForceVerifyUsesKeptHeaders(t *testing.T) {
	e := newCLIEnv(t)

	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-opencode-session")
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()

	e.writeRegistry(t, `providers:
  opencode-go:
    kind: openai
    base_url: `+srv.URL+`/v1
    api_key: enc:opencode-go
    headers:
      x-opencode-session: 3f2c-uuid
`)
	t.Setenv("NEW_KEY", "sk-new")

	code, stdout, stderr := runCLI(t, "",
		"add", "--name", "opencode-go", "--base-url", srv.URL+"/v1",
		"--api-key-env", "NEW_KEY", "--force",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if gotHeader != "3f2c-uuid" {
		t.Fatalf("verify không gửi header cũ của provider: nhận %q", gotHeader)
	}

	reg, err := registry.Load(e.providersPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reg.Providers["opencode-go"].Headers["x-opencode-session"]; got != "3f2c-uuid" {
		t.Fatalf("headers không được giữ sau verify: %+v", reg.Providers["opencode-go"].Headers)
	}
}

func TestReadLineDoesNotReadAhead(t *testing.T) {
	r := strings.NewReader("tên\nsk-key\n")
	if got, _ := readLine(r); got != "tên" {
		t.Fatalf("dòng 1 = %q", got)
	}
	if r.Len() != len("sk-key\n") {
		t.Fatalf("readLine đọc lố: còn %d byte, muốn %d", r.Len(), len("sk-key\n"))
	}
	if got, _ := readLine(r); got != "sk-key" {
		t.Fatalf("dòng 2 = %q", got)
	}
}
