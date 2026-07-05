# Releasing Claude Migrator

## Build

Tag push builds everything via CI (`.github/workflows/release.yml`):

```bash
git tag vX.Y.Z && git push --tags
```

Locally: `bash scripts/build.sh` → `dist/ClaudeMigrator-win.exe` + `dist/ClaudeMigrator-mac.tar.gz`.

## Code signing (recommended once the project has users)

Unsigned binaries trigger Windows SmartScreen and macOS Gatekeeper warnings.
Signing removes that friction. Both are optional; the app works without them.

### Windows (Authenticode)

Requires an OV or EV code-signing certificate (DigiCert, Sectigo, Azure Trusted
Signing, ~$100–400/yr). Then:

```powershell
signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 ^
  /f mycert.pfx /p <password> ClaudeMigrator-win.exe
```

SmartScreen reputation builds over time even with OV; EV starts trusted.

### macOS (Developer ID + notarization)

Requires an Apple Developer account ($99/yr) with a "Developer ID Application"
certificate:

```bash
codesign --force --options runtime --timestamp \
  --sign "Developer ID Application: YOUR NAME (TEAMID)" ClaudeMigrator.app
ditto -c -k --keepParent ClaudeMigrator.app CM.zip
xcrun notarytool submit CM.zip --apple-id you@example.com \
  --team-id TEAMID --password <app-specific-pw> --wait
xcrun stapler staple ClaudeMigrator.app
tar -czf ClaudeMigrator-mac.tar.gz ClaudeMigrator.app
```

### Until then

- **Windows:** SmartScreen → "More info" → "Run anyway".
- **macOS:** right-click the app → **Open** (first launch only).
