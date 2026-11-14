package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hi-tools/internal/llm/keystore"
)

func newTestEncStore(t *testing.T) *keystore.EncStore {
	t.Helper()
	t.Setenv(keystore.EnvMasterPassword, "test-master-password")
	return keystore.NewEncStore(filepath.Join(t.TempDir(), "keys.enc"), keystore.EnvPassword())
}

func TestKeyStatusEnv(t *testing.T) {
	enc := newTestEncStore(t)

	t.Setenv("HI_LLM_LS_VAR", "co-gia-tri")
	got := keyStatus("env:HI_LLM_LS_VAR", enc)
	if !strings.Contains(got, "env:HI_LLM_LS_VAR") || !strings.Contains(got, "✅") {
		t.Fatalf("keyStatus = %q, muốn env:HI_LLM_LS_VAR ✅", got)
	}

	t.Setenv("HI_LLM_LS_EMPTY", "")
	got = keyStatus("env:HI_LLM_LS_EMPTY", enc)
	if !strings.Contains(got, "env:HI_LLM_LS_EMPTY") || !strings.Contains(got, "❌") {
		t.Fatalf("keyStatus = %q, muốn env:HI_LLM_LS_EMPTY ❌", got)
	}
}

func TestKeyStatusEnc(t *testing.T) {
	enc := newTestEncStore(t)
	if err := enc.Set("p", "sk-x"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got := keyStatus("enc:p", enc); !strings.Contains(got, "enc") || !strings.Contains(got, "✅") {
		t.Fatalf("keyStatus = %q, muốn enc ✅", got)
	}
	if got := keyStatus("enc:missing", enc); !strings.Contains(got, "❌") {
		t.Fatalf("keyStatus key thiếu = %q, muốn enc ❌", got)
	}

	empty := keystore.NewEncStore(filepath.Join(t.TempDir(), "keys.enc"), keystore.EnvPassword())
	if got := keyStatus("enc:p", empty); !strings.Contains(got, "❌") {
		t.Fatalf("keyStatus khi chưa có keys.enc = %q, muốn enc ❌", got)
	}
}

func TestKeyStatusUnsupported(t *testing.T) {
	enc := newTestEncStore(t)
	for _, ptr := range []string{"plain:foo", "keychain:foo"} {
		got := keyStatus(ptr, enc)
		if !strings.Contains(got, "chưa hỗ trợ") {
			t.Fatalf("keyStatus(%q) = %q, muốn nêu chưa hỗ trợ", ptr, got)
		}
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		d       time.Duration
		wantSub string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
	}
	for _, tc := range cases {
		got := formatAge(tc.d)
		if got == "" {
			t.Fatalf("formatAge(%v) rỗng", tc.d)
		}
		if !strings.Contains(got, tc.wantSub) {
			t.Fatalf("formatAge(%v) = %q, muốn chứa %q", tc.d, got, tc.wantSub)
		}
	}
	if got := formatAge(5 * time.Minute); !strings.Contains(got, "5m") {
		t.Fatalf("formatAge(5m) = %q, muốn dạng ngắn chứa 5m", got)
	}
}

func TestLsShowsEncLockedWithoutPassword(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	e.seedCache(t, "deeprelay", []string{"a"}, time.Minute)

	// Không có HI_LLM_MASTER_PASSWORD: ls không được hỏi password, phải in 🔒.
	t.Setenv(keystore.EnvMasterPassword, "")
	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "enc 🔒") {
		t.Fatalf("stdout không có enc 🔒:\n%s", stdout)
	}
	if strings.Contains(stdout, "test-master-password") {
		t.Fatalf("stdout lộ master password:\n%s", stdout)
	}
}

func TestLsEncOKWithPassword(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	store := keystore.NewEncStore(e.keysPath(), keystore.EnvPassword())
	if err := store.Set("deeprelay", "sk-REAL-SECRET"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "enc ✅") {
		t.Fatalf("stdout không có enc ✅:\n%s", stdout)
	}
	if strings.Contains(stdout, "sk-REAL-SECRET") {
		t.Fatalf("ls làm lộ secret:\n%s", stdout)
	}
}

func TestLsEncMissingKeysFile(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "enc ❌") {
		t.Fatalf("stdout (chưa có keys.enc) không có enc ❌:\n%s", stdout)
	}
	if e.exists(e.keysPath()) {
		t.Fatal("ls đã tạo keys.enc")
	}
}

func TestLsEncWrongPassword(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	wrong := keystore.NewEncStore(e.keysPath(), func(confirm bool) (string, error) {
		return "another-password", nil
	})
	if err := wrong.Set("deeprelay", "sk-x"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "enc ❌") {
		t.Fatalf("stdout (sai password) không có enc ❌:\n%s", stdout)
	}
}

func TestLsShowsEnvColumn(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  p:
    kind: openai
    base_url: https://p.example/v1
    api_key: env:HI_LLM_LS_COL
`)
	t.Setenv("HI_LLM_LS_COL", "secret-value")

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "env:HI_LLM_LS_COL ✅") {
		t.Fatalf("stdout không có env:HI_LLM_LS_COL ✅:\n%s", stdout)
	}
	if strings.Contains(stdout, "secret-value") {
		t.Fatalf("ls làm lộ giá trị env:\n%s", stdout)
	}
}

func TestLsShowsModelsAge(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	e.seedCache(t, "deeprelay", []string{"a", "b"}, 5*time.Minute)

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "2 (") {
		t.Fatalf("stdout không có số model kèm tuổi cache (vd \"2 (5m)\"):\n%s", stdout)
	}
	if !strings.Contains(stdout, "5m") {
		t.Fatalf("stdout thiếu tuổi cache ~5m cạnh số model:\n%s", stdout)
	}
}

func TestLsNoCacheShowsDash(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if !strings.HasPrefix(line, "deeprelay") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[len(fields)-1] != "-" {
			t.Fatalf("dòng deeprelay chưa có cache phải kết thúc bằng -: %q", line)
		}
		return
	}
	t.Fatalf("không thấy dòng deeprelay trong output:\n%s", stdout)
}

func TestLsListsSubscriptions(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "claude") {
		t.Fatalf("stdout thiếu subscription claude:\n%s", stdout)
	}
	if !strings.Contains(stdout, "sub") {
		t.Fatalf("stdout thiếu KIND=sub:\n%s", stdout)
	}
}

func TestLsSortedByName(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  zeta:
    kind: openai
    base_url: https://z.example/v1
    api_key: env:Z
  alpha:
    kind: openai
    base_url: https://a.example/v1
    api_key: env:A
`)

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	ia := strings.Index(stdout, "alpha")
	iz := strings.Index(stdout, "zeta")
	if ia < 0 || iz < 0 || ia > iz {
		t.Fatalf("ls không sort theo tên (alpha trước zeta):\n%s", stdout)
	}
}

func TestLsHeader(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr)
	}
	for _, col := range []string{"NAME", "KIND", "BASE_URL", "KEY", "MODELS"} {
		if !strings.Contains(stdout, col) {
			t.Fatalf("bảng thiếu cột %s:\n%s", col, stdout)
		}
	}
}

func TestLsEmptyRegistry(t *testing.T) {
	newCLIEnv(t) // home chưa tồn tại

	code, stdout, stderr := runCLI(t, "", "ls")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 với registry rỗng (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout+stderr, "hi-llm add") {
		t.Fatalf("registry rỗng không in hướng dẫn hi-llm add: stdout=%q stderr=%q", stdout, stderr)
	}
}
