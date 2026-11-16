package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"hi-tools/internal/hix"
	"hi-tools/internal/llm/provider"
	"hi-tools/internal/llm/registry"
)

// envRoles là biến môi trường override đường dẫn roles.conf.
const envRoles = "HI_LLM_ROLES"

// pickFn chọn một candidate cho role; thay được trong test.
var pickFn = fzfPick

// rolesPath tìm roles.conf: $HI_LLM_ROLES, hoặc cạnh binary flow trong PATH.
func rolesPath() (string, error) {
	if path := os.Getenv(envRoles); path != "" {
		return path, nil
	}
	path, err := exec.LookPath("flow")
	if err != nil {
		return "", errors.New("không tìm thấy roles.conf, đặt HI_LLM_ROLES")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Join(filepath.Dir(path), "roles.conf"), nil
}

// readRoles đọc roles.conf: mỗi role một dòng theo thứ tự xuất hiện, dòng cuối thắng.
func readRoles(data []byte) (order []string, values map[string]string) {
	values = map[string]string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		role := line[:idx]
		if role == "" {
			continue
		}
		if !seen[role] {
			seen[role] = true
			order = append(order, role)
		}
		values[role] = line[idx+1:]
	}
	return order, values
}

// setRole thay mọi dòng "^role=" bằng "role=value", giữ nguyên phần còn lại từng byte.
func setRole(data []byte, role, value string) ([]byte, bool) {
	lines := strings.Split(string(data), "\n")
	had := false
	prefix := role + "="
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + value
			had = true
		}
	}
	return []byte(strings.Join(lines, "\n")), had
}

// roleCandidates gộp model của provider (theo harness) và models khai cho subscription.
func roleCandidates(reg *registry.Registry, cacheDir string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}

	for name, p := range reg.Providers {
		c, err := provider.LoadCache(cacheDir, name)
		if err != nil {
			continue
		}
		for _, h := range p.HarnessList() {
			for _, model := range c.Models {
				add(h + ":" + name + "/" + model)
			}
		}
	}
	for _, sub := range reg.Subscriptions {
		for _, model := range sub.Models {
			add(sub.Harness + ":" + model)
		}
	}
	sort.Strings(out)
	return out
}

func runRole(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("role", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var force bool
	fs.BoolVar(&force, "force", false, "đặt value ngoài danh sách model (phải dạng harness:model)")

	pos, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) > 2 {
		fmt.Fprintf(stderr, "hi-llm role: tham số thừa %q\n", pos[2:])
		return 2
	}

	path, err := rolesPath()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm role: %v\n", err)
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm role: đọc %s: %v\n", path, err)
		return 1
	}
	order, values := readRoles(data)

	if len(pos) == 0 {
		tbl := hix.NewTable("ROLE", "VALUE")
		for _, role := range order {
			tbl.Add(role, values[role])
		}
		if err := tbl.Render(stdout); err != nil {
			fmt.Fprintf(stderr, "hi-llm role: %v\n", err)
			return 1
		}
		return 0
	}

	role := pos[0]
	current, exists := values[role]
	if !exists {
		fmt.Fprintf(stderr, "hi-llm role: role %q không có trong %s\n", role, path)
		return 2
	}

	var value string
	cands := []string{}
	if !force || len(pos) == 1 {
		home, err := registry.HomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm role: %v\n", err)
			return 1
		}
		reg, err := registry.Load(registry.Path(home))
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm role: %v\n", err)
			return 1
		}
		cacheDir, err := provider.CacheDir()
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm role: %v\n", err)
			return 1
		}
		cands = roleCandidates(reg, cacheDir)
	}

	if len(pos) == 2 {
		value = pos[1]
		if force && !validRoleValue(value) {
			fmt.Fprintf(stderr, "hi-llm role: --force cần value dạng harness:model, nhận %q\n", value)
			return 2
		}
	} else {
		if len(cands) == 0 {
			fmt.Fprintln(stderr, "hi-llm role: chưa có model nào: chạy hi-llm models <provider> hoặc khai models cho subscription")
			return 1
		}
		if !hix.IsTTY(stdin) {
			fmt.Fprintln(stderr, "hi-llm role: cần TTY để chọn, hoặc truyền <value> (vd hi-llm role <role> <harness:model>)")
			return 2
		}
		if _, err := exec.LookPath("fzf"); err != nil {
			fmt.Fprintln(stderr, "hi-llm role: không có fzf trong PATH; truyền <value> (vd hi-llm role <role> <harness:model>)")
			return 2
		}
		picked, err := pickFn(role+"> ", cands)
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm role: %v\n", err)
			return 1
		}
		value = picked
	}

	if !force && !containsString(cands, value) {
		fmt.Fprintf(stderr, "hi-llm role: %q không có trong danh sách model, dùng --force để đặt model chưa có trong cache\n", value)
		return 2
	}

	if value == current {
		fmt.Fprintf(stdout, "%s: không đổi\n", role)
		return 0
	}

	out, had := setRole(data, role, value)
	if !had {
		fmt.Fprintf(stderr, "hi-llm role: role %q không có trong %s\n", role, path)
		return 2
	}

	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := hix.WriteFileAtomic(path, out, perm); err != nil {
		fmt.Fprintf(stderr, "hi-llm role: ghi %s: %v\n", path, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %s → %s\n", role, current, value)
	return 0
}

// validRoleValue kiểm tra dạng harness:model (có ":" và hai phần không rỗng).
func validRoleValue(value string) bool {
	idx := strings.IndexByte(value, ':')
	return idx > 0 && idx < len(value)-1
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// fzfPick chạy fzf với candidates qua stdin và trả lựa chọn.
func fzfPick(prompt string, cands []string) (string, error) {
	cmd := exec.Command("fzf", "--prompt", prompt)
	cmd.Stdin = strings.NewReader(strings.Join(cands, "\n") + "\n")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code == 1 || code == 130 {
				return "", errors.New("đã huỷ")
			}
		}
		return "", fmt.Errorf("fzf: %w", err)
	}
	sel := strings.TrimSpace(string(out))
	if sel == "" {
		return "", errors.New("đã huỷ")
	}
	return sel, nil
}
