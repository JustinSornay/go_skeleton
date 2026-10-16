# Local variables (DATABASE_URL, AUTH_MODE…) come from .env when present.
# Copy .env.example to .env to get a working local setup.
-include .env
export

GO                    ?= go
GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION   ?= v1.9.0
VERSION               ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS               := -s -w -X main.version=$(VERSION)
IMAGE                 ?= go-skeleton:$(VERSION)

.DEFAULT_GOAL := help

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2} /^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5)}' $(MAKEFILE_LIST)

##@ Local environment

.PHONY: up down clean logs
up: ## Start PostgreSQL (add PROFILES=redis for Redis)
	docker compose $(addprefix --profile ,$(PROFILES)) up -d --wait

down: ## Stop containers (data is kept)
	docker compose --profile redis --profile app down

clean: ## Stop containers and delete their data
	docker compose --profile redis --profile app down -v

logs: ## Follow container logs
	docker compose logs -f

##@ Run

.PHONY: run dev compose-app
run: ## Run the API locally (reads .env)
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/api

dev: up migrate run ## Start dependencies, migrate, run the API

compose-app: ## Build and run migrate + API in containers
	docker compose --profile app up -d --build --wait

##@ Database

.PHONY: migrate migrate-down migrate-status migration
migrate: ## Apply pending migrations
	$(GO) run ./cmd/migrate up

migrate-down: ## Roll back the latest migration
	$(GO) run ./cmd/migrate down

migrate-status: ## Show migration status
	$(GO) run ./cmd/migrate status

migration: ## Create a migration: make migration name=add_users
	@test -n "$(name)" || (echo "usage: make migration name=<snake_case_name>" && exit 1)
	$(GO) run ./cmd/migrate create $(name)

##@ Quality

.PHONY: fmt lint test test-race test-integration cover vuln check
fmt: ## Format code (gofmt + goimports)
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) fmt ./...

lint: ## Run golangci-lint
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

test: ## Run unit tests (no Docker needed)
	$(GO) test -count=1 ./...

test-race: ## Run unit tests with the race detector
	$(GO) test -race -count=1 ./...

test-integration: ## Run all tests incl. PostgreSQL ones (Docker or TEST_DATABASE_URL)
	$(GO) test -race -count=1 -tags integration ./...

cover: ## Run all tests and open the coverage report
	$(GO) test -count=1 -tags integration -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

vuln: ## Check dependencies for known vulnerabilities
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

check: lint test-integration vuln ## Everything CI runs

##@ Build

.PHONY: build docker-build tidy
build: ## Build all binaries into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...

docker-build: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

##@ Template

.PHONY: rename
rename: ## Rename the Go module: make rename MODULE=github.com/acme/my-api
	@test -n "$(MODULE)" || (echo "usage: make rename MODULE=<new/module/path>" && exit 1)
	./scripts/rename-module.sh $(MODULE)
