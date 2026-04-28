# Production Validation Record

This document is the durable record for the first real host/network validation. Do not mark
the deployment fully production-validated until the checks below are run on the target host
and the evidence is copied or summarized here.

## Current Status

- Status: Pending live host validation
- Last repository release gate: passing
- Last GitHub CI gate: passing
- Last production validation report:
- Validation binary:
- Validation config:
- Validation LAN DNS server:
- Validation API URL:
- Live Linux host install: not recorded
- Live SQLite migration dry-run: not recorded
- Live LAN DNS validation: not recorded
- Live client traffic observation: not recorded

## Required Command

Run this on the target host after installation, DNS binding, and router/DHCP DNS updates:

```sh
sudo SIS_PROD_VALIDATE_LAN_DNS_SERVER=192.168.1.2:53 \
  SIS_PROD_VALIDATE_STRICT=1 \
  SIS_PROD_VALIDATE_BLOCKED_DOMAIN=blocked.example.com \
  SIS_PROD_VALIDATE_API_COOKIE='sis_session=...' \
  SIS_PROD_VALIDATE_REAL_CLIENT=1 \
  SIS_PROD_VALIDATE_REAL_CLIENT_ID=192.168.1.50 \
  SIS_PROD_VALIDATE_DIAGNOSTICS=1 \
  ./scripts/run-production-validation.sh
```

Adjust `SIS_PROD_VALIDATE_LAN_DNS_SERVER` to the host IP and DNS port clients use. Use a
real domain from the deployed block policy for `SIS_PROD_VALIDATE_BLOCKED_DOMAIN`.
Use a short-lived authenticated session cookie for `SIS_PROD_VALIDATE_API_COOKIE`; the
generated report redacts the cookie value from the logged command.
Set `SIS_PROD_VALIDATE_REAL_CLIENT_ID` to a real client IP/key after that client has made
at least one DNS query through Sis; omit it only when any query-log entry is acceptable.
`SIS_PROD_VALIDATE_STRICT=1` fails before running checks when the report would be incomplete
for a release-candidate gate.

## Acceptance Criteria

All required checks must pass:

- `verify-linux-service.sh` confirms binary, config, store readability, service state,
  HTTP health/readiness, and DNS query.
- `validate-sqlite-migration.sh` creates a backup, restores it into a temporary directory,
  migrates that copy to SQLite, exports it back to JSON, and validates SQLite config loading.
- `validate-lan-dns.sh` confirms UDP DNS, TCP DNS, optional blocked-domain policy, and HTTP
  health/readiness from the validation environment.
- Authenticated API store verification confirms `/api/v1/system/store/verify` is reachable
  through the management API when `SIS_PROD_VALIDATE_API_COOKIE` is provided.
- At least one real client uses Sis through router/DHCP DNS settings and appears in the
  clients or query log API. `SIS_PROD_VALIDATE_REAL_CLIENT=1` runs this check through
  `scripts/validate-real-client.sh`.
- A diagnostics bundle is generated without including config, database, backup contents, or
  journal logs unless explicitly accepted.

## Evidence To Paste

After running `scripts/run-production-validation.sh`, copy the summary section from the
generated `sis-validation/production-validation-*.md` report here, or run:

```sh
./scripts/update-production-validation-record.sh sis-validation/production-validation-YYYYMMDDTHHMMSSZ.md
```

The helper updates only the generated summary and results table sections. Keep host details
and real-client observations accurate by editing them after the live run.

<!-- sis-validation-summary:start -->
```text
Paste validation summary here.
```
<!-- sis-validation-summary:end -->

## Host Details

- Validation date:
- Sis version:
- Commit or release tag:
- Host OS/kernel:
- Host IP:
- DNS listen address:
- HTTP listen address:
- Store backend:
- Data directory:
- Router/DHCP DNS setting:

## Results

| Check | Result | Notes |
| --- | --- | --- |
| Service verification | Pending | |
| SQLite migration dry-run | Pending | |
| LAN UDP DNS | Pending | |
| LAN TCP DNS | Pending | |
| Blocked-domain policy | Pending | |
| HTTP health/readiness | Pending | |
| Authenticated API store verification | Pending | |
| Real client query observed | Pending | Use SIS_PROD_VALIDATE_REAL_CLIENT=1 during live validation. |
| Diagnostics bundle generated | Pending | |

## Open Risks Until Completed

- Port 53 binding, firewall rules, and router/DHCP behavior are environment-dependent.
- Client devices may cache previous DNS settings until DHCP renewal or reconnect.
- SQLite operational collections are normalized and backup-aware, but live disk, firewall,
  router, and client behavior still need target-host validation.
