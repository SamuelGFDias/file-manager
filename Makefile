.PHONY: build build-linux build-windows build-all test e2e lint fmt new-tool docs clean

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

# e2e roda os testes ponta a ponta que abrem o binário real dentro de um
# terminal virtual (internal/testcli). Exigem Linux (/dev/ptmx) e são bem
# mais lentos que a suíte normal (cada cenário inicia um processo e navega
# por prompts reais) — por isso ficam sob a tag de build "e2e" e fora do
# alvo "test" e do "go test ./..." do dia a dia. Compila o binário sob teste
# uma única vez (dentro do próprio TestMain) e roda cada cenário contra ele.
e2e:
	@echo "Rodando testes e2e (terminal virtual, só Linux, mais lentos que 'make test')..."
	go test -tags e2e ./e2e/... -v -count=1

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
