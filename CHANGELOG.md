# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

## [0.3.1] - 2026-10-22

### Fixed

- `keychain`: on macOS, replacing or removing a token saved by another build of the tool no longer fails with "An invalid attempt to change the owner of an item. (-25244)". The item is removed with Apple's `security` tool instead, and the new one belongs to the current build. If that fails too, the error names the item to delete in Keychain Access.

## [0.3.0] - 2026-10-16

### Added

- `picker`: a full-screen fuzzy finder for the terminal (type to filter, fzf-style matching and keys, action keys, alternate screen) and a scrollable text view.
- `people`: `LabelStore` and `FileStore` keep labels across sessions of a profile, with file locking so programs sharing the file agree; labels from earlier sessions resolve for mentions.

## [0.2.0] - 2026-10-13

### Added

- `keychain` stores tokens through the Secret Service on Linux (GNOME Keyring, KWallet), using github.com/zalando/go-keyring, with no fallback to files or environment variables. CI checks it against a real GNOME Keyring.

## [0.1.0] - 2026-10-12

### Added

- Packages `approval`, `people`, `atlassian`, `profile`, `keychain` and `sanitize`, extracted from tix-for-jira and generalised for Confluence.
- `atlassian` routes scoped tokens through the API gateway for Jira and Confluence, reads the error formats of both, and names a missing scope through `WithScopeHint`.

[0.3.1]: https://github.com/nyactl/approvekit/releases/tag/v0.3.1
[0.3.0]: https://github.com/nyactl/approvekit/releases/tag/v0.3.0
[0.2.0]: https://github.com/nyactl/approvekit/releases/tag/v0.2.0
[0.1.0]: https://github.com/nyactl/approvekit/releases/tag/v0.1.0
