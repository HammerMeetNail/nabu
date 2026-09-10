# Deploy and CI verification runbook

> Extracted from `AGENTS.md` for progressive disclosure — read this when cutting a release tag, watching the deploy pipeline, or verifying production.

## Deploy trigger

Push a `v*` tag on `main` (e.g. `git tag v0.1.7 && git push origin v0.1.7`). CI builds, tests, scans/signs the image, deploys via SSH + Cloudflare Tunnel, and creates a GitHub release. The deploy job verifies the tagged commit is reachable from `origin/main` before proceeding — **never tag on a branch.**

Production URL: `https://nabu-app.com`. The health/cache checks below need no login. For authenticated checks, use production test credentials provided in the authorized session or secret storage; keep credentials out of repository files and logs.

## 1. Watch the CI run

After an authorized `v*` tag push, monitor the pipeline to completion and verify production. Delegate monitoring to an available `watcher` with the repository, exact tag/commit, run ID, artifact locations, and acceptance conditions. The lead owns corrections and reruns; the watcher reports failures and terminal results. If no suitable role is available, the lead completes monitoring locally.

```bash
# Find the run ID and verify its headSha matches the intended tag commit
gh run list --workflow ci.yaml --branch <tag> --limit 5 \
  --json databaseId,headSha,headBranch,event,status,conclusion,url

# Stream logs until the run completes (blocks until done)
gh run watch <run-id> --exit-status

# If a job fails, check which step failed
gh run view <run-id> --json jobs \
  --jq '.jobs[] | {name: .name, conclusion: .conclusion, steps: [.steps[] | select(.conclusion == "failure") | .name]}'

# Re-run only failed jobs (for transient infra errors)
gh run rerun <run-id> --failed
```

Do not treat a green deploy job as an overall pass. Inspect every required job, including iOS, and distinguish expected path-filter skips from missing validation. CI skips the main branch-push validation jobs; ordinary feature branch pushes do not trigger this workflow. Follow the PR or tag run for the exact commit rather than waiting for a nonexistent branch run.

## 2. Diagnose failures before rerunning

- Classify the failed step and evidence: setup, runner, network, or simulator failures may be transient; compiler errors, assertions, and reproducible application failures need a fix. A job name alone is not enough to classify a failure.
- For a supported transient diagnosis, the lead can rerun the failed jobs and have the watcher verify the new attempt. If the same failure recurs, investigate it instead of repeatedly rerunning unchanged work.
- For a code failure, fix it in a worktree, validate it, and merge through `main` before cutting a new release tag. Never move an existing release tag or tag an unmerged fix branch. If a required job fails after deployment already succeeded, report the release as failing validation and assess the production impact.

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

# Confirm correct version in the app shell — anonymous GET / now serves the
# marketing page, so use a SPA route like /login for the shell
curl -s https://nabu-app.com/login | grep 'app.js'
# Expected: src="/static/js/app.js?v=0.1.X"

# Confirm the anonymous root serves the server-rendered marketing page with a
# canonical URL pointing at the root
curl -s https://nabu-app.com/ | grep 'rel="canonical"'
# Expected: <link rel="canonical" href="https://nabu-app.com/">

# HTML cache headers must also be no-store (Cloudflare reports DYNAMIC —
# its passthrough marker for uncacheable responses — rather than BYPASS)
curl -sI https://nabu-app.com/ | grep -i cache
# Expected: cache-control: no-store
#           cf-cache-status: DYNAMIC
```

### Verify per-IP rate limiting (after the trusted-proxy deploy, once only)

With `TRUSTED_PROXY_CIDRS` set, the auth limiter must key on real client IPs — not one shared tunnel bucket:

```bash
# 8 rapid login attempts from one machine: expect 401 x5, then 429s
for i in $(seq 1 8); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -X POST https://nabu-app.com/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"email":"nobody@example.com","password":"wrongpass"}'
done
# Expected: 401 401 401 401 401 429 429 429 (with Retry-After on the 429s)

# From a second client on a different network (e.g. phone on LTE), a login
# attempt must still return 401 while the first client is limited — that
# proves per-IP bucketing rather than a sitewide bucket.
```

Also spot-check request logs: the hashed `client` attribute should now vary between visitors instead of being one tunnel IP for everyone.

### Troubleshooting

- If `cf-cache-status` is `HIT` or `MISS` (not `BYPASS` for JS / `DYNAMIC` for HTML), the `no-store` header is not reaching Cloudflare — investigate `internal/app/server.go` and the CI build logs.
- If imports still show the old version number, the binary was not rebuilt with the new tag — check that `internal/version/version.go` is populated at build time via `-ldflags`.

## Known CI limitations

### iOS tests on release tags

The `changes` job forces the iOS lane on for every `v*` tag push, including
server/web-only releases. However, `build-image.needs` lists the Go/JS/E2E
checks but omits `ios`; the downstream image publication and deployment
can succeed while iOS is running or failing. It is a required result to
inspect, not an enforced deployment dependency today. Verify the full run
before reporting release success. Changing that job graph is separate CI
work; do not assume documentation makes it a gate.

The native job runs `NabuTests`, not XCUITest flows. Snapshot suites can skip
when the runner's simulator does not match their recorded OS major. Inspect
those skips and run required native UI/visual coverage on a suitable Mac
when the release changes those behaviors. Backend-only PRs do not select
the iOS lane unless iOS files also change.
