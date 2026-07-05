# Security Policy

## Supported versions

Only the latest release is supported.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting ("Report a vulnerability"
under the Security tab) on this repository. You should receive a response within
a few days. Please do not open public issues for security reports.

## Scope notes

- The UI listens on 127.0.0.1 only, with a random port, and the process exits
  when the page closes.
- Backups are plain zips (not encrypted); treat them like the data they contain.
- The binaries are currently unsigned; verify by building from source if needed.
