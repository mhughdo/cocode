# Release Packaging

This document describes the MVP desktop packaging path. The current release target is macOS; Windows and Linux builder targets are configured as a starting point, but need platform-specific smoke before release.

## Build Commands

```sh
pnpm desktop:dist:dir
pnpm desktop:dist:mac
```

- `pnpm backend:build:mac` builds `services/cocoded/cmd/cocoded` into `apps/desktop/build/bin/cocoded`.
- `pnpm --filter @cocode/desktop build` builds Electron main, preload, and renderer assets into `apps/desktop/out`.
- `pnpm --filter @cocode/desktop dist:dir` creates an unpacked macOS app at `apps/desktop/dist/mac-arm64/cocode.app`.
- `pnpm --filter @cocode/desktop dist:mac` creates distributable macOS artifacts with electron-builder.

The packaged app expects the backend at `Contents/Resources/cocoded` on macOS and Linux, or `Contents/Resources/cocoded.exe` on Windows. `apps/desktop/electron-builder.yml` copies `apps/desktop/build/bin/cocoded*` into Electron resources.

## Local Smoke

Run the unpacked build:

```sh
pnpm desktop:dist:dir
```

Then verify:

- `apps/desktop/dist/mac-arm64/cocode.app/Contents/Resources/cocoded` exists and is executable.
- The app opens a window.
- `window.cocode.getBackendInfo()` reports `status: "ready"`.
- The reported backend URL returns `/api/health` with `status: "ok"`.
- Logs are written under `~/Library/Logs/@cocode/desktop`.

## Signing And Notarization

Local smoke builds are unsigned/ad-hoc signed:

```sh
CSC_IDENTITY_AUTO_DISCOVERY=false pnpm --filter @cocode/desktop dist:dir
```

Release signing needs:

- Apple Developer Program membership.
- Developer ID Application certificate installed in the build keychain.
- Hardened runtime enabled.
- `apps/desktop/build/entitlements.mac.plist` supplied to electron-builder.
- Apple notarization credentials configured in CI or the release shell.

Supported notarization credential sets:

- App Store Connect API key: `APPLE_API_KEY`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER`.
- Apple ID flow: `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID`.
- Keychain profile flow if CI stores notarization credentials in a named keychain profile.

Release validation must attach the signed app, notarization log, and `spctl`/Gatekeeper result to the release notes.

## Update Strategy

MVP uses manual updates only.

- Build with `--publish never`.
- Attach DMG/ZIP artifacts to a GitHub Release manually.
- Users install the new app by replacing the old app bundle.
- No background updater is enabled in MVP.

The macOS `zip` target remains configured so a future auto-update path can be added without changing the artifact shape. Do not enable publish or auto-update until release signing, rollback, and user-facing update prompts are designed.

## Platform Notes

- macOS arm64 is the smoke-tested target.
- macOS x64/universal requires building the matching Go backend binary, or combining arm64/x64 backend binaries with `lipo`.
- Windows packaging requires a `cocoded.exe` backend build and NSIS smoke.
- Linux packaging requires an executable `cocoded` backend build and AppImage smoke.
