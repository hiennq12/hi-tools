package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hi-tools/internal/hix"
	"hi-tools/internal/llm/registry"
)

// writeRolesFile ghi roles.conf trong thư mục tạm và trỏ HI_LLM_ROLES vào đó.
func writeRolesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roles.conf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("ghi roles.conf: %v", err)
	}
	t.Setenv("HI_LLM_ROLES", path)
	return path
}

// fakePick thay pickFn trong một test và tự khôi phục. Cài thêm một `fzf` giả
// trong PATH để implementation có kiểm tra exec.LookPath vẫn đi qua nhánh pick.
func fakePick(t *testing.T, fn func(prompt string, cands []string) (string, error)) {
	t.Helper()
	old := pickFn
	pickFn = fn
	t.Cleanup(func() { pickFn = old })

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "fzf"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("tạo fzf giả: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// --- readRoles ---

func TestReadRolesOrderLastWinsAndSkipsComments(t *testing.T) {
	data := []byte(`# comment
implement=opencode:old
review=agy:gemini

implement=opencode:new
`)
	order, values := readRoles(data)

	if want := []string{"implement", "review"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, muốn %v (mỗi role một dòng, theo thứ tự xuất hiện, role trùng lấy dòng cuối)", order, want)
	}
	if values["implement"] != "opencode:new" {
		t.Fatalf("implement = %q, muốn dòng cuối opencode:new", values["implement"])
	}
	if values["review"] != "agy:gemini" {
		t.Fatalf("review = %q, muốn agy:gemini", values["review"])
	}
	if _, ok := values["# comment"]; ok {
		t.Fatalf("comment bị coi là role: %v", values)
	}
}

// --- setRole ---

func TestSetRoleReplacesAllAndKeepsRest(t *testing.T) {
	in := []byte("# giữ comment\nimplement=old\nreview=keep\n\nimplement=newer\n# cuối\n")
	out, had := setRole(in, "implement", "opencode:deeprelay/m1")

	if !had {
		t.Fatal("setRole báo không có dòng implement")
	}
	want := "# giữ comment\nimplement=opencode:deeprelay/m1\nreview=keep\n\nimplement=opencode:deeprelay/m1\n# cuối\n"
	if string(out) != want {
		t.Fatalf("setRole = %q, muốn %q", out, want)
	}
}

func TestSetRoleKeepsMissingTrailingNewline(t *testing.T) {
	out, had := setRole([]byte("implement=a\nreview=b"), "implement", "v")
	if !had {
		t.Fatal("setRole báo không có dòng implement")
	}
	if string(out) != "implement=v\nreview=b" {
		t.Fatalf("setRole = %q, muốn giữ nguyên thiếu newline cuối", out)
	}
}

func TestSetRoleRoleMissing(t *testing.T) {
	in := []byte("implement=a\n")
	out, had := setRole(in, "review", "v")
	if had {
		t.Fatal("setRole báo có dòng review dù không có")
	}
	if string(out) != string(in) {
		t.Fatalf("setRole thay đổi dữ liệu khi role không tồn tại: %q", out)
	}
}

func TestSetRoleDoesNotMatchPrefix(t *testing.T) {
	in := []byte("implement-x=a\nimplement=b\n")
	out, _ := setRole(in, "implement", "v")
	if string(out) != "implement-x=a\nimplement=v\n" {
		t.Fatalf("setRole khớp cả tiền tố sai: %q", out)
	}
}

// --- rolesPath ---

func TestRolesPathUsesEnv(t *testing.T) {
	path := writeRolesFile(t, "implement=a\n")

	got, err := rolesPath()
	if err != nil {
		t.Fatalf("rolesPath: %v", err)
	}
	if got != path {
		t.Fatalf("rolesPath = %q, muốn %q", got, path)
	}
}

func TestRolesPathNotFound(t *testing.T) {
	t.Setenv("HI_LLM_ROLES", "")
	t.Setenv("PATH", "")

	_, err := rolesPath()
	if err == nil {
		t.Fatal("rolesPath không có env và không có flow trong PATH trả nil, muốn lỗi")
	}
	if !strings.Contains(err.Error(), "HI_LLM_ROLES") {
		t.Fatalf("lỗi = %q, muốn nêu HI_LLM_ROLES", err)
	}
}

// --- roleCandidates ---

func TestRoleCandidatesFromCacheHarnessesAndSubscriptions(t *testing.T) {
	e := newCLIEnv(t)
	reg := &registry.Registry{
		Providers: map[string]registry.Provider{
			"deeprelay": {Kind: "openai", BaseURL: "https://d.example/v1", APIKey: "env:D"},
			"oc": {
				Kind: "openai", BaseURL: "https://o.example/v1", APIKey: "env:O",
				Harnesses: []string{"opencode", "commandcode"},
			},
			"chua-cache": {Kind: "openai", BaseURL: "https://c.example/v1", APIKey: "env:C"},
		},
		Subscriptions: map[string]registry.Subscription{
			"agy":    {Harness: "agy", Models: []string{"gemini-3.8-flash-high"}},
			"claude": {Harness: "claude"},
		},
	}
	e.seedCache(t, "deeprelay", []string{"m2", "m1"}, 0)
	e.seedCache(t, "oc", []string{"m3"}, 0)

	got := roleCandidates(reg, e.cacheDir)
	want := []string{
		"agy:gemini-3.8-flash-high",
		"commandcode:oc/m3",
		"opencode:deeprelay/m1",
		"opencode:deeprelay/m2",
		"opencode:oc/m3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roleCandidates = %v, muốn %v (sort + bỏ trùng)", got, want)
	}
}

func TestRoleCandidatesDedupes(t *testing.T) {
	e := newCLIEnv(t)
	reg := &registry.Registry{Providers: map[string]registry.Provider{
		"p": {
			Kind: "openai", BaseURL: "https://p.example/v1", APIKey: "env:P",
			Harnesses: []string{"opencode", "opencode"},
		},
	}}
	e.seedCache(t, "p", []string{"m"}, 0)

	got := roleCandidates(reg, e.cacheDir)
	want := []string{"opencode:p/m"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roleCandidates = %v, muốn %v (bỏ trùng harness)", got, want)
	}
}

func TestRoleCandidatesEmptyWhenNoCacheAndNoSubscriptionModels(t *testing.T) {
	e := newCLIEnv(t)
	reg := &registry.Registry{
		Providers: map[string]registry.Provider{
			"deeprelay": {Kind: "openai", BaseURL: "https://d.example/v1", APIKey: "env:D"},
		},
		Subscriptions: map[string]registry.Subscription{
			"claude": {Harness: "claude"},
		},
	}

	got := roleCandidates(reg, e.cacheDir)
	if len(got) != 0 {
		t.Fatalf("roleCandidates = %v, muốn rỗng khi chưa có cache và subscription không khai models", got)
	}
}

func TestRoleEmptyCandidatesExitError(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	// Không seed cache và subscription không khai models → không có candidate nào.
	path := writeRolesFile(t, "implement=opencode:old\n")

	// Cài pickFn giả: nếu implementation vẫn gọi picker thì đánh dấu lại.
	picked := false
	fakePick(t, func(prompt string, cands []string) (string, error) {
		picked = true
		return "opencode:deeprelay/m1", nil
	})

	// Lỗi "chưa có model nào" phải xảy ra trước picker. Implementation có thể kiểm
	// TTY trước khi tính candidates; khi đó thử lại qua /dev/tty thật.
	code, stdout, stderr := runCLI(t, "", "role", "implement")
	if code == 2 && !strings.Contains(stdout+stderr, "chưa có model nào") {
		code, stdout, stderr = runCLIWithTTY(t, "role", "implement")
	}
	if code == 0 {
		t.Fatalf("exit code = 0, muốn lỗi khi không có candidate nào (stdout=%q stderr=%q)", stdout, stderr)
	}
	if picked {
		t.Fatal("vẫn mở picker khi không có candidate nào")
	}
	if !strings.Contains(stdout+stderr, "chưa có model nào") {
		t.Fatalf("lỗi = %q, muốn nêu \"chưa có model nào\"", stdout+stderr)
	}
	if !strings.Contains(stdout+stderr, "hi-llm models") || !strings.Contains(stdout+stderr, "subscription") {
		t.Fatalf("lỗi = %q, muốn gợi ý hi-llm models / khai models cho subscription", stdout+stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=opencode:old\n" {
		t.Fatalf("roles.conf bị sửa khi không có candidate: %q", data)
	}
}

// --- run role ---

func TestRoleTable(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, registryWithDeeprelay)
	writeRolesFile(t, "# comment\nimplement=opencode:old\nreview=agy:gemini\nimplement=opencode:new\n")

	code, stdout, stderr := runCLI(t, "", "role")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "ROLE") || !strings.Contains(stdout, "VALUE") {
		t.Fatalf("bảng thiếu header ROLE/VALUE:\n%s", stdout)
	}
	if !strings.Contains(stdout, "implement") || !strings.Contains(stdout, "opencode:new") {
		t.Fatalf("bảng thiếu implement với dòng cuối:\n%s", stdout)
	}
	if strings.Contains(stdout, "opencode:old") {
		t.Fatalf("bảng dùng dòng đầu thay vì dòng cuối:\n%s", stdout)
	}
	if !strings.Contains(stdout, "review") || !strings.Contains(stdout, "agy:gemini") {
		t.Fatalf("bảng thiếu review:\n%s", stdout)
	}
	if strings.Contains(stdout, "# comment") {
		t.Fatalf("bảng hiện comment:\n%s", stdout)
	}
}

func TestRoleSetValueInCandidates(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1", "m2"}, 0)
	path := writeRolesFile(t, "# giữ comment\nimplement=opencode:old\nreview=agy:x\n")

	code, stdout, stderr := runCLI(t, "", "role", "implement", "opencode:deeprelay/m2")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "implement") || !strings.Contains(stdout, "→") || !strings.Contains(stdout, "opencode:deeprelay/m2") {
		t.Fatalf("stdout = %q, muốn in <role>: <cũ> → <mới>", stdout)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	want := "# giữ comment\nimplement=opencode:deeprelay/m2\nreview=agy:x\n"
	if string(got) != want {
		t.Fatalf("roles.conf = %q, muốn %q", got, want)
	}
}

func TestRoleSetValueKeepsFilePerm(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=old\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	code, stdout, stderr := runCLI(t, "", "role", "implement", "opencode:deeprelay/m1")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat roles.conf: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("quyền roles.conf = %o, muốn giữ 640", perm)
	}
}

func TestRoleUnchangedValueDoesNotWrite(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=opencode:deeprelay/m1\n")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat trước: %v", err)
	}

	code, stdout, stderr := runCLI(t, "", "role", "implement", "opencode:deeprelay/m1")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "không đổi") {
		t.Fatalf("stdout = %q, muốn nêu không đổi", stdout)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat sau: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("giá trị không đổi nhưng file bị ghi lại")
	}
}

func TestRoleMissingRoleExit2(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=a\n")

	// Value hợp lệ để lỗi (nếu có) chắc chắn là do role không tồn tại.
	code, stdout, stderr := runCLI(t, "", "role", "review", "opencode:deeprelay/m1")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 với role không có (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, path) {
		t.Fatalf("lỗi không nêu file roles.conf %s: stdout=%q stderr=%q", path, stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=a\n" {
		t.Fatalf("roles.conf bị sửa: %q", data)
	}
}

func TestRoleValueOutsideCandidatesExit2(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=opencode:deeprelay/m1\n")

	code, stdout, stderr := runCLI(t, "", "role", "implement", "opencode:khong-co/m9")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 với value ngoài candidates (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "--force") {
		t.Fatalf("lỗi không gợi ý --force: stdout=%q stderr=%q", stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=opencode:deeprelay/m1\n" {
		t.Fatalf("roles.conf bị sửa dù value ngoài candidates: %q", data)
	}
}

func TestRoleForceWritesValueOutsideCandidates(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=opencode:old\n")

	code, stdout, stderr := runCLI(t, "", "role", "implement", "commandcode:zai-org/glm-5.3", "--force")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 với --force (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=commandcode:zai-org/glm-5.3\n" {
		t.Fatalf("roles.conf = %q, muốn ghi value với --force", data)
	}
}

func TestRoleForceRequiresHarnessModelForm(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=opencode:old\n")

	for _, bad := range []string{"khong-co-dau-hai-cham", ":chi-model", "harness:"} {
		code, stdout, stderr := runCLI(t, "", "role", "implement", bad, "--force")
		if code != 2 {
			t.Fatalf("--force value %q exit %d, muốn 2 (stdout=%q stderr=%q)", bad, code, stdout, stderr)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=opencode:old\n" {
		t.Fatalf("roles.conf bị sửa với value sai dạng: %q", data)
	}
}

func TestRoleNoValueNoTTYExit2(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=opencode:old\n")
	// Không có fzf trong PATH để chắc chắn không mở picker thật.
	t.Setenv("PATH", "")

	code, stdout, stderr := runCLI(t, "", "role", "implement")
	if code != 2 {
		t.Fatalf("exit code = %d, muốn 2 khi không TTY/không fzf (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "<value>") && !strings.Contains(stdout+stderr, "value") {
		t.Fatalf("lỗi không nêu cách truyền <value>: stdout=%q stderr=%q", stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=opencode:old\n" {
		t.Fatalf("roles.conf bị sửa khi không chọn: %q", data)
	}
}

// runCLIWithTTY gọi run() với stdin là terminal thật (nhánh fzf/pickFn).
func runCLIWithTTY(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skip("không có /dev/tty để kiểm nhánh picker")
	}
	defer f.Close()
	if !hix.IsTTY(f) {
		t.Skip("stdin không phải terminal để kiểm nhánh picker")
	}
	var out, errOut bytes.Buffer
	code = run(args, f, &out, &errOut)
	return code, out.String(), errOut.String()
}

// runRolePick chạy `role <role>` qua nhánh picker. Implementation có thể chỉ gọi
// pickFn khi stdin là TTY; thử không TTY trước, nếu implementation từ chối (exit 2)
// thì chạy lại bằng /dev/tty thật. Không có TTY (CI) → skip.
func runRolePick(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	code, stdout, stderr = runCLI(t, "", args...)
	if code == 2 {
		return runCLIWithTTY(t, args...)
	}
	return code, stdout, stderr
}

func TestRolePickFnUsesChoice(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1", "m2"}, 0)
	path := writeRolesFile(t, "implement=opencode:old\n")

	var (
		gotPrompt string
		gotCands  []string
	)
	fakePick(t, func(prompt string, cands []string) (string, error) {
		gotPrompt = prompt
		gotCands = append([]string(nil), cands...)
		return "opencode:deeprelay/m2", nil
	})

	code, stdout, stderr := runRolePick(t, "role", "implement")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(gotPrompt, "implement") {
		t.Fatalf("prompt = %q, muốn chứa tên role", gotPrompt)
	}
	if len(gotCands) == 0 {
		t.Fatal("pickFn không nhận candidates")
	}
	want := "opencode:deeprelay/m2"
	found := false
	for _, c := range gotCands {
		if c == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("candidates %v thiếu %q", gotCands, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=opencode:deeprelay/m2\n" {
		t.Fatalf("roles.conf = %q, muốn lựa chọn từ pickFn", data)
	}
}

func TestRolePickFnCancelDoesNotWrite(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `providers:
  deeprelay:
    kind: openai
    base_url: https://d.example/v1
    api_key: env:D
`)
	e.seedCache(t, "deeprelay", []string{"m1"}, 0)
	path := writeRolesFile(t, "implement=opencode:old\n")

	fakePick(t, func(prompt string, cands []string) (string, error) {
		return "", errors.New("đã huỷ")
	})

	code, stdout, stderr := runRolePick(t, "role", "implement")
	if code != 1 {
		t.Fatalf("exit code = %d, muốn 1 khi huỷ (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "huỷ") {
		t.Fatalf("output không nêu đã huỷ: stdout=%q stderr=%q", stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "implement=opencode:old\n" {
		t.Fatalf("roles.conf bị sửa khi huỷ: %q", data)
	}
}

func TestRoleCandidatesFromSubscriptionOnly(t *testing.T) {
	e := newCLIEnv(t)
	e.writeRegistry(t, `subscriptions:
  agy:
    harness: agy
    models: [gemini-3.8-flash-high, gemini-3.8-flash-medium]
`)
	path := writeRolesFile(t, "review=old\n")

	code, stdout, stderr := runCLI(t, "", "role", "review", "agy:gemini-3.8-flash-medium")
	if code != 0 {
		t.Fatalf("exit code = %d, muốn 0 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc roles.conf: %v", err)
	}
	if string(data) != "review=agy:gemini-3.8-flash-medium\n" {
		t.Fatalf("roles.conf = %q, muốn dùng model của subscription", data)
	}
}
