# Privacy & Compliance

## What this tool does with your data

Claude Migrator runs **entirely on your computer**. It reads your local Claude
folders, writes one backup zip to a folder you choose, and restores from that
zip. That's all.

- **No network calls.** The only "server" is a localhost web page used as the UI
  (bound to 127.0.0.1, random port, unreachable from the network).
- **No telemetry, no analytics, no accounts.** We never see your data — there is
  no "us" in the data path at all.
- **No third-party code.** Pure Go standard library; the supply-chain surface is
  the Go toolchain itself.
- Machine-bound credentials (logins, browser/connector pairings) are deliberately
  excluded from backups.

## GDPR

The tool processes personal data only on your device, under your control. No data
is transmitted to the author or any third party, so no controller/processor
relationship arises. Your backup zip may contain personal data — store and
transfer it as carefully as the original, and delete it when done (that is the
whole "erasure" story: delete the file).

## HIPAA

Claude Migrator never receives data, so the author is not a business associate
and no BAA is applicable. Organizations subject to HIPAA can evaluate it as a
local-only utility: PHI never leaves the machine unless *you* move the backup
zip. Backups are **not encrypted** — if your Claude data contains PHI, keep the
zip on encrypted storage (FileVault/BitLocker) or an encrypted volume.

## SOC 2

SOC 2 audits service organizations that operate services handling customer data.
Claude Migrator is an offline binary, not a service — there is no vendor-side
processing to audit. For vendor-review questionnaires the accurate answer is:
"local tool, zero data collection, zero data transmission, open source."

## Verifying the binary

Builds are reproducible from source (`scripts/build.sh` or the GitHub Actions
release workflow). If you don't want to trust a downloaded binary, build it
yourself: `go build .` is the entire process.
