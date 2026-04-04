APP := imsg-bridge
CLI := imsg-bridge-cli
VERSION := $(shell tr -d '\n' < VERSION)
BUILD_LDFLAGS := -X github.com/kacy/imsg-bridge/internal/buildinfo.Version=$(VERSION)

.PHONY: build build-cli test fmt run clean dist release release-patch release-minor version

build:
	mkdir -p bin
	go build -ldflags "$(BUILD_LDFLAGS)" -o bin/$(APP) ./cmd/imsg-bridge

build-cli:
	mkdir -p bin
	go build -o bin/$(CLI) ./cmd/imsg-bridge-cli

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

run:
	go run -ldflags "$(BUILD_LDFLAGS)" ./cmd/imsg-bridge

clean:
	rm -rf bin dist

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
