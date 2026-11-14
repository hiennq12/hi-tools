package registry_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hi-tools/internal/llm/registry"
)

const validYAML = `providers:
  deeprelay:
    kind: openai
    base_url: https://api.deeprelay.example/v1
    api_key: enc:deeprelay
subscriptions:
  claude:
    harness: claude
`

func TestParseValid(t *testing.T) {
	reg, err := registry.Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, ok := reg.Providers["deeprelay"]
	if !ok {
		t.Fatalf("thiếu provider deeprelay: %+v", reg.Providers)
	}
	if p.Kind != "openai" || p.BaseURL != "https://api.deeprelay.example/v1" || p.APIKey != "enc:deeprelay" {
		t.Fatalf("provider parse sai: %+v", p)
	}
	sub, ok := reg.Subscriptions["claude"]
	if !ok {
		t.Fatalf("thiếu subscription claude: %+v", reg.Subscriptions)
	}
	if sub.Harness != "claude" {
		t.Fatalf("harness = %q, muốn claude", sub.Harness)
	}
}

func TestParseRejectsBadNames(t *testing.T) {
	cases := []string{
		"Bad/Name",
		"Upper",
		"..",
		"-leading",
		"has space",
		"a:b",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			yaml := `providers:
  "` + name + `":
    kind: openai
    base_url: https://api.example.com/v1
    api_key: enc:` + name + `
`
			if _, err := registry.Parse([]byte(yaml)); err == nil {
				t.Fatalf("Parse chấp nhận tên %q", name)
			}
		})
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	reg := &registry.Registry{Providers: map[string]registry.Provider{
		"p": {Kind: "anthropic", BaseURL: "https://api.example.com/v1", APIKey: "env:P_KEY"},
	}}
	if err := reg.Validate(); err == nil {
		t.Fatal("Validate chấp nhận kind anthropic")
	}

	reg.Providers["p"] = registry.Provider{Kind: "", BaseURL: "https://api.example.com/v1", APIKey: "env:P_KEY"}
	if err := reg.Validate(); err == nil {
		t.Fatal("Validate chấp nhận kind rỗng")
	}
}

func TestValidateRejectsBadPointer(t *testing.T) {
	for _, pointer := range []string{"", "deeprelay", "enc:", "bogus:x", "ENV:VAR"} {
		reg := &registry.Registry{Providers: map[string]registry.Provider{
			"p": {Kind: "openai", BaseURL: "https://api.example.com/v1", APIKey: pointer},
		}}
		if err := reg.Validate(); err == nil {
			t.Fatalf("Validate chấp nhận api_key %q", pointer)
		}
	}
}

func TestValidateRejectsDuplicateNames(t *testing.T) {
	reg := &registry.Registry{
		Providers: map[string]registry.Provider{
			"same": {Kind: "openai", BaseURL: "https://api.example.com/v1", APIKey: "env:KEY"},
		},
		Subscriptions: map[string]registry.Subscription{
			"same": {Harness: "claude"},
		},
	}
	if err := reg.Validate(); err == nil {
		t.Fatal("Validate chấp nhận tên trùng giữa providers và subscriptions")
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "khong-co", "providers.yaml")
	reg, err := registry.Load(path)
	if err != nil {
		t.Fatalf("Load file chưa có lỗi: %v", err)
	}
	if reg == nil {
		t.Fatal("Load file chưa có trả nil registry")
	}
	if len(reg.Providers) != 0 || len(reg.Subscriptions) != 0 {
		t.Fatalf("registry rỗng mong đợi, nhận %+v", reg)
	}
}

func TestLoadInvalidYAMLIncludesPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(path, []byte("providers: [khong-dong"), 0o600); err != nil {
		t.Fatalf("ghi yaml hỏng: %v", err)
	}

	_, err := registry.Load(path)
	if err == nil {
		t.Fatal("Load yaml hỏng trả nil error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("lỗi Load = %q, muốn kèm đường dẫn %s", err, path)
	}
}

func TestLoadValidateFailIncludesPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	bad := `providers:
  p:
    kind: gemini
    base_url: https://api.example.com/v1
    api_key: env:P_KEY
`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatalf("ghi yaml: %v", err)
	}

	_, err := registry.Load(path)
	if err == nil {
		t.Fatal("Load registry validate fail trả nil error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("lỗi Load = %q, muốn kèm đường dẫn %s", err, path)
	}
}

func TestSaveRoundTripAndStableOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "providers.yaml")
	reg := &registry.Registry{
		Providers: map[string]registry.Provider{
			"zeta":  {Kind: "openai", BaseURL: "https://z.example/v1", APIKey: "env:Z_KEY"},
			"alpha": {Kind: "openai", BaseURL: "https://a.example/v1", APIKey: "enc:alpha"},
		},
		Subscriptions: map[string]registry.Subscription{
			"claude": {Harness: "claude"},
		},
	}
	if err := reg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc file vừa ghi: %v", err)
	}
	if i, j := bytes.Index(data, []byte("alpha")), bytes.Index(data, []byte("zeta")); i < 0 || j < 0 || i > j {
		t.Fatalf("yaml không sort theo tên:\n%s", data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("quyền providers.yaml = %o, muốn 600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat thư mục cha: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("quyền thư mục config = %o, muốn 700", perm)
	}

	loaded, err := registry.Load(path)
	if err != nil {
		t.Fatalf("Load sau Save: %v", err)
	}
	if loaded.Providers["alpha"].BaseURL != "https://a.example/v1" {
		t.Fatalf("round-trip mất dữ liệu: %+v", loaded.Providers)
	}
	if !loaded.IsSubscription("claude") {
		t.Fatalf("round-trip mất subscription: %+v", loaded.Subscriptions)
	}

	// Ghi lần hai với cùng dữ liệu phải ra đúng cùng byte (diff ổn định).
	if err := reg.Save(path); err != nil {
		t.Fatalf("Save lần hai: %v", err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc lại: %v", err)
	}
	if !bytes.Equal(data, again) {
		t.Fatalf("Save không ổn định:\n%s\n---\n%s", data, again)
	}
}

func TestSaveDoesNotContainSecretForEncPointer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.yaml")
	reg := &registry.Registry{Providers: map[string]registry.Provider{
		"p": {Kind: "openai", BaseURL: "https://api.example.com/v1", APIKey: "enc:p"},
	}}
	if err := reg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc: %v", err)
	}
	if !strings.Contains(string(data), "enc:p") {
		t.Fatalf("yaml thiếu con trỏ enc:p:\n%s", data)
	}
}

func TestValidName(t *testing.T) {
	valid := []string{"a", "9lives", "deep-relay_1", "a.b.c", "x_y"}
	for _, name := range valid {
		if err := registry.ValidName(name); err != nil {
			t.Fatalf("ValidName(%q) lỗi: %v", name, err)
		}
	}

	invalid := []string{"", "Bad", "a/b", "..", ".", "-a", "_a", "a b", "a:b", "a\\b"}
	for _, name := range invalid {
		if err := registry.ValidName(name); err == nil {
			t.Fatalf("ValidName(%q) = nil, muốn lỗi", name)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://api.example.com", "https://api.example.com/v1"},
		{"https://api.example.com/", "https://api.example.com/v1"},
		{"https://api.example.com/v1", "https://api.example.com/v1"},
		{"https://api.example.com/v1/", "https://api.example.com/v1"},
		{"https://api.example.com/openai", "https://api.example.com/openai"},
		{"http://localhost:8080", "http://localhost:8080/v1"},
		{"http://localhost:8080/", "http://localhost:8080/v1"},
	}
	for _, tc := range cases {
		got, err := registry.NormalizeBaseURL(tc.in)
		if err != nil {
			t.Fatalf("NormalizeBaseURL(%q) lỗi: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeBaseURL(%q) = %q, muốn %q", tc.in, got, tc.want)
		}
	}

	invalid := []string{"", "khong-phai-url", "ftp://api.example.com", "https://", "mailto:a@b.c"}
	for _, in := range invalid {
		if _, err := registry.NormalizeBaseURL(in); err == nil {
			t.Fatalf("NormalizeBaseURL(%q) = nil, muốn lỗi", in)
		}
	}
}

func TestNamesAndIsSubscription(t *testing.T) {
	reg, err := registry.Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !reg.IsSubscription("claude") {
		t.Fatal("IsSubscription(claude) = false")
	}
	if reg.IsSubscription("deeprelay") {
		t.Fatal("IsSubscription(deeprelay) = true")
	}

	names := reg.Names()
	want := []string{"claude", "deeprelay"}
	if len(names) != len(want) {
		t.Fatalf("Names() = %v, muốn %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Names() = %v, muốn %v (đã sort)", names, want)
		}
	}
}

func TestHomeDirAndPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv(registry.EnvHome, home)

	got, err := registry.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}
	if got != home {
		t.Fatalf("HomeDir = %q, muốn %q", got, home)
	}
	if p := registry.Path(home); p != filepath.Join(home, "providers.yaml") {
		t.Fatalf("Path = %q, muốn %q", p, filepath.Join(home, "providers.yaml"))
	}
}
