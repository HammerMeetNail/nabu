# Coordinating Multiple Local Dev Stacks

> Extracted from `AGENTS.md` for progressive disclosure — read this only when you actually need more than one local stack at a time.

The compose file binds to fixed host ports (8080, 5432, 8025, 1025). Only one stack can run at a time. When multiple agents are working across worktrees, they will conflict.

**Simple rule**: only one agent runs `make local` at a time. Before starting a new stack, run `make down` in the worktree that has it to free the ports. If you're not sure which worktree owns the running containers, use:

```bash
podman ps --format '{{.Names}}' | grep nabu
```

If containers are running, `cd` to the worktree that started them and run `make down`.

**If you genuinely need two stacks simultaneously** (e.g. comparing behaviour between branches):

```bash
# First stack (default ports): clean up any existing stack, then start fresh
make down
COMPOSE_PROJECT_NAME=nabu-A make local

# Second stack: use a unique project name and override ports
COMPOSE_PROJECT_NAME=nabu-B \
  PORT=8081 APP_BASE_URL=http://localhost:8081 \
  make -e local
```

Then edit `compose.yaml` temporarily in the second worktree to change the host-side port mappings (app `8081:8080`, Postgres `5433:5432`, Mailpit `8026:8025`), or pass them as `COMPOSE_FILE` overrides. When done, bring both down:

```bash
COMPOSE_PROJECT_NAME=nabu-A make down
COMPOSE_PROJECT_NAME=nabu-B make down
```

E2E tests respect the `BASE_URL` environment variable (defaults to `http://localhost:8080`), so you can target a non-default app port by setting `BASE_URL=http://localhost:8081 make e2e`.
