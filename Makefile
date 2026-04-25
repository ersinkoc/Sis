APP := sis
MODULE := github.com/ersinkoc/sis
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X '$(MODULE)/pkg/version.Version=$(VERSION)' -X '$(MODULE)/pkg/version.Commit=$(COMMIT)' -X '$(MODULE)/pkg/version.Date=$(DATE)'

WEBUI_PM := $(shell command -v pnpm >/dev/null 2>&1 && echo pnpm || echo npm)

.PHONY: build test coverage lint fmt webui webui-check check all release clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/sis

test:
	go test ./...

coverage:
	./scripts/coverage.sh

lint:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './dist/*' -not -path './webui/node_modules/*')

webui:
	cd webui && $(WEBUI_PM) install && $(WEBUI_PM) run build
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	cp -R webui/dist/. internal/webui/dist/

webui-check:
	cd webui && $(WEBUI_PM) install && $(WEBUI_PM) run build && $(WEBUI_PM) run lint

check:
	WEBUI_PM=$(WEBUI_PM) ./scripts/check.sh

all: webui fmt lint coverage build

release:
	./scripts/build.sh

clean:
	rm -rf bin dist
