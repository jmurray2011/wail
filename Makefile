VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X 'main.version=$(VERSION)' \
  -X 'main.commit=$(COMMIT)' \
  -X 'main.date=$(DATE)'

build:
	go build -ldflags="$(LDFLAGS)" -o wail ./cmd/wail

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o wail.exe ./cmd/wail

.PHONY: build build-windows