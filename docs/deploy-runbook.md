# Deploy & CI Babysitting Runbook

> Extracted from `AGENTS.md` for progressive disclosure — read this when cutting a release tag, watching the deploy pipeline, or verifying production.

## Deploy trigger

Push a `v*` tag on `main` (e.g. `git tag v0.1.7 && git push origin v0.1.7`). CI builds, tests, scans/signs the image, deploys via SSH + Cloudflare Tunnel, and creates a GitHub release. The deploy job verifies the tagged commit is reachable from `origin/main` before proceeding — **never tag on a branch.**

Production URL: `https://nabu-app.com`. Production test account: `verify@yearofbingo.com` / `test123456`.

## 1. Watch the CI run

After pushing a `v*` tag, monitor the pipeline to completion and verify production. Do not wait for the user to ask. (A cheaper subagent may be delegated to this.)

```bash
# Find the run ID for the tag
gh run list --limit 5

# Stream logs until the run completes (blocks until done)
gh run watch <run-id>

# If a job fails, check which step failed
gh run view <run-id> --json jobs \
  --jq '.jobs[] | {name: .name, conclusion: .conclusion, steps: [.steps[] | select(.conclusion == "failure") | .name]}'

# Re-run only failed jobs (for transient infra errors)
gh run rerun <run-id> --failed
```

## 2. Distinguish transient vs. real failures

- If **only** the checkout/setup step failed and all test jobs passed → transient GitHub Actions infra error → re-run with `gh run rerun <run-id> --failed`.
- If a test job (Go Tests, JS Tests, E2E, Lint, iOS Unit Tests) failed → real failure → read the full log, fix the code, commit, re-tag, and push a new `v*` tag.

## 3. Verify production after deploy

Once the `Deploy to Production` job goes green:

```bash
# Confirm the app is up
curl -sS -o /dev/null -w "%{http_code}\n" https://nabu-app.com/health   # expect 200

# Confirm versioned imports carry the new tag
curl -s https://nabu-app.com/static/js/calendar.js | grep "^import"
# Expected: import { ... } from "./utils.js?v=0.1.X";

# Confirm cache headers — must be no-store / BYPASS, NOT max-age / HIT
curl -sI https://nabu-app.com/static/js/app.js | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: BYPASS

# Confirm correct version in the index page
curl -s https://nabu-app.com/ | grep 'app.js'
# Expected: src="/static/js/app.js?v=0.1.X"
```

### Troubleshooting

- If `cf-cache-status` is `HIT` or `MISS` (not `BYPASS`), the `no-store` header is not reaching Cloudflare — investigate `internal/app/server.go` and the CI build logs.
- If imports still show the old version number, the binary was not rebuilt with the new tag — check that `internal/version/version.go` is populated at build time via `-ldflags`.
