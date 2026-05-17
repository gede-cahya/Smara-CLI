VERSION := 1.20.4
BINARY := smara
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build clean install release test-cloud web-build sync-dist

all: build

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
			cd dist && zip $$target_name.zip $$target_name.exe && rm $$target_name.exe && cd ..; \
		else \
			cd dist && tar -czf $$target_name.tar.gz $$target_name && rm $$target_name && cd ..; \
		fi; \
	done
	@echo "✓ Release archives in dist/"

# Arch Linux package
pkg-arch:
	makepkg -si

.DEFAULT_GOAL := build
