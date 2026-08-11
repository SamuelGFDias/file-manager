.PHONY: build build-linux build-windows build-all test lint fmt new-tool docs clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/file-manager ./cmd/file-manager

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/file-manager-linux-amd64 ./cmd/file-manager

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/file-manager-windows-amd64.exe ./cmd/file-manager

build-all: build-linux build-windows

test:
	go test ./... -race -cover

lint:
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

fmt:
	gofmt -w .

new-tool:
	go run ./cmd/scaffold $(NAME)

docs:
	mkdir -p dist
	go run ./cmd/file-manager docs export --format context --output dist/file-manager-docs.md
	go run ./cmd/file-manager docs export --format skill --output dist/SKILL.md

clean:
	rm -rf dist/
