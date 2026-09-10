# Playwright E2E instructions

Read the [root guide](../../AGENTS.md) and [PWA guide](../../web/AGENTS.md) for application changes. Tests use `@playwright/test`; [playwright.config.js](../../playwright.config.js) defines the Chromium project, mobile viewport, timeouts, workers, and `BASE_URL`.

## Coverage and isolation

- For a bug fix, add a test that fails on the old behavior and passes with the fix. New features cover the happy path, applicable cancel/error paths, and persistence across reload when persistence is part of the feature.
- Name specs `<area>-<feature>.spec.js` and follow adjacent conventions. Use a unique email/household per test to avoid contamination across parallel workers.
- Reuse or adapt existing `uniqueEmail()`, `setupWithChores(page)`, and `longPress(page, locator)` helpers. A long press commonly uses 650 ms to cross the 500 ms threshold; timed waiting is appropriate for the gesture itself.
- Prefer locator assertions and observable completion (`toBeVisible`, `toHaveCount`, relevant responses/state) over fixed sleeps. Register event/response waits before triggering the action. Do not mask race conditions with retries or longer arbitrary delays.
- Use `page.request` for setup and targeted API assertions when the UI operation is not what the test exercises. Preserve real UI coverage for the behavior under test; a successful setup request does not prove the interaction worked.
- Test security at the exact sink/resource boundary. Authorization tests need a second household and negative assertions. Match email messages by recipient and subject, not the first Mailpit message from a shared inbox.

## Running against the right server

Install dependencies with `pnpm install --frozen-lockfile` and `pnpm exec playwright install chromium`. Rebuild the owned app after Go or embedded web asset changes. One owner coordinates the stack; never take down another worktree's containers.

For the default disposable development stack, `make local` starts/rebuilds it and `make e2e` runs the full suite. `make local-fresh` is a data reset, not a prerequisite for every test run. Default ports are app 8080 and Mailpit 8025/1025; Compose raises test rate limits.

**Runner limitations:** [scripts/e2e.sh](../../scripts/e2e.sh), called by `make e2e`, always starts a server/stack. Locally it calls Podman/Docker Compose directly rather than honoring Makefile's `COMPOSE` override. Its health check defaults to localhost:8080 even if `BASE_URL` changes. In CI it starts an in-memory Go server and expects Mailpit separately. Do not use `make e2e` merely to target an already-running custom-port server.

For a focused test on an already rebuilt, healthy owned stack:

```bash
pnpm exec playwright test tests/e2e/home-when-picker.spec.js --project=chromium
```

For an isolated stack on another port, verify its health first, then invoke Playwright directly:

```bash
curl --fail --silent --show-error http://localhost:8081/health
BASE_URL=http://localhost:8081 pnpm exec playwright test tests/e2e/home-when-picker.spec.js --project=chromium
```

Some existing specs (for example [magic-link.spec.js](magic-link.spec.js)) hard-code app/Mailpit URLs. Inspect selected specs before running on alternate ports; `BASE_URL` does not override literal URLs. Use the default ports for the full suite unless all participating specs are configured for the isolated stack. See [stack coordination](../../docs/dev-multistack.md).

Keep raw logs, screenshots, traces, and reports in an external artifact directory. For Playwright artifacts, pass `--output /absolute/temporary/path` and configure reporter output there when using a file reporter. Existing tracked snapshot baselines are source assets, not disposable troubleshooting output.

## Completion

Run a focused regression first, then the full suite for PWA behavior changes. Verify process exit status and the final passed/failed/skipped/flaky summary. If a failure looks like shared server state, stale embedded assets, port collision, or missing Mailpit, diagnose that evidence before changing product behavior.

Coordinate any Go checks from the root guide sequentially with builds. Send long-run monitoring to `watcher` when available; include the source fingerprint, exact command/session, artifact locations, and expected coverage. Report unavailable browser/stack prerequisites and unrun tests accurately.
