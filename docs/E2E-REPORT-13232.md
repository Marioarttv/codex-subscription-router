# Build 13232 verification report

Verified on 6 October 2026 on Apple silicon macOS.

| Input | Value |
| --- | --- |
| Official version | `26.930.61225` |
| Official build | `13232` |
| Official ASAR SHA-256 | `88b8cce6f627771bf341f5a6bb464ad220749b0d442d44f618d7741c2de7318b` |
| Electron framework | `154.0.8037.98` |

## Automated checks

- `npm run check`: Go tests and vet, four JavaScript regression tests, nine
  Python tests, JavaScript/Python syntax and installer shell syntax passed.
- The real C launcher was compiled and exercised with a fake Electron process:
  an inherited official CLI override was replaced with the router CLI; isolated
  settings and shared conversation database paths were retained.
- Embedded integrity tests verify digest replacement, helper signing order,
  and rejection of mismatched source digests or unknown digest versions before
  mutation.
- Notification tests preserve custom callbacks and unrelated settings through
  multiline TOML normalization and repeated runs.
- `npm run release:check` and native launcher syntax passed.

## Installed runtime checks

- The independently signed build launched normally, with the test bridge disabled.
- Strict deep signature verification passed. The Electron embedded dictionary
  digest matched the patched integrity plist; ASAR integrity remains enabled.
- The packaged CLI and legacy aliases started the multiplexer, with a healthy
  authenticated loopback control API.
- With one enabled Plus subscription, a real new chat responded exactly
  `ROUTER_SMOKE_OK`; after restarting the app, a follow-up responded exactly
  `ROUTER_FOLLOWUP_OK`. The persistent thread owner remained Primary.
- The footer changed from next-task scope on Home to current-task scope after
  opening the chat, despite the document URL staying `app://-/index.html`.
- The menu displayed one connected enabled subscription and its live usage.
- The Usage dialog showed the same subscription, quota windows and two available
  native reset credits. No reset credit was redeemed.
- The native Plugins directory and installed Plugins settings loaded. The
  connection selector was checked on the main settings page after correcting
  the anchor from an inner MCP detail page.
- Secondary account records and credentials were retained and disabled;
  credential refreshes preserved their identities. Private state recovery and
  the previous installed app were retained locally, outside Git.

## Scope

This is live verification of a single enabled subscription. Actual quota
exhaustion, multi-account failover, reset redemption, new OAuth connections,
Appshots capture and a router-driven Computer Use task were not performed in
this update. Existing Go tests cover routing and account isolation; historical
E2E reports are separate evidence, not proof of those flows on build 13232.
Personal screenshots, tokens, account identifiers and state files are excluded
from publication. The official installed application was not modified by the
router builder.
