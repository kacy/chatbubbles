APP := imsg-bridge
CLI := imsg-bridge-cli

.PHONY: build test fmt run clean

build:
	go build -o bin/$(APP) ./cmd/imsg-bridge

build-cli:
	go build -o bin/$(CLI) ./cmd/imsg-bridge-cli

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

run:
	go run ./cmd/imsg-bridge

clean:
	rm -rf bin
