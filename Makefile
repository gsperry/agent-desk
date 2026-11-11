VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN := bin/agent-desk
STATICCHECK := go run honnef.co/go/tools/cmd/staticcheck@v0.8.2

.PHONY: build dev test lint web clean

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

clean:
	rm -rf bin tmp web/dist/assets web/dist/index.html
