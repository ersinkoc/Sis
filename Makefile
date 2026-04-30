APP := sis
MODULE := github.com/ersinkoc/sis
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X '$(MODULE)/pkg/version.Version=$(VERSION)' -X '$(MODULE)/pkg/version.Commit=$(COMMIT)' -X '$(MODULE)/pkg/version.Date=$(DATE)'

WEBUI_PM := $(shell command -v pnpm >/dev/null 2>&1 && echo pnpm || echo npm)
GO_PACKAGES ?= $(shell go list ./... 2>/dev/null | grep -v '/webui/node_modules/')
BENCHTIME ?= 100ms
BENCHCOUNT ?= 1

.PHONY: preflight build test test-integration coverage bench godoc lint fmt webui webui-check check all release release-smoke clean

preflight:
	./scripts/preflight.sh

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/sis

test:
	go test $(GO_PACKAGES)

test-integration:
	./scripts/integration.sh

coverage:
	./scripts/coverage.sh

bench:
	go test -run '^$$' -bench=. -benchmem -benchtime=$(BENCHTIME) -count=$(BENCHCOUNT) $(GO_PACKAGES)

godoc:
	./scripts/godoc.sh

lint:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './dist/*' -not -path './webui/node_modules/*')

webui:
	cd webui && $(WEBUI_PM) install && $(WEBUI_PM) run build
	./scripts/webui-embed.sh

webui-check:
	cd webui && $(WEBUI_PM) install && $(WEBUI_PM) run build && $(WEBUI_PM) run lint

check: preflight
	WEBUI_PM=$(WEBUI_PM) ./scripts/check.sh

all: preflight webui fmt lint coverage build

release:
	./scripts/build.sh

release-smoke: release
	./scripts/release-smoke.sh

clean:
	rm -rf bin dist
