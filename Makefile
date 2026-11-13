VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN := bin/agent-desk
STATICCHECK := go run honnef.co/go/tools/cmd/staticcheck@v0.8.2

.PHONY: build dev test lint web e2e screenshot desktop release-cli desktop-universal checksums clean

build: web
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/agent-desk

web: web/node_modules
	cd web && npm run build

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch web/node_modules

dev: web/node_modules
	./scripts/dev.sh

test: web/node_modules
	go test -race ./...
	cd web && npm test

lint: web/node_modules
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	$(STATICCHECK) ./...
	cd web && npm run lint && npm run typecheck

e2e: build
	cd web && npx playwright test

screenshot: build
	cd web && SCREENSHOT_PATH=../docs/screenshot.png npx playwright test -g screenshot

# desktop builds desktop/target/release/bundle/macos/Agent Desk.app (needs
# Rust and the Tauri CLI: cargo install tauri-cli --version "^2").
desktop: build
	mkdir -p desktop/binaries
	cp $(BIN) desktop/binaries/agent-desk-$$(rustc -vV | sed -n 's/^host: //p')
	cd desktop && cargo tauri build

# Releases go to dist/. APP_VERSION is VERSION without its "v" when VERSION
# is a release tag (v1.2.3), else 0.0.0; the app bundle needs a plain
# version number.
DIST := dist
APP_VERSION := $(or $(shell echo "$(VERSION)" | sed -n 's/^v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$$/\1/p'),0.0.0)
CLI_TARGETS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64
APP_BUNDLE := desktop/target/universal-apple-darwin/release/bundle/macos/Agent Desk.app

# release-cli builds the command for each platform into dist/ as
# agent-desk_<version>_<os>_<arch>.tar.gz.
release-cli: web
	mkdir -p $(DIST)/cli
	set -e; for t in $(CLI_TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; name=agent-desk_$(APP_VERSION)_$${os}_$${arch}; \
	  mkdir -p $(DIST)/cli/$$name; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/cli/$$name/agent-desk ./cmd/agent-desk; \
	  cp LICENSE README.md $(DIST)/cli/$$name/; \
	  tar -C $(DIST)/cli -czf $(DIST)/$$name.tar.gz $$name; \
	done

# desktop-universal builds the app for Apple silicon and Intel into
# dist/Agent-Desk_<version>_macos.zip. With APPLE_SIGNING_IDENTITY (or
# APPLE_CERTIFICATE and APPLE_CERTIFICATE_PASSWORD) set, the Tauri CLI signs
# it; with APPLE_API_ISSUER, APPLE_API_KEY and APPLE_API_KEY_PATH set, it
# notarizes and staples it too.
desktop-universal: web
	mkdir -p desktop/binaries $(DIST) bin
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/agent-desk-darwin-arm64 ./cmd/agent-desk
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/agent-desk-darwin-amd64 ./cmd/agent-desk
	cp bin/agent-desk-darwin-arm64 desktop/binaries/agent-desk-aarch64-apple-darwin
	cp bin/agent-desk-darwin-amd64 desktop/binaries/agent-desk-x86_64-apple-darwin
	lipo -create -output desktop/binaries/agent-desk-universal-apple-darwin bin/agent-desk-darwin-arm64 bin/agent-desk-darwin-amd64
	cd desktop && cargo tauri build --target universal-apple-darwin --bundles app --config '{"version":"$(APP_VERSION)"}'
	ditto -c -k --keepParent "$(APP_BUNDLE)" "$(DIST)/Agent-Desk_$(APP_VERSION)_macos.zip"

# checksums writes dist/SHA256SUMS for everything in dist/.
checksums:
	cd $(DIST) && shasum -a 256 *.tar.gz *.zip > SHA256SUMS

clean:
	rm -rf bin tmp web/dist/assets web/dist/index.html
