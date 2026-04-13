VERSION := 1.0.0
BINARY := smara
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build clean install release

all: build

build:
	CGO_ENABLED=1 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/smara/

install: build
	install -Dm755 $(BINARY) /usr/local/bin/$(BINARY)

uninstall:
	rm -f /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf dist/

test:
	go test ./...

# Build for all platforms (requires cross-compilation setup)
release: clean
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-$(VERSION)-$$os-$$arch$$ext ./cmd/smara/ 2>/dev/null || \
		echo "  ⚠ Skipped $$os/$$arch (CGO required)"; \
	done
	@echo "✓ Release builds in dist/"

# Arch Linux package
pkg-arch:
	makepkg -si

.DEFAULT_GOAL := build
