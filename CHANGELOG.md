# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

## [0.1.0] - 2026-10-12

### Added

- Packages `approval`, `people`, `atlassian`, `profile`, `keychain` and `sanitize`, extracted from tix-for-jira and generalised for Confluence.
- `atlassian` routes scoped tokens through the API gateway for Jira and Confluence, reads the error formats of both, and names a missing scope through `WithScopeHint`.

[0.1.0]: https://github.com/nyactl/approvekit/releases/tag/v0.1.0
