// Command hi-llm: quản lý tập trung nhiều LLM provider (API relay + subscription CLI)
// và sinh config cho nhiều harness (opencode, commandcode, my-flow...).
//
// Xem docs tại hi-llm/README.md và hi-llm/SPEC-phase1.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "add":
		return runAdd(args[1:], stdin, stdout, stderr)
	case "ls":
		return runLs(args[1:], stdin, stdout, stderr)
	case "models":
		return runModels(args[1:], stdin, stdout, stderr)
	case "env":
		return runEnv(args[1:], stdin, stdout, stderr)
	case "ping":
		return runPing(args[1:], stdin, stdout, stderr)
	case "role":
		return runRole(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "hi-llm: lệnh không hợp lệ %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `hi-llm — quản lý nhiều LLM provider tập trung

Usage:
  hi-llm add [--name N] [--base-url URL] [--kind openai]
             [--api-key-stdin | --api-key-env VAR] [--force] [--skip-verify]
  hi-llm ls
  hi-llm models [provider] [--refresh] [--ttl DURATION] [--json]
  hi-llm env <provider> [--style openai|anthropic]
  hi-llm ping [provider...] [--chat MODEL]
  hi-llm role [role [value] [--force]]
`)
}

// parseArgs parse flag ở bất kỳ vị trí nào rồi trả các tham số vị trí.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
}
