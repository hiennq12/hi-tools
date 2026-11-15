package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"hi-tools/internal/hix"
	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/provider"
	"hi-tools/internal/llm/registry"
)

// pingOutcome là kết quả một provider trong một lần chạy ping.
type pingOutcome struct {
	result provider.PingResult
	models []string
}

func runPing(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var chatModel string
	fs.StringVar(&chatModel, "chat", "", "kiểm bằng POST /chat/completions với model này")

	pos, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}

	home, err := registry.HomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm ping: %v\n", err)
		return 1
	}
	reg, err := registry.Load(registry.Path(home))
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm ping: %v\n", err)
		return 1
	}
	if len(reg.Providers) == 0 {
		fmt.Fprintln(stdout, "Chưa có provider nào. Chạy `hi-llm add` để thêm provider đầu tiên.")
		return 0
	}

	var names []string
	if len(pos) == 0 {
		for name := range reg.Providers {
			names = append(names, name)
		}
		sort.Strings(names)
	} else {
		for _, name := range pos {
			if reg.IsSubscription(name) {
				fmt.Fprintf(stderr, "hi-llm ping: %q là subscription, subscription không ping được\n", name)
				return 2
			}
			if _, ok := reg.Providers[name]; !ok {
				fmt.Fprintf(stderr, "hi-llm ping: không có provider %q\n", name)
				return 2
			}
		}
		names = pos
	}
	sort.Strings(names)
	if chatModel != "" && len(pos) != 1 {
		fmt.Fprintln(stderr, "hi-llm ping: --chat chỉ dùng khi chỉ định đúng một provider")
		return 2
	}

	cacheDir, err := provider.CacheDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm ping: %v\n", err)
		return 1
	}

	// Resolve key tuần tự trước khi ping để chỉ hỏi master password một lần.
	resolver := keystore.Resolver{Enc: keystore.NewEncStore(
		keystore.KeysPath(home),
		keystore.OncePassword(keystore.PromptPassword(stdin, stderr)),
	)}
	keys := make(map[string]string, len(names))
	keyErrs := make(map[string]error)
	for _, name := range names {
		key, err := resolver.Resolve(reg.Providers[name].APIKey)
		if err != nil {
			keyErrs[name] = err
			continue
		}
		keys[name] = key
	}

	outcomes := make(map[string]pingOutcome, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := reg.Providers[name]
			var out pingOutcome

			if keyErr, bad := keyErrs[name]; bad {
				out.result = provider.PingResult{
					At:        time.Now(),
					LatencyMS: -1,
					Kind:      "key",
					Error:     keyErr.Error(),
				}
				mu.Lock()
				outcomes[name] = out
				mu.Unlock()
				return
			}

			client := provider.NewClient()
			client.Headers = p.Headers

			start := time.Now()
			if chatModel != "" {
				err := client.Chat(context.Background(), p.BaseURL, keys[name], chatModel)
				out.result = provider.PingResult{At: time.Now(), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					out.result.Kind = provider.Classify(err)
					out.result.Error = err.Error()
				} else {
					out.result.OK = true
				}
			} else {
				models, err := client.ListModels(context.Background(), p.BaseURL, keys[name])
				out.result = provider.PingResult{At: time.Now(), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					out.result.Kind = provider.Classify(err)
					out.result.Error = err.Error()
				} else {
					out.result.OK = true
					out.models = models
					if serr := provider.SaveCache(cacheDir, name, &provider.Cache{
						FetchedAt: time.Now(),
						Models:    models,
					}); serr != nil {
						fmt.Fprintf(stderr, "hi-llm ping: ghi cache %s: %v\n", name, serr)
					}
				}
			}

			mu.Lock()
			outcomes[name] = out
			mu.Unlock()
		}()
	}
	wg.Wait()

	results := make(map[string]provider.PingResult, len(outcomes))
	tbl := hix.NewTable("NAME", "STATUS", "LATENCY", "DETAIL")
	allOK := true
	for _, name := range names {
		out := outcomes[name]
		results[name] = out.result
		if !out.result.OK {
			allOK = false
		}

		status := "✅"
		if !out.result.OK {
			status = "❌"
		}
		latency := "-"
		if out.result.LatencyMS >= 0 {
			latency = fmt.Sprintf("%dms", out.result.LatencyMS)
		}
		detail := "chat ok"
		if !out.result.OK {
			detail = out.result.Kind + ": " + out.result.Error
		} else if chatModel == "" {
			detail = fmt.Sprintf("%d models", len(out.models))
		}
		tbl.Add(name, status, latency, detail)
	}

	if err := tbl.Render(stdout); err != nil {
		fmt.Fprintf(stderr, "hi-llm ping: %v\n", err)
		return 1
	}
	if err := provider.SavePings(cacheDir, results); err != nil {
		fmt.Fprintf(stderr, "hi-llm ping: ghi _ping.json: %v\n", err)
		return 1
	}
	if !allOK {
		return 1
	}
	return 0
}
