package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"hi-tools/internal/hix"
	"hi-tools/internal/llm/keystore"
	"hi-tools/internal/llm/provider"
	"hi-tools/internal/llm/registry"
)

func runAdd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		name        string
		baseURL     string
		kind        string
		apiKeyEnv   string
		apiKeyStdin bool
		force       bool
		skipVerify  bool
	)
	fs.StringVar(&name, "name", "", "tên provider")
	fs.StringVar(&baseURL, "base-url", "", "base URL của API relay")
	fs.StringVar(&kind, "kind", "openai", "loại provider (chỉ openai)")
	fs.StringVar(&apiKeyEnv, "api-key-env", "", "lưu key dưới dạng env:VAR")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "đọc api key từ stdin (lưu enc:)")
	fs.BoolVar(&force, "force", false, "ghi đè provider đã có")
	fs.BoolVar(&skipVerify, "skip-verify", false, "bỏ qua bước gọi thử /models")

	pos, err := parseArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) > 0 {
		fmt.Fprintf(stderr, "hi-llm add: tham số thừa %q (tên provider dùng --name)\n", pos)
		return 2
	}
	if apiKeyStdin && apiKeyEnv != "" {
		fmt.Fprintln(stderr, "hi-llm add: --api-key-stdin và --api-key-env loại trừ nhau")
		return 2
	}

	tty := hix.IsTTY(stdin)
	in := stdin // đọc không đệm, dùng chung fd với hix.ReadPassword

	if name == "" {
		if !tty {
			fmt.Fprintln(stderr, "hi-llm add: thiếu --name")
			return 2
		}
		v, err := promptLine(in, stderr, "Tên provider: ")
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
			return 1
		}
		name = v
	}
	if err := registry.ValidName(name); err != nil {
		fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
		return 2
	}

	if baseURL == "" {
		if !tty {
			fmt.Fprintln(stderr, "hi-llm add: thiếu --base-url")
			return 2
		}
		v, err := promptLine(in, stderr, "Base URL: ")
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
			return 1
		}
		baseURL = v
	}
	normalized, err := registry.NormalizeBaseURL(baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
		return 2
	}
	baseURL = normalized

	if kind != "openai" {
		fmt.Fprintf(stderr, "hi-llm add: kind %q không hỗ trợ (chỉ nhận openai)\n", kind)
		return 2
	}

	home, err := registry.HomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
		return 1
	}
	reg, err := registry.Load(registry.Path(home))
	if err != nil {
		fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
		return 1
	}
	if reg.IsSubscription(name) {
		fmt.Fprintf(stderr, "hi-llm add: tên %q đã là subscription, không thể thêm provider\n", name)
		return 2
	}
	old, existed := reg.Providers[name]
	if existed {
		if !force {
			if !tty {
				fmt.Fprintf(stderr, "hi-llm add: provider %q đã tồn tại, dùng --force để ghi đè\n", name)
				return 2
			}
			ok, err := confirm(in, stderr, fmt.Sprintf("Provider %q đã tồn tại, ghi đè? [y/N] ", name))
			if err != nil {
				fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
				return 1
			}
			if !ok {
				fmt.Fprintln(stderr, "hi-llm add: đã huỷ")
				return 1
			}
		}
	}

	var (
		pointer string
		secret  string
	)
	switch {
	case apiKeyStdin:
		line, err := readLine(in)
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm add: đọc api key từ stdin: %v\n", err)
			return 1
		}
		secret = strings.TrimSpace(line)
		if secret == "" {
			fmt.Fprintln(stderr, "hi-llm add: api key từ stdin rỗng")
			return 2
		}
		pointer = "enc:" + name
	case apiKeyEnv != "":
		pointer = "env:" + apiKeyEnv
	default:
		if !tty {
			fmt.Fprintln(stderr, "hi-llm add: thiếu api key, dùng --api-key-stdin hoặc --api-key-env VAR")
			return 2
		}
		method, err := promptLine(in, stderr, "Cách lưu key — (1) enc, (2) env: ")
		if err != nil {
			fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
			return 1
		}
		if method == "2" || strings.EqualFold(method, "env") {
			varName, err := promptLine(in, stderr, "Tên biến môi trường: ")
			if err != nil {
				fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
				return 1
			}
			if varName == "" {
				fmt.Fprintln(stderr, "hi-llm add: tên biến môi trường rỗng")
				return 2
			}
			pointer = "env:" + varName
		} else {
			key, err := hix.ReadPassword("API key: ", stdin, stderr)
			if err != nil {
				fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
				return 1
			}
			secret = strings.TrimSpace(key)
			if secret == "" {
				fmt.Fprintln(stderr, "hi-llm add: api key rỗng")
				return 2
			}
			pointer = "enc:" + name
		}
	}

	// Ghi đè chỉ thay thông tin kết nối; header/harness sửa tay được giữ nguyên.
	var keptHeaders map[string]string
	var keptHarnesses []string
	if existed {
		keptHeaders = old.Headers
		keptHarnesses = old.Harnesses
	}

	if !skipVerify {
		key, verr := secret, error(nil)
		if strings.HasPrefix(pointer, "env:") {
			key, verr = (keystore.EnvStore{}).Get(strings.TrimPrefix(pointer, "env:"))
		}
		if verr == nil {
			client := provider.NewClient()
			client.Headers = keptHeaders
			if _, err := client.ListModels(context.Background(), baseURL, key); err != nil {
				verr = fmt.Errorf("xác minh %s/models thất bại: %w", baseURL, err)
			}
		}
		if verr != nil {
			fmt.Fprintf(stderr, "cảnh báo: %v\n", verr)
			if !tty {
				return 1
			}
			ok, cerr := confirm(in, stderr, "Vẫn lưu provider? [y/N] ")
			if cerr != nil {
				fmt.Fprintf(stderr, "hi-llm add: %v\n", cerr)
				return 1
			}
			if !ok {
				return 1
			}
		}
	}

	if strings.HasPrefix(pointer, "enc:") {
		store := keystore.NewEncStore(keystore.KeysPath(home), keystore.PromptPassword(stdin, stderr))
		if err := store.Set(name, secret); err != nil {
			fmt.Fprintf(stderr, "hi-llm add: lưu key: %v\n", err)
			return 1
		}
	}

	if reg.Providers == nil {
		reg.Providers = map[string]registry.Provider{}
	}
	reg.Providers[name] = registry.Provider{
		Kind:      kind,
		BaseURL:   baseURL,
		APIKey:    pointer,
		Headers:   keptHeaders,
		Harnesses: keptHarnesses,
	}
	if err := reg.Validate(); err != nil {
		fmt.Fprintf(stderr, "hi-llm add: %v\n", err)
		return 1
	}
	if err := reg.Save(registry.Path(home)); err != nil {
		fmt.Fprintf(stderr, "hi-llm add: ghi providers.yaml: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "đã thêm provider %s (%s)\n", name, baseURL)
	return 0
}
