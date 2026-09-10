# Coordinating local development stacks

Read this when worktrees or agents share a development host. The default [compose.yaml](../compose.yaml) publishes app 8080, Postgres 5432, and Mailpit 8025/1025. A Compose project name isolates containers and networks, but does not change those host ports.

## One owner per stack

Inspect running containers and their Compose labels to identify the owning project/worktree before starting or stopping anything:

```bash
podman ps --format '{{.Names}}\t{{.Ports}}'
```

Coordinate with that owner. To temporarily free ports while retaining containers and data, run `podman compose stop` with the owning worktree's Compose file/project selection. Do not stop another task's stack merely because it occupies the default ports.

Use `make local` to rebuild/start an owned default stack after Go or embedded web changes. **`make down` runs `compose down -v`; `make local-fresh` calls it first. Both delete stack volumes/data.** Use them only when an intentional reset of a disposable stack is appropriate, not as a routine asset refresh or ownership handoff. Plain Compose `down` also removes containers; prefer `stop` when handing off preserved state.

## Two stacks at once

Both stacks need unique project names and nonconflicting **host** ports. Shell assignments such as `PORT=8081 make local` do not override the literal port/environment values in the checked-in Compose file.

For a second stack, prepare a complete task-specific Compose file in an external temporary directory, based on `compose.yaml`. Set `app.build.context` to the absolute task-worktree path (relative paths otherwise resolve beside the temporary file). Preserve the internal service names/ports and choose unused host ports, for example:

| Service | Default mapping | Second stack mapping |
|---|---|---|
| App | `8080:8080` | `8081:8080` |
| Postgres | `5432:5432` | `5433:5432` |
| Mailpit web | `8025:8025` | `8026:8025` |
| Mailpit SMTP | `1025:1025` | `1026:1025` |

Set the second app's `APP_BASE_URL` to `http://localhost:8081`. Its container `PORT` remains 8080, database connection stays on `postgres:5432`, and SMTP stays on `mailpit:1025`. A complete file avoids Compose overrides accidentally retaining the default ports as well as adding new ones. Keep temporary configuration out of commits.

After selecting the prepared file and a unique project name:

```bash
export NABU_COMPOSE_FILE="/absolute/path/to/temporary/compose.yaml"
export NABU_STACK_PROJECT="nabu-my-task"
podman compose -f "$NABU_COMPOSE_FILE" -p "$NABU_STACK_PROJECT" config
podman compose -f "$NABU_COMPOSE_FILE" -p "$NABU_STACK_PROJECT" up -d --build
sh scripts/wait-for-stack.sh localhost:8081
```

Use the same file/project flags for subsequent rebuilds, logs, and stop/cleanup commands. The Go build inside a Compose image build also needs coordination with host Go checks.

## Browser tests on an existing stack

`make e2e` runs [scripts/e2e.sh](../scripts/e2e.sh), which starts its own stack and checks localhost:8080; it does not honor Makefile's `COMPOSE` override. `BASE_URL` affects Playwright, not that startup/health check. To test an already-running alternate stack, invoke Playwright directly:

```bash
BASE_URL=http://localhost:8081 pnpm exec playwright test tests/e2e/home-when-picker.spec.js --project=chromium
```

Inspect the selected specs first: some still hard-code app 8080 or Mailpit 8025 (for example `magic-link.spec.js`). The full suite therefore needs the default-port stack unless those specs are configured for the alternate services. Do not claim isolation from `BASE_URL` alone. See [the E2E guide](../tests/e2e/AGENTS.md) for coverage, artifacts, and result verification.
