APP := chatbubbles
CLI := chatbubbles-cli
VERSION := $(shell tr -d '\n' < VERSION)
BUILD_LDFLAGS := -X github.com/kacy/chatbubbles/internal/buildinfo.Version=$(VERSION)
IMSG_BIN ?= $(shell [ -x ../imsg/bin/imsg ] && printf '%s' ../imsg/bin/imsg || printf '%s' imsg)

.PHONY: build build-cli test fmt run clean dist release release-patch release-minor version web-install web-dev web-build web-test homebrew-formula

	build:
	mkdir -p bin
	go build -ldflags "$(BUILD_LDFLAGS)" -o bin/$(APP) ./cmd/chatbubbles

build-cli:
	mkdir -p bin
	go build -o bin/$(CLI) ./cmd/chatbubbles-cli

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

run:
	go run -ldflags "$(BUILD_LDFLAGS)" ./cmd/chatbubbles -imsg-bin "$(IMSG_BIN)"

clean:
	rm -rf bin dist web/dist web/*.tsbuildinfo web/vite.config.js web/vite.config.d.ts web/tailwind.config.js web/tailwind.config.d.ts

dist:
	sh ./scripts/build-release.sh $(VERSION)

version:
	@printf '%s\n' $(VERSION)

release:
	sh ./scripts/release.sh current

release-patch:
	sh ./scripts/release.sh patch

release-minor:
	sh ./scripts/release.sh minor

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-test:
	cd web && npm test

homebrew-formula:
	@if [ -n "$(ARM64_SHA)" ] && [ -n "$(AMD64_SHA)" ]; then \
		sh ./scripts/render-homebrew-formula.sh "$(VERSION)" "$(ARM64_SHA)" "$(AMD64_SHA)"; \
	else \
		sh ./scripts/render-homebrew-formula.sh "$(VERSION)"; \
	fi
