BINARY := go-mcp-server
PKG    := ./cmd/server
BIN    := bin/$(BINARY)

GO        ?= go
PORT      ?= 8080
NGROK_URL ?= robust-enough-loon.ngrok-free.app
GOFILES   := $(shell find . -name '*.go' -not -path './vendor/*')

# Load .env if present so `make run` picks up local config.
ifneq (,$(wildcard ./.env))
include .env
export
endif

.DEFAULT_GOAL := help

## help: list available targets
help:
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk '{ \
		name = $$0; sub(/:.*/, "", name); \
		desc = $$0; sub(/^[^:]*: */, "", desc); \
		printf "  \033[36m%-12s\033[0m %s\n", name, desc }'

## run: run the server (PORT=8080)
run:
	$(GO) run $(PKG)

## build: compile the server to bin/
build:
	$(GO) build -o $(BIN) $(PKG)
	@echo "built $(BIN)"

## test: run all tests
test:
	$(GO) test ./...

## testv: run all tests, verbose
testv:
	$(GO) test ./... -v

## cover: run tests and open the coverage report
cover:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out

## fmt: format code (gofmt + goimports via golangci-lint)
fmt:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint fmt ./...; \
	else \
		echo "golangci-lint not installed; falling back to gofmt"; \
		gofmt -w -s $(GOFILES); \
	fi

## vet: run go vet
vet:
	$(GO) vet ./...

## lint: run golangci-lint (config: .golangci.yml)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; skipping"; \
		echo "  brew install golangci-lint"; \
	fi

## lintfix: run golangci-lint and auto-fix what it can
lintfix:
	golangci-lint run --fix ./...

## tidy: tidy and verify go.mod
tidy:
	$(GO) mod tidy

## migrate: apply migrations to $DATABASE_URL
migrate:
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is not set"; exit 1; }
	@for f in migrations/*.sql; do echo "applying $$f"; psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$$f" >/dev/null || exit 1; done
	@echo "migrations applied"

## pgdev: run a local pgvector in docker (needs Docker running)
pgdev:
	docker run -d --name mcp-pg -e POSTGRES_PASSWORD=pg -e POSTGRES_DB=mcp \
		-p 55432:5432 pgvector/pgvector:pg17
	@echo 'DATABASE_URL=postgres://postgres:pg@localhost:55432/mcp?sslmode=disable'

## check: fmt + vet + lint + test
check: fmt vet lint test

## health: curl the health endpoint of a running server
health:
	@curl -fsS http://localhost:$(PORT)/healthz && echo

## tunnel: expose the local server publicly via ngrok (needs `make run` first)
tunnel:
	@command -v ngrok >/dev/null 2>&1 || { echo "ngrok not installed: brew install ngrok"; exit 1; }
	@echo "Namespaces will be reachable at:"
	@echo "  https://$(NGROK_URL)/memory/mcp"
	@echo "  https://$(NGROK_URL)/skills/mcp"
	@echo "  https://$(NGROK_URL)/event/mcp"
	@echo
	ngrok http --url=$(NGROK_URL) $(PORT)

## clean: remove build and coverage artifacts
clean:
	rm -rf bin coverage.out
	@echo "cleaned"

.PHONY: help run build test testv cover fmt vet lint lintfix tidy migrate pgdev check health tunnel clean
