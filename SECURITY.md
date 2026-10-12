# Security policy

## Supported versions

Only the latest release receives fixes.

## Reporting a vulnerability

Please do not open a public issue. Report privately through the repository's **Security** tab → **Report a vulnerability**, with the version or commit, steps to reproduce and the impact you expect.

You will get an acknowledgement within 7 days. Fixes are released before details are published, and reporters are credited unless they prefer otherwise.

## What approvekit promises

Reports that break any of these are in scope:

1. **Approval:** `approval.Gate` lets a change through only after the human approved exactly that change; an approval cannot be replayed, forged, used after 10 minutes, or applied to a change that differs from what was shown.
2. **People:** in mode `Own`, `people.People` never returns another person's account ID, display name or email, and redacts every name and email it has seen in text passed to `Text`.
3. **Token containment:** `atlassian.Client` sends the token only to the configured site or the API gateway and refuses redirects to other hosts; `keychain` keeps the token in the login keychain, not synchronised.
4. **Development builds:** `profile.CheckDevBuild` never lets a build that is not a tagged release use the default profile, or a profile that did not opt in.
5. **Output safety:** `sanitize.String` removes terminal control sequences, bidirectional overrides and invisible characters.

Known limits, not vulnerabilities: names written in free text by people who never appear in structured data; MCP clients configured to answer elicitations automatically.
