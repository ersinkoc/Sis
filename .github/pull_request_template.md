## Summary

- 

## Validation

- [ ] `./scripts/check.sh`
- [ ] `./scripts/release-readiness.sh v0.0.0-readiness` for release-impacting changes
- [ ] `./scripts/release-candidate-check.sh v0.0.0-rc.1` before release-candidate tags
- [ ] GitHub CI is green

## Release Impact

- [ ] `CHANGELOG.md` updated, or this change does not affect release notes
- [ ] `README.md`, `docs/PRODUCTION.md`, or `.github/RELEASE.md` updated if operator behavior changes
- [ ] New or changed release artifacts are covered by smoke or verification scripts

## Security And Operations

- [ ] No secrets, cookies, password hashes, private client data, or backup contents are committed
- [ ] Auth, session, DNS, logging, backup, or systemd changes include focused tests or smoke coverage
- [ ] Public issues are not used for vulnerability details; `SECURITY.md` remains the reporting path
