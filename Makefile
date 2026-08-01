VERSION := $(shell cat VERSION)
BINARY := smara
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 darwin/amd64 darwin/arm64 windows/amd64

AIR_BIN := $(shell command -v air 2>/dev/null || printf '%s/bin/air' "$$(go env GOPATH 2>/dev/null)")

.PHONY: all build clean install release test-cloud web-build sync-dist dev dev-web dev-backend dev-frontend dev-stop dev-check-tools dev-install-tools

# Build the React front-end and copy the dist into internal/web so that
# //go:embed picks it up. Run this whenever the front-end changes.
web-build:
	cd web && npx vite build

sync-dist: web-build
	rm -rf internal/web/dist
	cp -r web/dist internal/web/dist

build: sync-dist
	CGO_ENABLED=1 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/smara/

install: build
	install -Dm755 $(BINARY) /usr/local/bin/$(BINARY)

uninstall:
	rm -f /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf dist/
	rm -rf internal/web/dist/

test:
	go test ./...

test-cloud:
	@command -v turso >/dev/null 2>&1 || { echo "[skip] 'turso' CLI tidak tersedia di PATH"; exit 0; }
	go test -tags=integration -count=1 ./internal/memory/cloud/...

# Build and package for all platforms
release: clean sync-dist
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		target_name="$(BINARY)-v$(VERSION)-$$os-$$arch"; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$$target_name$$ext ./cmd/smara/; \
		if [ "$$os" = "windows" ]; then \
			(cd dist && zip $$target_name.zip $$target_name.exe && rm $$target_name.exe); \
		else \
			(cd dist && tar -czf $$target_name.tar.gz $$target_name && rm $$target_name); \
		fi; \
	done
	@echo "✓ Release archives in dist/"

# Arch Linux package
pkg-arch:
	makepkg -si

dev: dev-web

# Browser development mode: frontend HMR on :5173 + Go backend auto-reload on :8080.
dev-web: dev-check-tools dev-stop
	@echo "Starting Smara Web development mode..."
	@echo "Frontend: http://127.0.0.1:5173"
	@echo "Backend:  http://127.0.0.1:8080"
	@frontend_pid=""; air_pid=""; \
		cleanup() { \
			[ -n "$$air_pid" ] && kill "$$air_pid" >/dev/null 2>&1 || true; \
			[ -n "$$frontend_pid" ] && kill "$$frontend_pid" >/dev/null 2>&1 || true; \
		}; \
		trap 'cleanup; exit 0' INT TERM; \
		trap 'cleanup' EXIT; \
		(cd web && npm run dev -- --host 127.0.0.1 --port 5173 --strictPort) & \
		frontend_pid=$$!; \
		while true; do \
			"$(AIR_BIN)" & \
			air_pid=$$!; \
			wait $$air_pid; \
			status=$$?; \
			air_pid=""; \
			if ! kill -0 $$frontend_pid >/dev/null 2>&1; then \
				echo "Frontend dev server stopped; exiting."; \
				exit $$status; \
			fi; \
			echo "Backend watcher stopped with status $$status; restarting in 2s..."; \
			sleep 2; \
		done

dev-backend: dev-check-tools
	@air_pid=""; \
		cleanup() { \
			[ -n "$$air_pid" ] && kill "$$air_pid" >/dev/null 2>&1 || true; \
		}; \
		trap 'cleanup; exit 0' INT TERM; \
		trap 'cleanup' EXIT; \
		while true; do \
			"$(AIR_BIN)" & \
			air_pid=$$!; \
			wait $$air_pid; \
			status=$$?; \
			air_pid=""; \
			echo "Backend watcher stopped with status $$status; restarting in 2s..."; \
			sleep 2; \
		done

dev-frontend:
	cd web && npm run dev -- --host 127.0.0.1 --port 5173 --strictPort

dev-stop:
	-@lsof -ti:8080 | xargs -r kill
	-@lsof -ti:5173 | xargs -r kill

dev-check-tools:
	@command -v npm >/dev/null 2>&1 || { echo "Missing 'npm'. Install Node.js/npm first."; exit 1; }
	@command -v curl >/dev/null 2>&1 || { echo "Missing 'curl'. Install curl first."; exit 1; }
	@command -v lsof >/dev/null 2>&1 || { echo "Missing 'lsof'. Install lsof first."; exit 1; }
	@if [ ! -x "$(AIR_BIN)" ]; then \
		echo "Installing air to $$(go env GOPATH)/bin/air..."; \
		go install github.com/air-verse/air@latest; \
	fi

dev-install-tools:
	@go install github.com/air-verse/air@latest
	@echo "air installed at $$(go env GOPATH)/bin/air"
	@command -v npm >/dev/null 2>&1 || { echo "Missing 'npm'. Install Node.js/npm first."; exit 1; }

.DEFAULT_GOAL := build
