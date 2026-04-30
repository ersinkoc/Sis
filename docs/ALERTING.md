# Alerting Guide

Scope: operator alert definitions for the current v1 small-site deployment posture.

Sis exposes Prometheus text metrics on `/metrics` from the configured management listener.
Keep the management listener on localhost, VPN, or a trusted management network before
scraping it from another host.

## Critical Alerts

| Signal | Check | Suggested trigger | Action |
|---|---|---|---|
| Service down | `systemctl is-active sis` or HTTP `/healthz` | Fails for 2 consecutive checks over 1 minute | Restart once, then collect diagnostics and inspect journal. |
| Not ready | HTTP `/readyz` | Returns non-200 for 2 consecutive checks over 1 minute | Run `sis config check`, `sis store verify`, upstream health checks, and DNS listener validation. |
| Store unreadable | `sis store verify -config /etc/sis/sis.yaml` | Any non-zero exit | Stop risky changes, take filesystem snapshot if possible, restore last verified backup if needed. |
| DNS path failed | `scripts/validate-lan-dns.sh` | UDP or TCP query fails from validation host | Check bind address, firewall, router/DHCP DNS settings, and upstream health. |
| Backup failed | `scripts/backup-linux-service.sh` | Non-zero exit or missing backup artifact | Do not upgrade; fix config/store path and rerun backup verification. |

## Warning Alerts

| Signal | Check | Suggested trigger | Action |
|---|---|---|---|
| Upstream degraded | `sis upstream -cookie ... health` or `/readyz` details | Any required upstream unhealthy for 5 minutes | Test upstreams, confirm bootstrap reachability, rotate to a healthy resolver if needed. |
| Rate limiting increased | `rate_limited_total` from stats summary or rollups | Sustained increase outside expected client volume | Check client loops, malware/noisy clients, DNS amplification attempts, and configured rate limits. |
| Malformed DNS increased | `malformed_total` from stats summary or rollups | Sustained increase above baseline | Inspect source clients and firewall exposure; malformed packets should be rare on trusted LANs. |
| Store growth abnormal | `sis store verify` collection counts, DB size | Unexpected growth across daily checks | Review retention settings, client/session churn, and query history policy. |
| Diagnostics needed | `scripts/collect-linux-diagnostics.sh` | Any critical alert lasting more than 5 minutes | Generate bundle with journal excluded by default; include journal only after accepting metadata exposure. |

## Manual Check Commands

```sh
sudo systemctl is-active sis
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/metrics
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo /usr/local/bin/sis store verify -config /etc/sis/sis.yaml
sudo ./scripts/validate-lan-dns.sh
sudo ./scripts/backup-linux-service.sh
sudo ./scripts/collect-linux-diagnostics.sh
```

Authenticated stats/API checks need a short-lived management cookie:

```sh
sis stats -cookie 'sis_session=...' top-domains
sis system -cookie 'sis_session=...' store-verify
sis upstream -cookie 'sis_session=...' health
```

## Paging Policy

- Page immediately for service down, readiness failure, store unreadable, or DNS path failed.
- Page before any upgrade if backup creation or backup verification fails.
- Open a ticket, not a page, for isolated upstream degradation when failover remains healthy.
- Open a ticket for increased rate-limited or malformed counters unless they correlate with
  DNS failure, CPU saturation, or untrusted-network exposure.

## Missing Automation

1. No bundled alert manager integration exists.
2. Thresholds should be calibrated from the target site baseline after live production
   validation and sustained load testing.
