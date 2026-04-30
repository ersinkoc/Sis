# Development Setup

This document lists the local tools needed to run the same checks that CI runs.

## Required Tools

| Tool | Version | Used by |
| --- | --- | --- |
| Go | From `go.mod`, currently `1.25.9` | Go tests, vet, build, release scripts, smoke tests |
| Node.js | `24` recommended | WebUI dependencies and Vite 8 toolchain |
| npm | Bundled with Node 24 | `webui/package-lock.json`, WebUI build/lint/e2e |
| curl | system package | HTTP health/readiness checks and smoke scripts |
| bash/coreutils | system packages | project scripts |

Check the basics:

```sh
go version
gofmt --help >/dev/null
node --version
npm --version
curl --version
```

The project preflight checks only `go`, `gofmt`, and `npm` because the other tools are used
later by narrower scripts.

## WebUI Dependencies

Install WebUI packages from the lockfile:

```sh
cd webui
npm ci
```

Build and lint:

```sh
npm run build
npm run lint
```

The root check script runs these through `WEBUI_PM` and `WEBUI_INSTALL`. CI uses:

```sh
WEBUI_PM=npm WEBUI_INSTALL=ci ./scripts/check.sh
```

## Playwright

The WebUI e2e suite uses Playwright Chromium. Install the browser and OS dependencies with:

```sh
cd webui
npx playwright install --with-deps chromium
```

Run the browser smoke against a local Sis process managed by the script:

```sh
./scripts/build.sh
./scripts/webui-smoke.sh
```

Or run Playwright directly against an already running WebUI/API:

```sh
cd webui
SIS_WEBUI_BASE_URL=http://127.0.0.1:18080 npx playwright test
```

Known limitation: Playwright browser packages can lag behind newly released Linux
distributions. If `npx playwright install --with-deps chromium` says the host platform is
unsupported, run the browser smoke in CI or a supported distro/container.

## Main Check Gate

Run the full local gate from the repository root:

```sh
./scripts/check.sh
```

On non-`main` release-hardening branches, set the branch expected by release smoke checks:

```sh
SIS_RELEASE_BRANCH="$(git branch --show-current)" ./scripts/check.sh
```

Run the source-level integration subset separately when you want the SPEC §19 DNS/API
acceptance path without the full gate:

```sh
./scripts/integration.sh
make test-integration
```

The gate performs:

- Go formatting check.
- Clean tracked diff check.
- Go doc comment check.
- Release and production-validation smoke scripts.
- WebUI install/build/lint.
- WebUI embed synchronization check.
- Coverage gate.
- `go vet`.
- Static binary build.
- Runtime smoke test covering health, readiness, DNS, API, CLI, config reload/history, and
  restart persistence.

Because `scripts/check.sh` requires a clean tracked diff, stage or commit intentional changes
before running it. Generated WebUI embed files must be committed when the WebUI build output
changes.

## Useful Development Commands

```sh
make build
make test
make webui-check
./scripts/coverage.sh
./scripts/smoke.sh
./scripts/release-smoke.sh
```

For package benchmarks:

```sh
packages="$(go list ./... | grep -v '/webui/node_modules/')"
go test -run '^$' -bench=. -benchmem -benchtime=100ms -count=1 ${packages}
```

For the current longer local baseline and interpretation, see
[PERFORMANCE_BASELINE.md](PERFORMANCE_BASELINE.md).

For vulnerability scanning, CI runs:

```sh
packages="$(go list ./... | grep -v '/webui/node_modules/')"
go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ${packages}
```

## Local Service Smoke

For a quick manual loop:

```sh
make build
./bin/sis config check -config examples/sis.yaml
./bin/sis serve -config examples/sis.yaml
```

Then in another shell:

```sh
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
./bin/sis query -server 127.0.0.1:5353 test example.com A
```

The example config uses `127.0.0.1:5353` so development does not require privileged port 53.
