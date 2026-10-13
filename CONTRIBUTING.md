# Contributing

Thanks for helping improve approvekit. Bug reports, fixes and focused proposals are welcome.

## Before you start

- approvekit serves tix-for-jira and dok-for-confluence. For a new feature, open an issue first and describe which tool needs it.
- Security problems go through private reporting, see [SECURITY.md](SECURITY.md).
- By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Development

Requires Go 1.26 or newer; the Keychain code needs macOS and cgo.

```sh
make test
make lint
APPROVEKIT_KEYCHAIN_TEST=1 go test ./keychain/   # writes to the real store (Keychain or Secret Service)
```

Tests must not contact a real Atlassian site.

## Rules

- Keep each package small and independent of any one product; Jira- or Confluence-specific code belongs in the tools.
- The promises in [SECURITY.md](SECURITY.md) need a test.
- Build strings with invisible characters in tests with escapes such as `\u200b`, never as raw characters.

## Commits and pull requests

- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/); mark breaking changes with `!`.
- Keep pull requests focused; tests, lint and govulncheck must pass.
- Add a line to the "Unreleased" section of [CHANGELOG.md](CHANGELOG.md) for user-visible changes.
