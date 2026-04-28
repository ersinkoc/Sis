# Release Process

Sis releases are tag-driven. CI runs the full check gate, benchmark suite, cross-compilation, SPDX SBOM generation, checksum validation, release smoke, and GitHub release publication.

## Signing Setup

Checksum signing is optional but recommended for production releases. Add these repository secrets before cutting a public release:

- `RELEASE_GPG_PRIVATE_KEY_B64`: base64-encoded ASCII-armored private key
- `RELEASE_GPG_PASSPHRASE`: passphrase for that private key, if any

When these secrets are present, CI uploads `SHA256SUMS.asc` and `release-signing-public-key.asc` next to the release artifacts. Without them, the release still publishes checksums and SBOM, but the checksum file is not signed.

To create a dedicated unprotected release-signing key for CI, run:

```sh
SIS_RELEASE_GPG_EMAIL=release@example.com ./scripts/generate-release-signing-key.sh
```

The script writes `release-signing-key/RELEASE_GPG_PRIVATE_KEY_B64.txt`; add that
file's single-line value to the `RELEASE_GPG_PRIVATE_KEY_B64` repository secret.
Store `release-signing-key/release-signing-private-key.asc` outside the repository,
then delete the generated `release-signing-key` directory. If you use a passphrase
on a manually-created key, also set `RELEASE_GPG_PASSPHRASE`.

## Cut A Release

1. Make sure `main` is green in CI.
2. Update `CHANGELOG.md` so the release scope, upgrade notes, and known limitations
   match the tag being cut.
3. For a release candidate, make sure the live host validation record is complete:

   ```sh
   ./scripts/release-candidate-check.sh v1.0.0-rc.1
   ```

4. Run the local release readiness gate. For prerelease tags, this also runs the
   release-candidate evidence check before the heavy build/test gate:

   ```sh
   ./scripts/release-readiness.sh v1.0.0
   ```

5. Optionally run the GitHub Actions `CI` workflow manually with `release_version=v1.0.0-dryrun`.
   This exercises the release build, release signing helper, optional signing, and release
   smoke without publishing a GitHub Release.
6. Choose the next semantic version tag, for example `v1.0.0`.
7. Create and push the tag:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

8. GitHub Actions builds and smoke-tests the release artifacts with:

   ```sh
   ./scripts/build.sh
   ./scripts/release-smoke.sh
   ```

9. The release job uploads:

   - `dist/sis_linux_amd64`
   - `dist/sis_linux_arm64`
   - `dist/sis_darwin_amd64`
   - `dist/sis_darwin_arm64`
   - `dist/sis.spdx.json`
   - `dist/SHA256SUMS`
   - `dist/SHA256SUMS.asc` when release signing is configured
   - `dist/release-signing-public-key.asc` when release signing is configured

## Release Notes

GitHub generates release notes automatically from merged pull requests. Labels are grouped by `.github/release.yml`.

Useful labels:

- `breaking-change`
- `security`
- `feature`
- `enhancement`
- `bug`
- `fix`
- `documentation`
- `docs`
- `maintenance`
- `dependencies`
- `ci`
- `skip-changelog`
- `ignore-for-release`

## Manual Checks

Before announcing a release, verify:

- The release contains all binaries and `SHA256SUMS`.
- The release contains `sis.spdx.json`, and `SHA256SUMS` validates it.
- If signing is configured, `gpg --verify SHA256SUMS.asc SHA256SUMS` succeeds.
- The release notes have no private implementation notes.
- `examples/sis.yaml` matches the current config schema.
- `README.md` quick start works from a clean checkout.
- A staged Linux service install works with:

  ```sh
  ./scripts/release-smoke.sh
  ```

Downloaded artifact bundles can be checked with:

```sh
SIS_RELEASE_DIST=dist ./scripts/verify-release-artifacts.sh
```
