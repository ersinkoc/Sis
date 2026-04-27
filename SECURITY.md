# Security Policy

Sis is a DNS gateway and administrative control plane, so security reports are handled with
priority even before the first public stable release.

## Supported Versions

Until `v1.0.0`, security fixes are made on `main` and included in the next `v0.x` release.
After `v1.0.0`, the latest minor release line will receive security fixes.

| Version | Supported |
|---------|-----------|
| `main`  | Yes       |
| `v0.x` latest | Yes |
| Older prereleases | No |

## Reporting A Vulnerability

Please do not open a public issue for a suspected vulnerability.

Use one of these private channels instead:

- GitHub private vulnerability reporting, if enabled for the repository.
- A direct private message to the repository owner if private reporting is not enabled.

Include:

- affected version or commit,
- deployment mode,
- reproduction steps,
- impact,
- logs or packet captures with secrets removed.

## Release Verification

Before installing a release, verify artifacts:

```sh
SIS_RELEASE_DIST=dist ./scripts/verify-release-artifacts.sh
```

When release signing is configured, verify the checksum signature:

```sh
gpg --import dist/release-signing-public-key.asc
gpg --verify dist/SHA256SUMS.asc dist/SHA256SUMS
```

## Operational Hardening

- Keep the HTTP listener on localhost unless it is protected by a trusted management network,
  firewall, VPN, or reverse proxy.
- Use TLS when exposing the HTTP API beyond localhost.
- Treat config files and backups as secrets; they can contain password hashes, sessions,
  client metadata, and privacy salts.
- Run the packaged systemd unit where possible; it includes capability bounding and sandboxing.
- Run `scripts/verify-linux-service.sh` after installation and upgrades.
