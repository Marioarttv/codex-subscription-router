# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/) and
this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- One-command installer with safe source updates, prerequisite checks, signed
  rebuilds, recoverable upgrades, and automatic launch.
- Reset-aware routing that prioritizes weekly quota at risk of expiring and
  gives a bounded boost to subscriptions with banked usage resets.
- Persistent Automatic/manual subscription selection for new chats.
- Compact per-account next-weekly-reset dates with visible 24-hour local times
  in the profile menu.
- Confirmed removal of secondary subscription profiles and their isolated login
  data, with safeguards for Primary and subscriptions that still own tasks.
- Atomic cross-account history handoff for existing chats, including paginated
  Codex rollouts and automatic failover.
- Five-hour plus weekly quota display and capacity checks for routing, manual
  selection, pooled depletion, and earliest-reset reporting.
- Guarded automatic continuation after a structured terminal usage-limit
  failure: preserve the failed turn, hand off the completed history, and send
  one text-only `Continue.` turn on the next available subscription.
- Validated renderer patching and signing for ChatGPT `26.825.51511` build
  `7377`.
- One context-aware account selector in the profile menu: it sets the next-task
  route on the home screen and moves the current task inside a conversation.
  A status-only footer always shows that scope and account; the duplicate task
  header and details-panel selectors were removed.
- Shortened visible weekly quota labels to `w` in compact account controls.

## [0.1.0] - 2026-08-15

### Added

- Multi-subscription routing with quota-aware balancing and sticky threads.
- Account isolation, device-code sign-in, pooled usage, and quota failover.
- Native account menu, masked emails, plan labels, and profile photos.
- Combined Profile statistics with per-account selection.
- Account-scoped Apps and MCP connection state in Settings → Plugins.
- Per-account rate-limit reset selection and pooled depletion handling.
- Independently signed Appshots and Computer Use support.
- Fail-closed upstream compatibility checks and deepest-first nested helper signing.
- Loopback-only, token-authenticated diagnostic UI states.
- Source-only CI, draft release automation, security documentation, and smoke tests.

[Unreleased]: https://github.com/Marioarttv/codex-subscription-router/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/b-nnett/codex-subscription-router/releases/tag/v0.1.0
