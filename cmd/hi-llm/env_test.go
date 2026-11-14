package main

import (
	"path/filepath"
	"strings"
	"testing"

	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/registry"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abc", "'abc'"},
		{"", "''"},
		{"sk-123", "'sk-123'"},
		{"a'b", `'a'\''b'`},
		{"'", `''\'''`},
	}
	for _, tc := range cases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Fatalf("shellQuote(%q) = %q, muốn %q", tc.in, got, tc.want)
		}
	}
}

func TestEnvLinesOpenAI(t *testing.T) {
	lines, err := envLines("openai", "https://api.example.com/v1", "sk-key")
	if err != nil {
		t.Fatalf("envLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "export OPENAI_API_KEY='sk-key'") {
		t.Fatalf("thiếu OPENAI_API_KEY:\n%s", joined)
	}
	if !strings.Contains(joined, "export OPENAI_BASE_URL='https://api.example.com/v1'") {
		t.Fatalf("thiếu OPENAI_BASE_URL (giữ nguyên /v1):\n%s", joined)
	}
	if strings.Contains(joined, "ANTHROPIC") {
		t.Fatalf("style openai không được in biến ANTHROPIC:\n%s", joined)
	}
}

func TestEnvLinesAnthropicStripsV1(t *testing.T) {
	lines, err := envLines("anthropic", "https://api.example.com/v1", "sk-key")
	if err != nil {
		t.Fatalf("envLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "export ANTHROPIC_AUTH_TOKEN='sk-key'") {
		t.Fatalf("thiếu ANTHROPIC_AUTH_TOKEN:\n%s", joined)
	}
	if !strings.Contains(joined, "export ANTHROPIC_BASE_URL='https://api.example.com'") {
		t.Fatalf("ANTHROPIC_BASE_URL phải bỏ /v1:\n%s", joined)
	}
	if strings.Contains(joined, "/v1") {
		t.Fatalf("output anthropic còn /v1:\n%s", joined)
	}
	if strings.Contains(joined, "OPENAI") {
		t.Fatalf("style anthropic không được in biến OPENAI:\n%s", joined)
	}
}

func TestEnvLinesAnthropicKeepsURLWithoutV1(t *testing.T) {
	lines, err := envLines("anthropic", "https://api.example.com/openai", "sk-key")
	if err != nil {
		t.Fatalf("envLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "export ANTHROPIC_BASE_URL='https://api.example.com/openai'") {
		t.Fatalf("URL không kết thúc /v1 phải giữ nguyên:\n%s", joined)
	}
}

func TestEnvLinesRejectsBadStyle(t *testing.T) {
	if _, err := envLines("gemini", "https://api.example.com/v1", "k"); err == nil {
		t.Fatal("envLines style lạ không trả lỗi")
	}
}

func TestEnvRunOpenAIDefaultStyle(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	store := keystore.NewEncStore(e.keysPath(), keystore.EnvPassword())
	if err := store.Set("deeprelay", "sk-live-secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	code, stdout, stderr := runCLI(t, "", "env", "deeprelay")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "export OPENAI_API_KEY='sk-live-secret'") {
		t.Fatalf("stdout thiếu OPENAI_API_KEY:\n%s", stdout)
	}
	if !strings.Contains(stdout, "export OPENAI_BASE_URL='https://api.deeprelay.example/v1'") {
		t.Fatalf("stdout thiếu OPENAI_BASE_URL:\n%s", stdout)
	}
}

func TestEnvFlagAfterPositional(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	store := keystore.NewEncStore(e.keysPath(), keystore.EnvPassword())
	if err := store.Set("deeprelay", "sk-live"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	code, stdout, stderr := runCLI(t, "", "env", "deeprelay", "--style", "anthropic")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("flag --style đặt sau positional không được nhận:\n%s", stdout)
	}
	if !strings.Contains(stdout, "ANTHROPIC_BASE_URL='https://api.deeprelay.example'") {
		t.Fatalf("ANTHROPIC_BASE_URL chưa bỏ /v1:\n%s", stdout)
	}
}

func TestEnvQuotesEmbeddedSingleQuote(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: https://p.example/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-with'quote")

	code, stdout, stderr := runCLI(t, "", "env", "p")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, `export OPENAI_API_KEY='sk-with'\''quote'`) {
		t.Fatalf("quoting nháy đơn sai:\n%s", stdout)
	}
}

func TestEnvSubscriptionIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "env", "claude")
	if code == 0 {
		t.Fatalf("env subscription exit 0 (stdout=%q)", stdout)
	}
	if !strings.Contains(stderr, "provider này là subscription CLI, không có env") {
		t.Fatalf("stderr = %q, muốn nêu provider này là subscription CLI, không có env", stderr)
	}
}

func TestEnvUnknownProviderIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, _ := runCLI(t, "", "env", "nope")
	if code == 0 {
		t.Fatalf("env provider lạ exit 0 (stdout=%q)", stdout)
	}
}

func TestEnvBadStyleIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, _ := runCLI(t, "", "env", "deeprelay", "--style", "gemini")
	if code == 0 {
		t.Fatalf("env style lạ exit 0 (stdout=%q)", stdout)
	}
}

func TestEnvMissingArgIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, _ := runCLI(t, "", "env")
	if code == 0 {
		t.Fatalf("env thiếu provider exit 0 (stdout=%q)", stdout)
	}
}

func TestEnvMissingKeyIsError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay) // enc:deeprelay nhưng chưa có keys.enc

	code, stdout, stderr := runCLI(t, "", "env", "deeprelay")
	if code == 0 {
		t.Fatalf("env thiếu key exit 0 (stdout=%q)", stdout)
	}
	if strings.Contains(stdout, "sk-") {
		t.Fatalf("stdout có secret dù lỗi:\n%s", stdout)
	}
	if !strings.Contains(stderr, "keys.enc") {
		t.Fatalf("stderr không nêu keys.enc: %q", stderr)
	}
}

func TestEnvRegistryPathUsesHome(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: https://p.example/v1
    api_key: env:P_KEY
`)
	t.Setenv("P_KEY", "sk-p")

	code, stdout, stderr := runCLI(t, "", "env", "p")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	// Registry phải được đọc từ HI_LLM_HOME, không phải ~ thật.
	reg, err := registry.Load(filepath.Join(e.home, "providers.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := reg.Providers["p"]; !ok {
		t.Fatal("provider p không nằm trong HI_LLM_HOME")
	}
	if !strings.Contains(stdout, "export OPENAI_BASE_URL='https://p.example/v1'") {
		t.Fatalf("stdout sai:\n%s", stdout)
	}
}
