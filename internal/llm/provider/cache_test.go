package provider_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hi-tools/internal/llm/provider"
)

func TestCacheFreshAndAge(t *testing.T) {
	now := time.Now()
	c := &provider.Cache{FetchedAt: now.Add(-5 * time.Minute), Models: []string{"m"}}

	if !c.Fresh(now, time.Hour) {
		t.Fatal("Fresh(now, 1h) = false với cache 5 phút")
	}
	if c.Fresh(now, time.Minute) {
		t.Fatal("Fresh(now, 1m) = true với cache 5 phút")
	}
	if age := c.Age(now); age < 4*time.Minute || age > 6*time.Minute {
		t.Fatalf("Age = %v, muốn ~5m", age)
	}
}

func TestLoadCacheMissingFile(t *testing.T) {
	_, err := provider.LoadCache(t.TempDir(), "khong-co")
	if err == nil {
		t.Fatal("LoadCache file chưa có trả nil error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lỗi = %v, muốn bọc fs.ErrNotExist", err)
	}
}

func TestSaveAndLoadCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chua", "co", "cache")
	c := &provider.Cache{FetchedAt: time.Now().Truncate(time.Second), Models: []string{"a", "b"}}

	if err := provider.SaveCache(dir, "deeprelay", c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deeprelay.json")); err != nil {
		t.Fatalf("cache deeprelay.json không được tạo: %v", err)
	}

	got, err := provider.LoadCache(dir, "deeprelay")
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if len(got.Models) != 2 || got.Models[0] != "a" || got.Models[1] != "b" {
		t.Fatalf("Models = %v, muốn [a b]", got.Models)
	}
	if !got.FetchedAt.Equal(c.FetchedAt) {
		t.Fatalf("FetchedAt = %v, muốn %v", got.FetchedAt, c.FetchedAt)
	}
}

func TestSaveCacheAtomicHasNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	c := &provider.Cache{FetchedAt: time.Now(), Models: []string{"m"}}
	if err := provider.SaveCache(dir, "p", c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "p.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("cache dir còn %v, muốn đúng p.json", names)
	}
}

func TestCacheDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(provider.EnvCacheDir, dir)

	got, err := provider.CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if got != dir {
		t.Fatalf("CacheDir = %q, muốn %q", got, dir)
	}
}

// CachedModels: cache còn hạn thì không gọi mạng và không gọi key().
func TestCachedModelsFreshCacheSkipsNetwork(t *testing.T) {
	dir := t.TempDir()
	if err := provider.SaveCache(dir, "p", &provider.Cache{
		FetchedAt: time.Now().Add(-5 * time.Minute),
		Models:    []string{"cached"},
	}); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_, _ = w.Write([]byte(`{"data":[{"id":"net"}]}`))
	}))
	defer srv.Close()

	keyCalls := 0
	key := func() (string, error) {
		keyCalls++
		return "sk-x", nil
	}

	client := provider.NewClient()
	got, err := client.CachedModels(context.Background(), dir, "p", srv.URL+"/v1", key, time.Hour, false)
	if err != nil {
		t.Fatalf("CachedModels: %v", err)
	}
	if len(got) != 1 || got[0] != "cached" {
		t.Fatalf("CachedModels = %v, muốn [cached]", got)
	}
	if called != 0 {
		t.Fatalf("cache còn hạn nhưng vẫn gọi mạng %d lần", called)
	}
	if keyCalls != 0 {
		t.Fatalf("cache còn hạn nhưng vẫn gọi key() %d lần", keyCalls)
	}
}

func TestCachedModelsExpiredFetchesAndSaves(t *testing.T) {
	dir := t.TempDir()
	if err := provider.SaveCache(dir, "p", &provider.Cache{
		FetchedAt: time.Now().Add(-2 * time.Hour),
		Models:    []string{"old"},
	}); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_, _ = w.Write([]byte(`{"data":[{"id":"net"}]}`))
	}))
	defer srv.Close()

	client := provider.NewClient()
	got, err := client.CachedModels(context.Background(), dir, "p", srv.URL+"/v1", func() (string, error) {
		return "sk-x", nil
	}, time.Hour, false)
	if err != nil {
		t.Fatalf("CachedModels: %v", err)
	}
	if len(got) != 1 || got[0] != "net" {
		t.Fatalf("CachedModels = %v, muốn [net]", got)
	}
	if called != 1 {
		t.Fatalf("cache hết hạn phải gọi mạng đúng 1 lần, nhận %d", called)
	}

	saved, err := provider.LoadCache(dir, "p")
	if err != nil {
		t.Fatalf("LoadCache sau fetch: %v", err)
	}
	if len(saved.Models) != 1 || saved.Models[0] != "net" {
		t.Fatalf("cache không được ghi lại: %+v", saved)
	}
	if time.Since(saved.FetchedAt) > time.Minute {
		t.Fatalf("fetched_at không được cập nhật: %v", saved.FetchedAt)
	}
}

func TestCachedModelsRefreshForcesFetch(t *testing.T) {
	dir := t.TempDir()
	if err := provider.SaveCache(dir, "p", &provider.Cache{
		FetchedAt: time.Now(),
		Models:    []string{"cached"},
	}); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_, _ = w.Write([]byte(`{"data":[{"id":"net"}]}`))
	}))
	defer srv.Close()

	client := provider.NewClient()
	got, err := client.CachedModels(context.Background(), dir, "p", srv.URL+"/v1", func() (string, error) {
		return "sk-x", nil
	}, time.Hour, true)
	if err != nil {
		t.Fatalf("CachedModels: %v", err)
	}
	if called != 1 {
		t.Fatalf("refresh=true phải gọi mạng đúng 1 lần, nhận %d", called)
	}
	if len(got) != 1 || got[0] != "net" {
		t.Fatalf("CachedModels = %v, muốn [net]", got)
	}
}

func TestCachedModelsKeyErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	wantErr := errors.New("không lấy được key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"net"}]}`))
	}))
	defer srv.Close()

	client := provider.NewClient()
	_, err := client.CachedModels(context.Background(), dir, "p", srv.URL+"/v1", func() (string, error) {
		return "", wantErr
	}, time.Hour, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("lỗi = %v, muốn bọc %v", err, wantErr)
	}
}
