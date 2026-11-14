package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cliEnv dựng môi trường cách ly cho mỗi test: HI_LLM_HOME và HI_LLM_CACHE_DIR
// trỏ vào t.TempDir(), master password đặt sẵn để không bao giờ hỏi terminal.
type cliEnv struct {
	t        *testing.T
	home     string
	cacheDir string
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	base := t.TempDir()
	e := &cliEnv{
		t:        t,
		home:     filepath.Join(base, "home"),
		cacheDir: filepath.Join(base, "cache"),
	}
	t.Setenv("HI_LLM_HOME", e.home)
	t.Setenv("HI_LLM_CACHE_DIR", e.cacheDir)
	t.Setenv("HI_LLM_MASTER_PASSWORD", "test-master-password")
	return e
}

func (e *cliEnv) providersPath() string { return filepath.Join(e.home, "providers.yaml") }
func (e *cliEnv) keysPath() string      { return filepath.Join(e.home, "keys.enc") }

// writeRegistry ghi thẳng providers.yaml để dựng trạng thái ban đầu.
func (e *cliEnv) writeRegistry(t *testing.T, yaml string) {
	t.Helper()
	if err := os.MkdirAll(e.home, 0o700); err != nil {
		t.Fatalf("MkdirAll home: %v", err)
	}
	if err := os.WriteFile(e.providersPath(), []byte(yaml), 0o600); err != nil {
		t.Fatalf("ghi providers.yaml: %v", err)
	}
}

func (e *cliEnv) readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc %s: %v", path, err)
	}
	return string(b)
}

func (e *cliEnv) exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runCLI gọi run() với stdin/stdout/stderr giả, trả exit code + output.
func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

const registryWithDeeprelay = `providers:
  deeprelay:
    kind: openai
    base_url: https://api.deeprelay.example/v1
    api_key: enc:deeprelay
subscriptions:
  claude:
    harness: claude
`
