package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"hi-tools/internal/hix"
)

const (
	// EnvCacheDir là biến môi trường override thư mục cache.
	EnvCacheDir = "HI_LLM_CACHE_DIR"
	// DefaultTTL là hạn mặc định của cache model.
	DefaultTTL = time.Hour
)

// CacheDir trả về thư mục cache: $HI_LLM_CACHE_DIR hoặc ~/.cache/hi-llm.
func CacheDir() (string, error) {
	if dir := os.Getenv(EnvCacheDir); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "hi-llm"), nil
}

// Cache là nội dung file cache của một provider.
type Cache struct {
	FetchedAt time.Time `json:"fetched_at"`
	Models    []string  `json:"models"`
}

// Fresh cho biết cache còn hạn tại now với ttl.
func (c *Cache) Fresh(now time.Time, ttl time.Duration) bool {
	return c.Age(now) <= ttl
}

// Age là tuổi cache tại now.
func (c *Cache) Age(now time.Time) time.Duration {
	return now.Sub(c.FetchedAt)
}

func cachePath(dir, name string) string {
	return filepath.Join(dir, name+".json")
}

// LoadCache đọc cache; file chưa có trả lỗi bọc fs.ErrNotExist.
func LoadCache(dir, name string) (*Cache, error) {
	data, err := os.ReadFile(cachePath(dir, name))
	if err != nil {
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveCache ghi cache atomic; tự tạo thư mục.
func SaveCache(dir, name string, c *Cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return hix.WriteFileAtomic(cachePath(dir, name), data, 0o600)
}

// CachedModels trả models từ cache nếu còn hạn (và không refresh),
// ngược lại gọi key() rồi fetch và ghi cache.
func (c *Client) CachedModels(ctx context.Context, dir, name, baseURL string, key func() (string, error), ttl time.Duration, refresh bool) ([]string, error) {
	if !refresh {
		if cached, err := LoadCache(dir, name); err == nil && cached.Fresh(time.Now(), ttl) {
			return cached.Models, nil
		}
	}

	secret, err := key()
	if err != nil {
		return nil, err
	}
	models, err := c.ListModels(ctx, baseURL, secret)
	if err != nil {
		return nil, err
	}
	if err := SaveCache(dir, name, &Cache{FetchedAt: time.Now(), Models: models}); err != nil {
		return nil, err
	}
	return models, nil
}
