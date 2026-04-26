# Release Process

Sis releases are tag-driven. CI runs the full check gate, benchmark suite, cross-compilation, checksums, and GitHub release publication.

## Cut A Release

1. Make sure `main` is green in CI.
2. Choose the next semantic version tag, for example `v1.0.0`.
3. Create and push the tag:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

4. GitHub Actions builds the release artifacts with:

   ```sh
   make release
   ```

5. The release job uploads:

   - `dist/sis_linux_amd64`
   - `dist/sis_linux_arm64`
   - `dist/sis_darwin_amd64`
   - `dist/sis_darwin_arm64`
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
- The release notes have no private implementation notes.
- `examples/sis.yaml` matches the current config schema.
- `README.md` quick start works from a clean checkout.
