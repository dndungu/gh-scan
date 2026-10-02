# gh-scan

`gh-scan` collects repositories from your GitHub account and organizations. It writes an inventory with each non-fork repository's URL and description, and saves each available README to `outputs/<owner>/<repo>/README.md`. You can write the inventory to a file or expose the scan as an MCP tool.

## Quick start

You need Go 1.22 or newer and a GitHub token that can read the repositories you want to scan. Set `GITHUB_TOKEN` in your shell (do not put the token in a committed file):

```sh
export GITHUB_TOKEN='your-token-here'
go run .
```

Run these commands from this repository's directory. The scan writes `all-repos.md` there, saves READMEs below `outputs/`, and prints the inventory path when complete. It discovers organizations visible to the authenticated user and includes that user's personal repositories by default. Private repositories appear only when the token can access them. If GitHub returns an authorization error, check the token's repository access and organization permissions.

The `outputs/` files can contain private repository content, so review them before sharing. A new run **overwrites** the inventory file and any README files it fetches. It does not remove README files left by earlier runs.

## Choose what to scan

```sh
# Scan two organizations and your personal account.
go run . -orgs acme,example -out inventory.md

# Scan only those organizations.
go run . -orgs acme,example -personal=false -out org-repos.md
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-out` | `all-repos.md` | Output Markdown file path in CLI mode. |
| `-orgs` | auto-discover | Comma-separated organization names to scan. |
| `-personal` | `true` | Include the authenticated user's repositories. Set to `false` for organization-only output. |
| `-mode` | `cli` | Use `cli` to write a file or `mcp` to serve the MCP tool over stdio. |

Forks are excluded. Repositories are grouped by owner and sorted by owner and name. A missing or inaccessible README is skipped; an older file from a previous run may remain. A failure to list an account's repositories stops the scan.

## MCP mode

Build the program, then configure an MCP client to launch it as a **stdio** server:

```sh
go build -o gh-scan .
```

For a client that accepts an MCP server JSON configuration, use the following shape, replacing the executable path and token with your own values:

```json
{
  "mcpServers": {
    "gh-scan": {
      "command": "/absolute/path/to/gh-scan",
      "args": ["-mode=mcp"],
      "env": { "GITHUB_TOKEN": "your-token-here" }
    }
  }
}
```

The server offers one tool, `scan_repositories`:

```json
{ "org_filter": "acme,example", "include_personal": false }
```

Both arguments are optional. With no arguments it scans the visible organizations and personal account, saves READMEs under `outputs/` relative to the MCP server's working directory, and returns the description inventory as tool text. MCP output is capped at about 900,000 characters; use CLI mode when you need the full inventory in a file.

## Troubleshooting

- `GITHUB_TOKEN environment variable is required`: set the token in the same environment that runs the CLI or MCP server.
- `listing orgs` or `listing repos for ...`: check the token's access to the requested account and GitHub API availability.
- Missing README file under `outputs/`: the repository may have no README, or it could not be retrieved. The repository remains in the inventory.
