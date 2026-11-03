# AgentCanvas build (§4.1). One binary: agent-canvas, CGO disabled.

GO       ?= go
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
LDFLAGS  := -s -w -X agentcanvas/internal/cli.Version=$(VERSION)
DIST     := dist
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all vendor vendor-verify test build release clean

all: test build

# Fetch every pinned asset listed in web/vendor/VERSIONS and verify its sha256.
vendor:
	@set -e; cd web/vendor; \
	grep -v '^#' VERSIONS | while IFS="$$(printf '\t')" read -r path name version url sum; do \
	  [ -n "$$path" ] || continue; \
	  mkdir -p "$$(dirname "$$path")"; \
	  curl -fsSL -o "$$path.tmp" "$$url"; \
	  got=$$( (shasum -a 256 "$$path.tmp" 2>/dev/null || sha256sum "$$path.tmp") | cut -d' ' -f1); \
	  if [ "$$got" != "$$sum" ]; then echo "sha256 mismatch for $$path: got $$got, want $$sum" >&2; rm -f "$$path.tmp"; exit 1; fi; \
	  mv "$$path.tmp" "$$path"; echo "ok  $$path  ($$name $$version)"; \
	done

# Verify the vendored files already on disk without downloading.
vendor-verify:
	@set -e; cd web/vendor; \
	grep -v '^#' VERSIONS | while IFS="$$(printf '\t')" read -r path name version url sum; do \
	  [ -n "$$path" ] || continue; \
	  got=$$( (shasum -a 256 "$$path" 2>/dev/null || sha256sum "$$path") | cut -d' ' -f1); \
	  if [ "$$got" != "$$sum" ]; then echo "sha256 mismatch for $$path" >&2; exit 1; fi; \
	  echo "ok  $$path"; \
	done

test:
	$(GO) vet ./...
	$(GO) test ./...

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/agent-canvas ./cmd/agent-canvas

# Six release binaries: linux/darwin/windows × amd64/arm64.
release:
	@set -e; mkdir -p $(DIST); \
	for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=; \
	  if [ "$$os" = windows ]; then ext=.exe; fi; \
	  out=$(DIST)/agent-canvas-$$os-$$arch$$ext; \
	  echo "build $$out"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/agent-canvas; \
	done; \
	ls -l $(DIST)

clean:
	rm -rf $(DIST)
