package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"time"

	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/provider"
	"hi-tools/internal/llm/registry"
)

type modelEntry struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// ttlFlag ghi nhớ --ttl có được truyền hay không (để báo lỗi khi thiếu provider).
type ttlFlag struct {
	value string
	set   bool
}

func (t *ttlFlag) String() string { return t.value }
func (t *ttlFlag) Set(v string) error {
	t.value = v
	t.set = true
	return nil
}

func runModels(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		refresh bool
		asJSON  bool
	)
	ttl := &ttlFlag{value: provider.DefaultTTL.String()}
	fs.BoolVar(&refresh, "refresh", false, "bỏ cache và gọi lại /models")
	fs.Var(ttl, "ttl", "hạn cache (time.ParseDuration, mặc định 1h)")
	fs.BoolVar(&asJSON, "json", false, "in JSON")

	pos, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) > 1 {
		fmt.Fprintf(stderr, "hi-llm models: quá nhiều tham số %v\n", pos)
		return 2
	}

	home, err := registry.HomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm models: %v\n", err)
		return 1
	}
	reg, err := registry.Load(registry.Path(home))
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm models: %v\n", err)
		return 1
	}
	cacheDir, err := provider.CacheDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm models: %v\n", err)
		return 1
	}

	if len(pos) == 0 {
		if refresh || ttl.set {
			fmt.Fprintln(stderr, "hi-llm models: --refresh/--ttl chỉ dùng kèm provider")
			return 2
		}
		entries := collectFromCache(reg, cacheDir, stderr)
		return outputModels(stdout, stderr, entries, asJSON)
	}

	name := pos[0]
	if reg.IsSubscription(name) {
		fmt.Fprintf(stderr, "hi-llm models: %q là subscription, không có danh sách model qua API\n", name)
		return 2
	}
	p, ok := reg.Providers[name]
	if !ok {
		fmt.Fprintf(stderr, "hi-llm models: không có provider %q\n", name)
		return 2
	}

	ttlDur, err := time.ParseDuration(ttl.value)
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm models: --ttl %q không hợp lệ: %v\n", ttl.value, err)
		return 2
	}

	resolver := keystore.Resolver{Enc: keystore.NewEncStore(keystore.KeysPath(home), keystore.PromptPassword(stdin, stderr))}
	keyFn := func() (string, error) { return resolver.Resolve(p.APIKey) }

	client := provider.NewClient()
	client.Headers = p.Headers

	models, err := client.CachedModels(context.Background(), cacheDir, name, p.BaseURL, keyFn, ttlDur, refresh)
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm models: %v\n", err)
		return 1
	}

	entries := make([]modelEntry, 0, len(models))
	for _, id := range models {
		entries = append(entries, modelEntry{Provider: name, ID: id})
	}
	return outputModels(stdout, stderr, entries, asJSON)
}

// collectFromCache gộp model của mọi provider chỉ từ cache (bỏ qua TTL).
func collectFromCache(reg *registry.Registry, cacheDir string, stderr io.Writer) []modelEntry {
	entries := []modelEntry{}
	names := make([]string, 0, len(reg.Providers))
	for name := range reg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c, err := provider.LoadCache(cacheDir, name)
		if err != nil {
			fmt.Fprintf(stderr, "cảnh báo: provider %s chưa có cache models\n", name)
			continue
		}
		for _, id := range c.Models {
			entries = append(entries, modelEntry{Provider: name, ID: id})
		}
	}
	return entries
}

func outputModels(stdout, stderr io.Writer, entries []modelEntry, asJSON bool) int {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Provider != entries[j].Provider {
			return entries[i].Provider < entries[j].Provider
		}
		return entries[i].ID < entries[j].ID
	})
	if asJSON {
		if entries == nil {
			entries = []modelEntry{}
		}
		if err := json.NewEncoder(stdout).Encode(entries); err != nil {
			fmt.Fprintf(stderr, "hi-llm models: %v\n", err)
			return 1
		}
		return 0
	}
	for _, e := range entries {
		fmt.Fprintf(stdout, "%s/%s\n", e.Provider, e.ID)
	}
	return 0
}
