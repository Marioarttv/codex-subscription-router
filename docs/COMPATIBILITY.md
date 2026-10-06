# Compatibility

The patcher is intentionally tied to known ChatGPT desktop bundle structures.
It verifies every modified renderer, main-process, and native binary anchor and
stops instead of applying a partial patch.

## Release 0.1.0

### Current source: build 13232

- Official version `26.930.61225`, build `13232`, Apple silicon.
- ASAR SHA-256: `88b8cce6f627771bf341f5a6bb464ad220749b0d442d44f618d7741c2de7318b`.
- Official update archive: `https://persistent.oaistatic.com/codex-app-prod/ChatGPT-darwin-arm64-26.930.61225.zip`.
- The Rolldown layout splits native menu items, reset modal, plugin settings and
  app-server RPC transport into separate chunks. Every patch retains exact
  anchor checks and writes only after all renderer checks pass.
- Electron 154 validates an embedded digest of the integrity dictionary. The
  patcher verifies the original digest, updates it for the patched ASAR, and
  re-signs the framework and its helpers with the existing signing team.
- Native memory-router hooks keep the task account selector and footer aligned
  with the open conversation; Primary reset queries use the native authenticated
  request path. Single-account desktop usage was tested.
- The packaged CLI entrypoint is wrapped while retaining the signed CLI app,
  package resources and legacy aliases. The launcher overrides an inherited
  `CODEX_CLI_PATH` so launching from another Codex instance cannot bypass routing.
- Signed-app validation and remaining coverage are recorded in
  [the build 13232 report](E2E-REPORT-13232.md).

### Historical sources

- Build 8109 (`26.901.51231`) is supported with updated profile, usage, and reset
  renderer bindings. ASAR SHA-256:
  `64fc2f27d2dddfa968acfacbe5e4e0328071bdc406351ff4a7d18f0b4692c83d`.
- Its conversation schema was checked against build 7982 before enabling shared
  primary history. Router configuration and macOS identities stay separate.


| Component | Tested value |
| --- | --- |
| Official ChatGPT version | `26.803.61601` |
| Official bundle build | `6396` |
| `app.asar` SHA-256 | `d5a44ed9e2f1db5f81dbbe85408aed256f3203c5b16f00817bb9d7cd941343cf` |
| Architecture | Apple silicon (`arm64`) |

## Previously validated source

| Component | Tested value |
| --- | --- |
| Official ChatGPT version | `26.901.41600` |
| Official bundle build | `7982` |
| `app.asar` SHA-256 | `077cc65356aeae34c5d8b4de0b4cc383f6fb137ed1d69a9b3dfe69ffafa058ab` |
| Architecture | Apple silicon (`arm64`) |

A different official version may work when all anchors remain identical, but
it is unverified. The patcher rejects a version, build, or ASAR hash mismatch by
default; `--allow-untested-source` is an explicit diagnostic override. Never
weaken an anchor-count or binary-constant check merely to make a new build
complete. Review the upstream change and update the patch deliberately.
