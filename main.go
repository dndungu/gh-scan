package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/go-github/v62/github"
)

func main() {
	out := flag.String("out", "all-repos.md", "output markdown file")
	orgFlag := flag.String("orgs", "", "comma-separated org list (optional; default: auto-discover all orgs)")
	includePersonal := flag.Bool("personal", true, "include personal account repos")
	mode := flag.String("mode", "cli", "'cli' to generate markdown, 'mcp' to run as stdio MCP server")

	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "GITHUB_TOKEN environment variable is required")
		os.Exit(1)
	}

	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(token)

	s := NewScanner(client, token)

	switch *mode {
	case "cli":
		if err := RunCLI(ctx, s, *out, *orgFlag, *includePersonal); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Wrote %s\n", *out)
	case "mcp":
		if err := RunMCP(ctx, s); err != nil {
			fmt.Fprintln(os.Stderr, "mcp error:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown mode:", *mode)
		os.Exit(1)
	}
}
