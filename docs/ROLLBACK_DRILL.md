# Rollback Drill

Drill date: 2026-04-30

Scope: local backup/verify/restore drill for the release-hardening branch. The drill used a
temporary JSON store with a sentinel custom-list entry, restored the backup into a separate
config/data directory, then verified the restored config and store.

ASSUMPTION: This drill validates the application backup/restore path in a temporary local
environment. It does not prove systemd stop/start behavior, disk permissions, firewall state,
or router/client behavior on a live production host.

## Commands

```sh
PATH=/tmp/sis-go1.25.9/go/bin:$PATH CGO_ENABLED=0 go build -trimpath -o bin/sis ./cmd/sis
tmp="$(mktemp -d)"
mkdir -p "${tmp}/data"
printf '{"store_meta:schema_version":7,"customlist:custom:rollback-drill.example":true}\n' > "${tmp}/data/sis.db.json"
SIS_DATA_DIR="${tmp}/data" ./bin/sis config check -config examples/sis.yaml
SIS_DATA_DIR="${tmp}/data" ./bin/sis backup create -config examples/sis.yaml -out "${tmp}/rollback-drill.tar.gz"
./bin/sis backup verify -in "${tmp}/rollback-drill.tar.gz"
./bin/sis backup restore -in "${tmp}/rollback-drill.tar.gz" -config "${tmp}/restore/sis.yaml" -data-dir "${tmp}/restore/data"
SIS_DATA_DIR="${tmp}/restore/data" ./bin/sis config check -config "${tmp}/restore/sis.yaml"
SIS_DATA_DIR="${tmp}/restore/data" ./bin/sis store verify -config "${tmp}/restore/sis.yaml"
grep -q rollback-drill.example "${tmp}/restore/data/sis.db.json"
rm -rf "${tmp}"
```

## Result

```text
config ok
backup written to /tmp/tmp.hTviyCGsBj/rollback-drill.tar.gz
backup ok
backup restored to /tmp/tmp.hTviyCGsBj/restore/sis.yaml and /tmp/tmp.hTviyCGsBj/restore/data
config ok
verified json store at /tmp/tmp.hTviyCGsBj/restore/data/sis.db.json (2 records, schema 7)
  customlist: 1
  store_meta: 1
rollback drill: restored config and sentinel store entry verified
```

## Production Rollback Procedure

Use this procedure after a failed upgrade when the last verified pre-upgrade backup is the
desired recovery point:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup verify -in /var/backups/sis/sis-YYYYMMDDTHHMMSSZ.tar.gz
sudo /usr/local/bin/sis backup restore \
  -in /var/backups/sis/sis-YYYYMMDDTHHMMSSZ.tar.gz \
  -config /etc/sis/sis.yaml \
  -data-dir /var/lib/sis \
  -force
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo /usr/local/bin/sis store verify -config /etc/sis/sis.yaml
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

## Remaining Live Drill Work

1. Repeat the rollback procedure on the actual target host after a staged upgrade.
2. Record service stop/start timestamps, restored backup path, `verify-linux-service` output,
   and post-restore DNS/API checks in `docs/PRODUCTION_VALIDATION.md`.
