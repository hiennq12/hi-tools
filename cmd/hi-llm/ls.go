package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"hi-tools/internal/hix"
	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/provider"
	"hi-tools/internal/llm/registry"
)

func runLs(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) > 0 {
		fmt.Fprintf(stderr, "hi-llm ls: tham số thừa %q\n", pos)
		return 2
	}

	home, err := registry.HomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm ls: %v\n", err)
		return 1
	}
	reg, err := registry.Load(registry.Path(home))
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm ls: %v\n", err)
		return 1
	}

	if len(reg.Providers) == 0 && len(reg.Subscriptions) == 0 {
		fmt.Fprintln(stdout, "Chưa có provider nào. Chạy `hi-llm add` để thêm provider đầu tiên.")
		return 0
	}

	cacheDir, err := provider.CacheDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm ls: %v\n", err)
		return 1
	}

	// ls không bao giờ hỏi password: chỉ dùng keystore khi có sẵn env.
	var enc keystore.KeyStore
	if os.Getenv(keystore.EnvMasterPassword) != "" {
		enc = keystore.NewEncStore(keystore.KeysPath(home), keystore.EnvPassword())
	}

	tbl := hix.NewTable("NAME", "KIND", "BASE_URL", "KEY", "MODELS", "PING")
	now := time.Now()
	pings := provider.LoadPings(cacheDir)
	for _, name := range reg.Names() {
		if sub, ok := reg.Subscriptions[name]; ok {
			tbl.Add(name, "sub", sub.Harness, "-", "-", "-")
			continue
		}
		p := reg.Providers[name]
		models := "-"
		if c, err := provider.LoadCache(cacheDir, name); err == nil {
			models = fmt.Sprintf("%d (%s)", len(c.Models), formatAge(now.Sub(c.FetchedAt)))
		}
		ping, pinged := pings[name]
		tbl.Add(name, p.Kind, p.BaseURL, keyStatus(p.APIKey, enc), models, pingStatus(ping, pinged, now))
	}

	if err := tbl.Render(stdout); err != nil {
		fmt.Fprintf(stderr, "hi-llm ls: %v\n", err)
		return 1
	}
	return 0
}

// keyStatus mô tả loại con trỏ + trạng thái, không bao giờ in secret.
func keyStatus(pointer string, enc keystore.KeyStore) string {
	kind, ref, err := keystore.ParsePointer(pointer)
	if err != nil {
		return pointer + " ❌"
	}
	switch kind {
	case "env":
		if _, err := (keystore.EnvStore{}).Get(ref); err != nil {
			return pointer + " ❌ thiếu"
		}
		return pointer + " ✅"
	case "enc":
		if enc == nil {
			return "enc 🔒"
		}
		if _, err := enc.Get(ref); err != nil {
			return "enc ❌"
		}
		return "enc ✅"
	case "plain", "keychain":
		return kind + " ❌ chưa hỗ trợ"
	default:
		return pointer + " ❌"
	}
}

// pingStatus mô tả kết quả ping gần nhất: ✅ 320ms (5m) / ❌ auth (2h) / -.
func pingStatus(r provider.PingResult, ok bool, now time.Time) string {
	if !ok {
		return "-"
	}
	age := formatAge(now.Sub(r.At))
	if r.OK {
		return fmt.Sprintf("✅ %dms (%s)", r.LatencyMS, age)
	}
	return fmt.Sprintf("❌ %s (%s)", r.Kind, age)
}

// formatAge in tuổi cache dạng ngắn: 30s, 5m, 2h.
func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
}
