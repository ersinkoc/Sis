#!/usr/bin/env bash
set -euo pipefail

go test -count=1 -run '^TestSpec19DNSAcceptance' ./internal/dns
go test -count=1 -run '^Test(SetupPersistsConfigAndSessionAcrossRestart|SystemStoreVerify|GroupSchedulePatchAffectsQueryTest|BlocklistSyncEndpointUpdatesPolicyEntries)$' ./internal/api
