# Changelog

## 3.29.2
- Added a "Report a bug" link in the app header and GitHub issue forms (bug report / feature request), so polishing is one click away.

## 3.29.1
- Fixed cross-OS import of connected folders that live outside your home folder: they are now placed under `<home>/ClaudeData/<name>` on the new machine (their original path doesn't exist there) and all project references are rewritten to match. Caught by the first public CI run.

## 3.29.0
- **Connector isolation (fixes cross-machine mixing):** browser/connector pairings (`ChromeNativeHost`, `buddy-tokens.json`) are machine-bound and are now excluded from export and never restored on import -- previously the new machine could keep driving the OLD computer's Chrome through stale pairings. The export records a `connectors.json` list instead, and after import (and in Preview) a popup shows exactly which connectors to reconnect on the new machine.
- Backup zips now have a human name: `ClaudeBackup_YY-MM-DD_HH-MM-SS.zip` (was `ClaudeMigration_<hostname>_<stamp>.zip`).
- Export screen now explains that claude.ai chats and **Claude Design** projects live in your Claude account (cloud) and don't need a local backup, and that connectors reset on import.
- Added `.github/FUNDING.yml` and a Support section in the README.
- Windows exe now embeds version metadata, an application manifest, and the mascot icon (`go-winres` .syso, build-time only) -- reduces antivirus false-positive heuristics and shows proper file properties.
- Added `PRIVACY.md` (local-only: no network, no telemetry; honest GDPR/HIPAA/SOC 2 positioning) and `SECURITY.md` (vulnerability reporting).

## 3.28.0
- **Fixed the macOS "zombie app"**: closing the browser tab used to leave a dead app icon that only Force Quit could remove. The app no longer shows a Dock icon at all (the browser tab is the UI), and the process now exits by itself ~15 seconds after the last tab closes (never during a running export/import). Same auto-exit on Windows.
- New **Quit button** in the header for an immediate, clean shutdown.
- The mascot is now the real app icon (`.icns` in the Mac bundle) and the browser-tab favicon.

## 3.27.0
- **Import now proves itself**: it ends with a verification report — how many conversations will actually resume (re-resolved exactly the way Claude looks them up) and how many connected folders exist on this machine.
- **New "Preview (dry run)" button** on Import: shows exactly what would be restored, merged, and skipped — without writing a single file.
- **Storage-format canary**: Scan and Export now warn if Claude's on-disk layout doesn't match what this version understands (e.g. after a Claude app update), instead of silently producing a questionable backup.
- Source split into focused files (`tokens.go`, `export.go`, `import.go`, `scan.go`, `server.go`, ...) — no behavior change, verified by the full self-test suite.
- Added `docs/RELEASING.md` (build, Windows signing, macOS notarization).

## 3.26.0
- **Fix: imported conversations couldn't be continued** ("No conversation found with session ID ...") and retrying wiped their history. Claude finds each conversation's transcript in a `projects/` folder whose *name* encodes a file path from the old machine; import now renames those path-encoded folders (Cowork sessions and Claude Code) to match the new machine and rewrites the encoded form inside files. Re-running Import over the same package repairs an already-affected machine.
- Export/Import progress bars now show a numeric percentage plus an honest finishing phase ("Verifying archive...", "Restoring files + rewriting paths...") instead of lingering at a full bar.

## 3.25.0
- Housekeeping for the public release: `gofmt`-clean source, no behavior changes.
- Optimized the embedded artwork (lossless, ~40 KB smaller binary).
- README: added a screenshot of the app.

## 3.24.0
- Fix: Code-tab sessions (`claude-code-sessions`) now restore on import. The merge import restores all Cowork data except login/volatile state (an earlier allowlist dropped it).
- Import now shows a live progress bar, matching export.

## 3.22.0 — Initial public release
- Export / Import a full Claude Desktop (Cowork) + Claude Code setup, **Windows ⇄ Mac**.
- Single self-contained binary per OS; embedded local web UI (no runtime, no install).
- **Scan-first** flow (read-only) with live total size and a progress bar during export.
- Connected-folder backup: **all** folders or **pick per project**.
- Merge-safe import with cross-OS path rewriting; never disturbs login or Projects.
- Excludes host-locked / bloat (vm_bundles, credentials, caches, node_modules).
- Projects are labelled by their connected folder name.
- 8-bit walking mascot; sticky header; version shown under the title.
