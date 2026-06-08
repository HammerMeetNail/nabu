GO ?= go
COMPOSE ?= podman compose

.PHONY: test test-go test-js fmt run e2e e2e-watch e2e-debug backup restore local local-fresh down seed lint coverage hooks check-parity

test: test-go test-js

test-go:
	$(GO) test ./...

test-js:
	node --test web/static/js/tests/runner.js

fmt:
	$(GO) fmt ./...

run:
	$(GO) run ./cmd/server

local:
	$(COMPOSE) up -d --build
	sh ./scripts/wait-for-stack.sh

local-fresh: down local

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
	sh ./scripts/backup.sh

restore:
	sh ./scripts/restore.sh $(DUMP)

seed:
	sh ./scripts/seed.sh

lint:
	mkdir -p .cache
	@if [ ! -f .cache/golangci-lint ]; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s v2.6.2; \
		mv bin/golangci-lint .cache/golangci-lint; \
		rm -rf bin; \
	fi
	.cache/golangci-lint run ./...

coverage:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

hooks:
	@echo "Installing git hooks..."
	@cp scripts/pre-push-hook.sh .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "  pre-push: parity check (skip with SKIP_PARITY=1)"

check-parity:
	bash scripts/check-parity.sh
