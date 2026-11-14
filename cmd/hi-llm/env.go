package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/registry"
)

func runEnv(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(stderr)
	style := fs.String("style", "openai", "kiểu env: openai hoặc anthropic")

	pos, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "hi-llm env: cần đúng một tên provider")
		return 2
	}
	name := pos[0]

	home, err := registry.HomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm env: %v\n", err)
		return 1
	}
	reg, err := registry.Load(registry.Path(home))
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm env: %v\n", err)
		return 1
	}
	if reg.IsSubscription(name) {
		fmt.Fprintln(stderr, "provider này là subscription CLI, không có env")
		return 2
	}
	p, ok := reg.Providers[name]
	if !ok {
		fmt.Fprintf(stderr, "hi-llm env: không có provider %q\n", name)
		return 2
	}

	resolver := keystore.Resolver{Enc: keystore.NewEncStore(keystore.KeysPath(home), keystore.PromptPassword(stdin, stderr))}
	key, err := resolver.Resolve(p.APIKey)
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm env: %v\n", err)
		return 1
	}

	lines, err := envLines(*style, p.BaseURL, key)
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm env: %v\n", err)
		return 2
	}
	for _, line := range lines {
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// shellQuote bọc s bằng nháy đơn, thoát nháy đơn bên trong thành '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// envLines sinh các dòng export cho style openai hoặc anthropic.
func envLines(style, baseURL, key string) ([]string, error) {
	switch style {
	case "openai":
		return []string{
			"export OPENAI_API_KEY=" + shellQuote(key),
			"export OPENAI_BASE_URL=" + shellQuote(baseURL),
		}, nil
	case "anthropic":
		return []string{
			"export ANTHROPIC_AUTH_TOKEN=" + shellQuote(key),
			"export ANTHROPIC_BASE_URL=" + shellQuote(strings.TrimSuffix(baseURL, "/v1")),
		}, nil
	default:
		return nil, fmt.Errorf("style %q không hợp lệ (chỉ openai hoặc anthropic)", style)
	}
}
