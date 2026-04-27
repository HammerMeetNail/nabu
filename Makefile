GO ?= go
COMPOSE ?= podman compose

.PHONY: test test-go test-js fmt run e2e backup restore local local-fresh down seed lint coverage

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
