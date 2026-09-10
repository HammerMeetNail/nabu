# Working on Nabu

Nabu has a Go HTTP backend, a PWA built with plain JavaScript ES modules, and a native SwiftUI iOS client. The backend owns shared business rules; both clients are first-class.

## Start here

- Follow the user's task and active session instructions. These repository guidelines supply project context; they do not override tool permissions or require repeating an approval already given.
- Inspect `git status --short --branch` and `git worktree list` before changing files. Preserve unrelated changes, untracked files, running stacks, and other agents' work.
- Read the scoped instructions for the files you will touch. When starting at the repository root, open the relevant nested guide explicitly; do not assume it was loaded automatically.

| Working on | Read |
|---|---|
| Backend, server startup, migrations | [internal/AGENTS.md](internal/AGENTS.md), also for `cmd/` and `migrations/` changes |
| PWA, HTML/CSS, browser state | [web/AGENTS.md](web/AGENTS.md) |
| Native iOS | [ios/AGENTS.md](ios/AGENTS.md) |
| Playwright tests or browser validation | [tests/e2e/AGENTS.md](tests/e2e/AGENTS.md) |
| Shared client behavior or API contracts | [Parity workflow](.opencode/skills/client-parity/SKILL.md) and [matrix](docs/plans/client-parity.md) |
| Multiple local stacks | [Stack coordination](docs/dev-multistack.md) |
| Release or production verification | [Deploy runbook](docs/deploy-runbook.md) |

Use `rg` and targeted reads to trace the implementation and existing tests before editing. Check branches for existing work before reimplementing a feature. Plans describe intent; verify their paths, test inventories, and completion claims against the checkout.

## Ownership and delegation

The lead owns requirements, diagnosis, design decisions, integration, validation, and the final result. Continue authorized work through implementation and appropriate checks. Ask only when missing information materially affects scope, product behavior, or an action that lacks authorization; continue independent work while waiting.

Use the collaboration tools and named roles actually available in the session. Keep the user's configured model, effort, and Standard service speed; model configuration belongs in the user's agent settings, not this repository. Do not change it as part of project work.

| Role | Delegate |
|---|---|
| `explorer` | Bounded, read-only searches and execution-path mapping |
| `worker` | Routine implementation with clear acceptance criteria and assigned files |
| `worker-complex` | Causal tracing and implementation across components, especially ownership, cancellation, and asynchronous behavior |
| `reviewer` | Independent, read-only correctness and regression review |
| `reviewer-logic` | Independent review of concurrency, callbacks, cancellation, and test synchronization |
| `reviewer-astra` | Security, secrets, privilege boundaries, sensitive persistence/migrations, or large blast radius |
| `watcher` | Read-only monitoring of long local validation runs and enabled CI |
| `default` | Other bounded work, including authorized Git mechanics |

- Delegate independent work when it improves quality or removes routine work from the lead. Workers may edit within their assignment; explorers, reviewers, and watchers stay read-only.
- Start new agents with `fork_turns="none"` when supported. Every handoff includes the goal, worktree, constraints, relevant files/symbols, acceptance criteria, and required output. Assign disjoint files to parallel writers and reserve capacity for review or monitoring.
- Reuse agents for related follow-ups. Give ambiguous design decisions back to the lead. Use configured `worker-sol`/`reviewer-sol` fallbacks when appropriate; report unavailable roles and retain the work locally if no suitable role exists.
- Give one owner control of Go builds/tests, simulators, and each local stack. A watcher receives the commit or source fingerprint, process/run identifier, artifact paths, and acceptance conditions. It reports failures and terminal results; the lead decides fixes and reruns.
- Commit, push, and PR creation can be handled by a suitable available role or the lead within the authorized task. There is no dependency on a `git-ops` role or a tool named `Task`.

## Worktrees and Git

All edits and validation, including documentation changes, happen in a task worktree. Reuse the assigned task worktree when continuing work; do not create a new one for every turn.

For new work, from the main checkout:

```bash
git status --short --branch
git worktree list
git fetch origin
git worktree add worktrees/<name> -b <name> origin/main
```

Creating from freshly fetched `origin/main` avoids switching or pulling another active checkout. If fetching is unavailable, state that limitation and the base used. Never reset, stash, clean, or overwrite unrelated work to make setup succeed.

Review the final diff and stage only intended files. Keep credentials, troubleshooting images, logs, coverage output, and build artifacts out of commits. Leave an unfinished worktree available for review; remove only a task-owned, clean worktree after its changes are safely integrated. A development request does not itself request a release.

## Commands and validation

Run commands from the task worktree root unless a scoped guide says otherwise. [Makefile](Makefile), [go.mod](go.mod), [package.json](package.json), [Containerfile](Containerfile), and [CI](.github/workflows/ci.yaml) are the sources for current targets and toolchain versions. The Go module minimum, CI toolchain, and container toolchain may differ; check all three for toolchain work. Install JS dependencies with `pnpm install --frozen-lockfile` and browser binaries with `pnpm exec playwright install chromium` when needed. iOS requires macOS and Xcode.

| Task | Command / behavior |
|---|---|
| Go build / vet | `go build ./...`, then `go vet ./...` |
| Go tests | `make test-go` (300-second package timeout) |
| JS tests | `make test-js` (Node test runner with jsdom) |
| Go + JS tests | `make test` (does not include E2E or iOS) |
| Go lint | `make lint` (golangci-lint, also used by CI; bootstraps locally) |
| Go format | `make fmt` |
| Go race and coverage | `make coverage` (writes `coverage.out`; keep it out of commits) |
| Parity matrix lint | `make check-parity` |
| In-memory backend | `make run` with `DATABASE_URL` unset/empty in development |
| Build/start local stack | `make local` (Podman Compose by default) |
| Reset local stack | `make local-fresh` (runs `down -v`: deletes stack volumes/data) |
| Full browser suite | `make e2e` (starts a stack; see the E2E guide) |

Choose checks by the changed behavior, then complete the applicable gate:

- **Documentation only:** check the diff, links, paths, and documented commands against their sources. Run `make check-parity` when editing parity guidance. No application tests or stack rebuild are needed solely for prose. CI path filters may still select jobs for docs under `web/` or `ios/`; report their actual status if a PR is opened.
- **Runtime code:** before committing/pushing, run `go build ./...`, `go vet ./...`, `make test-go`, `make test-js`, and `make lint`. Run `make check-parity` for client/API changes. Add the affected platform checks below; required hooks and CI still apply.
- **Features and fixes:** add a regression at the affected layer. PWA-visible behavior needs Playwright coverage; shared API behavior needs Go contract/authorization tests plus affected client coverage; native behavior needs appropriate XCTest/XCUITest coverage. Test cancel/error and persistence paths where the feature has them. Pure documentation and mechanical changes do not need invented user-flow tests.
- **Stores, scheduler, or concurrency:** also run `go test -race -timeout 600s ./...`. Obtain independent behavior and test-synchronization review before an expensive full gate for concurrency/callback changes.
- **iOS:** use the build/test sequence in the iOS guide, including relevant native flow and snapshot checks. On hosts without Xcode, complete portable checks and report native checks as unrun; do not claim CI will cover a backend-only change automatically.

Run focused regressions first. Once the integrated changes pass required checks, repeat broader runs only for a relevant change, failure, or unresolved concern. Run only one Go build/vet/lint/test invocation at a time across agents; overlapping runs contend with bcrypt-heavy tests and can create false timeouts. Reproduce a timed-out package in isolation before diagnosing a hang.

Use an external temporary directory for validation logs and transient artifacts. Check command exit status and authoritative summaries, including skips, crashes, and expected failures; passing test-case lines alone are insufficient. Never report unavailable, skipped, or pending checks as passing.

The [pre-push hook](scripts/pre-push-hook.sh) runs build, vet, Go tests, and JS tests when Node and `node_modules` exist. Its older client-path check is separate from the CI matrix gate. After a documented parity decision, `SKIP_PARITY=1` bypasses only that legacy parity section; it still runs the preceding checks. Do not bypass build/test failures. `make hooks` currently assumes `.git` is a directory and fails in linked worktrees; if installing a hook is part of the task, resolve its path with `git rev-parse --git-path hooks/pre-push` and preserve any existing hook.

## Shared security and client contracts

- Every resource read and mutation must verify household ownership, including services, foreign keys supplied by clients, exports, and operations involving another member. Preserve private-resource and role restrictions as well as household isolation.
- Validate all input server-side: lengths, formats, control characters, array size and item length, and relationships such as indicator defaults being a subset of labels. Colors must match `^#[0-9A-Fa-f]{6}$`.
- Treat user/database metadata as untrusted. Use the existing escaping helpers at HTML/SVG/attribute sinks and plain text for native rendering. Do not introduce a second escaping helper.
- Never log secrets, tokens, capability URLs, or raw PII. Use existing hashed-IP/email and endpoint-host logging helpers. Do not put production credentials into agent instructions or test fixtures.
- Security fixes need regression tests at the vulnerable layer: exact rendering sink for escaping; cross-household negative tests for authorization. Preserve the detailed auth, HTTP, and crypto rules in the backend guide.
- Shared log semantics: request `hour` becomes stored `slotHour`; null means Anytime, an integer means that local-hour row. `completedAt` is a timestamp with separate meaning. Preserve date, time, timezone, and selected-minute behavior across clients.

Evaluate every shared behavior, validation, security, or API change for **both** clients. Match behavior while using native presentation on iOS. Update [docs/plans/client-parity.md](docs/plans/client-parity.md) for affected rows and known differences; do not mark unverified work Done or expand a task to unrelated pending features.

CI's parity filter includes `web/static/js/**`, `web/static/css/**`, `web/templates/**`, `ios/**`, and `internal/handlers/**`. Such PRs must touch the matrix or contain `no-parity-update: <reason>` when no matrix change is appropriate (for example, agent documentation under `ios/`). Legacy “PWA-only change” wording alone does not satisfy this check. The [parity workflow](.opencode/skills/client-parity/SKILL.md) can be read directly; it does not require an installed slash command.

## Local stack and releases

Default host ports are app 8080, Postgres 5432, and Mailpit 8025/1025. Inspect containers and coordinate ownership before starting a stack. Rebuild embedded Go/HTML/JS/CSS assets with `make local` on the owned stack; do not erase data just to refresh a binary. Both `make down` and `make local-fresh` delete volumes. Use those only for an intentionally disposable stack; use Compose `stop` to free ports while retaining containers/data. See [stack coordination](docs/dev-multistack.md) for custom ports and runner limitations.

Local seed credentials are `test@nabu.local` / `correct horse battery`; run `make seed` after the local stack is healthy.

Deploy only after the change is merged into `origin/main`. For an authorized release, fetch, fast-forward a clean `main`, verify the intended commit, then tag/push from `main` using the [deploy runbook](docs/deploy-runbook.md). Do not tag a feature branch or use manual workflow dispatch to bypass this rule. Monitor the exact release run to completion with a watcher and verify production; a green deploy job alone does not establish that every test lane passed.

Finish with what changed, why, the validation results and material gaps, and the branch/worktree or PR location. Keep guidance small and scoped; update it when commands or invariants change. [OpenAI's instruction-file guide](https://learn.chatgpt.com/docs/agent-configuration/agents-md) explains discovery and loading limits.
