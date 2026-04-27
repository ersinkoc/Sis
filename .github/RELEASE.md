# Release Process

Sis releases are tag-driven. CI runs the full check gate, benchmark suite, cross-compilation, SPDX SBOM generation, checksum validation, release smoke, and GitHub release publication.

## Cut A Release

1. Make sure `main` is green in CI.
2. Run the local release smoke:

   ```sh
   make release-smoke
   ```

3. Choose the next semantic version tag, for example `v1.0.0`.
4. Create and push the tag:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

5. GitHub Actions builds and smoke-tests the release artifacts with:

   ```sh
   ./scripts/build.sh
   ./scripts/release-smoke.sh
   ```

6. The release job uploads:

   - `dist/sis_linux_amd64`
   - `dist/sis_linux_arm64`
   - `dist/sis_darwin_amd64`
   - `dist/sis_darwin_arm64`
   - `dist/sis.spdx.json`
   - `dist/SHA256SUMS`

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
- The release notes have no private implementation notes.
- `examples/sis.yaml` matches the current config schema.
- `README.md` quick start works from a clean checkout.
- A staged Linux service install works with:

  ```sh
  ./scripts/release-smoke.sh
  ```
