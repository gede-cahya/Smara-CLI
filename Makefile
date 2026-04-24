VERSION := 1.8.6
BINARY := smara
GOFLAGS := -trimpath
LDFLAGS := -s -w -X github.com/gede-cahya/Smara-CLI/cmd/smara.version=$(VERSION)
PLATFORMS := linux/amd64 darwin/amd64 darwin/arm64 windows/amd64

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

# Build and package for all platforms
release: clean
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
