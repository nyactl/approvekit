# approvekit

[![Go Reference](https://pkg.go.dev/badge/github.com/nyactl/approvekit.svg)](https://pkg.go.dev/github.com/nyactl/approvekit)
[![CI](https://github.com/nyactl/approvekit/actions/workflows/ci.yml/badge.svg)](https://github.com/nyactl/approvekit/actions/workflows/ci.yml)

Building blocks for command-line tools and MCP servers that let an assistant work with **Jira Cloud and Confluence Cloud** on a person's behalf, while that person keeps control:

- **every change is approved by the human**, in an MCP elicitation dialog the model cannot answer;
- **other people stay private**: account IDs, names and emails are replaced by stable labels such as "Person A";
- **the API token stays in the system's secret store** (the macOS Keychain, or the Secret Service on Linux) and only ever goes to the configured site or Atlassian's API gateway.

approvekit is shared by [tix-for-jira](https://github.com/nyactl/tix-for-jira) and dok-for-confluence. Rich text conversion lives in its own module, [go-adf](https://github.com/nyactl/go-adf).

## Packages

| Package | What it does |
|---|---|
| [`approval`](approval) | An MCP approval gate: a write tool asks the human through an elicitation and applies the change only after approval. Approvals are single-use, expire after 10 minutes, and are bound by an HMAC to the tool, its arguments and the exact text the human saw. |
| [`people`](people) | Replaces people by stable labels ("Me", "Person A", ...), redacts known names and emails in free text, and resolves labels back for mentions. A file store keeps labels across sessions of a profile, holding account IDs and labels only. |
| [`atlassian`](atlassian) | HTTP core for Atlassian Cloud REST APIs: API token authentication, routing through the API gateway for scoped tokens, retries on rate limits, error decoding for Jira and Confluence, uploads, and errors that name a missing scope. Redirects to another host are refused. |
| [`profile`](profile) | Settings per profile (e.g. production and a test site) with owner-only files, site and account validation, token expiry warnings, and a lock that keeps development builds away from production. |
| [`keychain`](keychain) | Token storage in the macOS Keychain (items bound to the creating binary, never synchronised to iCloud) or, on Linux, through the Secret Service (GNOME Keyring, KWallet), with no fallback to files or environment variables. |
| [`picker`](picker) | A full-screen fuzzy finder for the terminal, as fzf: type to filter, arrow keys to move, Enter or action keys to choose, on the alternate screen; and a scrollable full-screen view of text. Everything shown is sanitized. |
| [`sanitize`](sanitize) | Removes terminal control sequences, bidirectional overrides and invisible characters from remote text before it is printed or returned to a model. |

## Status

approvekit is at v0.x: it follows the needs of the two tools that use it, and its API may change between minor versions. Pin a version; the changes are listed in [CHANGELOG.md](CHANGELOG.md). Token storage works on macOS and Linux.

## Install

```sh
go get github.com/nyactl/approvekit
```

Requires Go 1.26 or newer, and cgo on macOS for the Keychain. On Linux no cgo is needed.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md); security problems go through [SECURITY.md](SECURITY.md).

## License

MIT, see [LICENSE](LICENSE).

Not affiliated with or endorsed by Atlassian. Jira, Confluence and Atlassian are trademarks of Atlassian.
