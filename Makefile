GO ?= go
COMPOSE ?= podman compose
GOLANGCI_VERSION := 2.15.0

.PHONY: test test-go test-js perf-stats fmt run e2e e2e-watch e2e-debug backup restore local local-fresh down seed lint coverage hooks check-parity deploy preview preview-down preview-status preview-logs

test: test-go test-js

test-go:
	$(GO) test -timeout 300s ./...

test-js:
	node --test web/static/js/tests/runner.js tests/ops/*.test.js

# Opt-in workload; use an already-running local PostgreSQL server.
perf-stats: export TEST_DATABASE_URL ?= postgres://nabu:nabu@localhost:5432/nabu?sslmode=disable
perf-stats: export NABU_PERF_REPORT ?= /tmp/nabu-stats-workload.json
perf-stats:
	@test -n "$$TEST_DATABASE_URL" || { echo "TEST_DATABASE_URL must not be empty" >&2; exit 1; }
	NABU_PERF=1 NABU_PERF_ENFORCE=1 $(GO) test -timeout 240s ./internal/stats -run '^TestLocalStatsWorkload$$' -count=1 -v

fmt:
	$(GO) fmt ./...

run:
	$(GO) run ./cmd/server

local:
	$(COMPOSE) up -d --build
	sh ./scripts/wait-for-stack.sh

local-fresh: down local

# Phone-testable preview of this checkout: a Podman pod published to your
# tailnet with Tailscale Serve. See docs/local-preview.md.
preview:
	bash ./scripts/preview.sh up

preview-down:
	bash ./scripts/preview.sh down

preview-status:
	bash ./scripts/preview.sh status

preview-logs:
	bash ./scripts/preview.sh logs

down:
	$(COMPOSE) down -v

e2e:
	./scripts/e2e.sh

e2e-watch: local
	@echo "Running E2E tests in watch mode (headed browser)..."
	@CHROMIUM_PATH="$$(find $(HOME)/.cache/ms-playwright -name chrome -type f -path '*/chrome-linux/*' 2>/dev/null | head -1)" \
	pnpm exec playwright test --project=chromium --headed --reporter=list

e2e-debug: local
	@echo "Running E2E tests in debug mode (headed, paused on each step)..."
	@CHROMIUM_PATH="$$(find $(HOME)/.cache/ms-playwright -name chrome -type f -path '*/chrome-linux/*' 2>/dev/null | head -1)" \
	pnpm exec playwright test --project=chromium --headed --debug --reporter=list

backup:
	bash ./scripts/backup.sh

restore:
	bash ./scripts/restore.sh $(RESTORE_ARGS)

seed:
	sh ./scripts/seed.sh

lint:
	mkdir -p .cache
	@if [ ! -f .cache/golangci-lint-$(GOLANGCI_VERSION) ]; then \
		set -e; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/v$(GOLANGCI_VERSION)/install.sh -o .cache/install-lint.sh; \
		sh .cache/install-lint.sh v$(GOLANGCI_VERSION); \
		mv bin/golangci-lint .cache/golangci-lint-$(GOLANGCI_VERSION); \
		rm -rf bin; \
	fi
	.cache/golangci-lint-$(GOLANGCI_VERSION) run ./...

coverage:
	$(GO) test -race -timeout 600s -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

hooks:
	@echo "Installing git hooks..."
	@hooks="$$(git rev-parse --git-path hooks)"; \
	mkdir -p "$$hooks"; \
	printf '#!/usr/bin/env bash\n# Installed by `make hooks`. Thin wrapper: runs the versioned\n# script from the working tree, so the installed hook can never drift.\nroot=$$(git rev-parse --show-toplevel) || exit 1\nexec bash "$$root/scripts/pre-push-hook.sh" "$$@"\n' > "$$hooks/pre-push"; \
	chmod +x "$$hooks/pre-push"; \
	echo "  pre-push -> scripts/pre-push-hook.sh (build, vet, go/js tests, parity matrix; skip with SKIP_PARITY=1 or SKIP_FAST=1)"

check-parity:
	bash scripts/check-parity.sh

deploy:
	bash scripts/deploy.sh

# Explicit image arguments keep drills independent of any live cluster.
recovery-drill:
	python3 scripts/recovery.py wal-drill --image "$(PG_IMAGE)" --app-image "$(APP_IMAGE)" --report "$(REPORT)"
