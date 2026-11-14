// Package registry quản lý file providers.yaml: load/save/validate registry provider.
package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"hi-tools/internal/hix"
)

// EnvHome là biến môi trường override thư mục config.
const EnvHome = "HI_LLM_HOME"

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Provider là một API relay OpenAI-compatible.
type Provider struct {
	Kind    string `yaml:"kind"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

// Subscription là một CLI đã login sẵn (claude, agy...).
type Subscription struct {
	Harness string `yaml:"harness"`
}

// Registry là nội dung providers.yaml.
type Registry struct {
	Providers     map[string]Provider     `yaml:"providers,omitempty"`
	Subscriptions map[string]Subscription `yaml:"subscriptions,omitempty"`
}

// HomeDir trả về thư mục config: $HI_LLM_HOME hoặc ~/.config/hi-llm.
func HomeDir() (string, error) {
	if dir := os.Getenv(EnvHome); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("không xác định được thư mục home: %w", err)
	}
	return filepath.Join(home, ".config", "hi-llm"), nil
}

// Path trả về đường dẫn providers.yaml trong home.
func Path(home string) string {
	return filepath.Join(home, "providers.yaml")
}

// Load đọc registry từ path. File chưa tồn tại → registry rỗng.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Registry{}, nil
		}
		return nil, fmt.Errorf("đọc %s: %w", path, err)
	}
	reg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return reg, nil
}

// Parse unmarshal YAML rồi validate.
func Parse(data []byte) (*Registry, error) {
	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("yaml không hợp lệ: %w", err)
	}
	if err := reg.Validate(); err != nil {
		return nil, err
	}
	return &reg, nil
}

// Validate kiểm tra tên, kind, con trỏ api_key và trùng tên giữa hai map.
func (r *Registry) Validate() error {
	for name, p := range r.Providers {
		if err := ValidName(name); err != nil {
			return fmt.Errorf("provider %q: %w", name, err)
		}
		if p.Kind != "openai" {
			return fmt.Errorf("provider %q: kind %q không hỗ trợ (chỉ nhận openai)", name, p.Kind)
		}
		if err := ValidPointer(p.APIKey); err != nil {
			return fmt.Errorf("provider %q: %w", name, err)
		}
	}
	for name := range r.Subscriptions {
		if err := ValidName(name); err != nil {
			return fmt.Errorf("subscription %q: %w", name, err)
		}
		if _, dup := r.Providers[name]; dup {
			return fmt.Errorf("tên %q trùng giữa providers và subscriptions", name)
		}
	}
	return nil
}

// Save ghi registry ra path (atomic, 0600), key trong yaml được sort ổn định.
func (r *Registry) Save(path string) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	return hix.WriteFileAtomic(path, data, 0o600)
}

// ValidName kiểm tra tên provider/subscription.
func ValidName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("tên %q không hợp lệ (chỉ [a-z0-9._-], bắt đầu bằng chữ/số)", name)
	}
	return nil
}

// ValidPointer kiểm tra con trỏ api_key: enc:|env:|plain:|keychain: + phần sau không rỗng.
func ValidPointer(p string) error {
	kind, ref, err := splitPointer(p)
	if err != nil {
		return err
	}
	_ = kind
	if ref == "" {
		return fmt.Errorf("con trỏ api_key %q rỗng sau tiền tố", p)
	}
	return nil
}

func splitPointer(p string) (kind, ref string, err error) {
	idx := strings.IndexByte(p, ':')
	if idx <= 0 {
		return "", "", fmt.Errorf("con trỏ api_key %q không hợp lệ (cần enc:|env:|plain:|keychain:)", p)
	}
	kind, ref = p[:idx], p[idx+1:]
	switch kind {
	case "enc", "env", "plain", "keychain":
		return kind, ref, nil
	default:
		return "", "", fmt.Errorf("con trỏ api_key %q có tiền tố lạ %q", p, kind)
	}
}

// NormalizeBaseURL chuẩn hoá base URL: chỉ nhận http/https có host,
// path rỗng hoặc "/" thì nối /v1, bỏ "/" cuối.
func NormalizeBaseURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("base_url rỗng")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("base_url %q không hợp lệ: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base_url %q phải dùng http hoặc https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base_url %q thiếu host", raw)
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/v1"
	}
	return u.Scheme + "://" + u.Host + path, nil
}

// IsSubscription trả về true nếu name là subscription.
func (r *Registry) IsSubscription(name string) bool {
	_, ok := r.Subscriptions[name]
	return ok
}

// Names trả về tên provider + subscription, đã sort.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.Providers)+len(r.Subscriptions))
	for name := range r.Providers {
		names = append(names, name)
	}
	for name := range r.Subscriptions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
