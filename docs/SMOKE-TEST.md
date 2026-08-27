# Signed-app smoke test

Complete this checklist on the exact official build recorded in
`docs/COMPATIBILITY.md` before publishing a release draft. Use a team-backed
signature and reuse the same Apple team as the previous installed build.

## Build and identity

- Confirm the patcher reports the expected version, build, and ASAR SHA-256.
- Verify the official `/Applications/ChatGPT.app` is unchanged.
- Verify the app and every nested Computer Use application with
  `codesign --verify --deep --strict`.
- Confirm the installed app and helper report the intended bundle IDs and the
  same `TeamIdentifier`.

## Accounts and routing

- Connect at least two subscriptions and confirm photos, plans, masked emails,
  separate five-hour/weekly percentages, compact weekly `DD/MM` reset dates,
  visible 24-hour reset times for both windows, and loading states.
- Select each subscription from the profile menu, start a new chat, and confirm
  the chat is pinned to that subscription. Restore Automatic routing and verify
  that the selection persists after an app restart.
- On the home screen, confirm the profile footer says `Next task` and displays
  Automatic routing or the manually pinned account without opening the menu.
- Open an existing task and confirm the footer changes to `Using now`. Collapse
  the task-summary panel and verify the header still shows the `Using` account
  dropdown. Switch through that dropdown and confirm both indicators update;
  duplicate labels must be distinguishable by email.
- Start chats until each account has received one; confirm every follow-up stays
  on its original account.
- Open an existing chat's Subscription picker, move it to another account, and
  confirm the same thread ID resumes there after an app restart. Verify the
  picker includes emails when account labels are duplicated.
- Spoof one account with five-hour usage at 100% but weekly usage below 100%;
  confirm it is excluded from new turns and disabled in the thread picker.
- Complete a turn with `status: failed` and
  `codexErrorInfo: usageLimitExceeded`; confirm the failure appears once, the
  same thread moves to another account, and exactly one `Continue.` turn starts.
  Confirm a completed turn and an unrelated failed turn never trigger recovery.
- Spoof all accounts depleted and confirm the combined alert uses the earliest
  limiting-window reset, including a five-hour reset when applicable.
- Open a quota-triggered reset sheet, switch subscriptions, consume a reset, and
  confirm only the selected account changes.

## Settings and plugins

- Confirm Profile opens in the combined state, uses 20 px avatar overlap, and
  toggles between combined and per-account statistics.
- In Settings → Plugins, select each subscription and verify Apps, MCP status,
  and MCP OAuth login reflect that account while installed definitions remain
  shared.

## Appshots and Computer Use

- In System Settings, grant Accessibility to Codex Subscription Router and
  Screen & System Audio Recording to Codex Subscription Router Computer Use.
  Quit and reopen when macOS asks.
- Capture an Appshot from the attachment menu and with the Command-key shortcut.
- Run a Computer Use task and confirm the native helper performs the action
  without falling back to `osascript`.
- Rebuild once with the same signing team and confirm existing permissions still
  work without adding duplicate permission rows.

Record the tested commit, macOS version, signing team ID, and any deviations in
the release draft before publishing it.
