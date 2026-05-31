You are setting up a production CI/CD pipeline for the choresy application (this
repository) that mirrors the pattern used by yearofbingo. The end result is:

- Every version tag (`v*`) pushed to GitHub triggers a full test → build →
  push to Quay.io → scan → sign → deploy pipeline
- The container image is hosted on Quay.io
- Production runs on a shared Hetzner server alongside yearofbingo, available
  at `https://nabu-app.com`
- Deployment is fully automated: push a tag, get a deploy

**Assumption**: choresy has already been manually deployed to the server using
the local-build method. This guide migrates it to a proper CI/CD workflow. If
choresy has not yet been deployed manually, complete the manual deploy first,
then return here.

---

## Phase 0 — Understand this repository

Before creating any files, read the repository thoroughly to answer:

1. **Container port**: What port does the app listen on inside the container?
   Check `Containerfile`, `EXPOSE`, `SERVER_PORT`, `PORT`, or the app's
   entrypoint.
2. **Backing services**: Does the app require postgres, redis, or other services?
3. **Test suites present**: Does the repo have Go tests (`*_test.go`), JavaScript
   tests, and/or Playwright E2E tests? What commands run them?
4. **Required environment variables**: List every env var the app needs at
   runtime. Check `.env.example`, README, config files, and the main entrypoint.
5. **Go version**: Check `go.mod`.
6. **Module path**: The Go module name from `go.mod` (needed for nothing here,
   just for context).

Record all answers — they drive every decision below.

---

## Phase 1 — Create a Quay.io repository and robot account

Do this manually in the Quay.io web UI (https://quay.io):

1. Create a new **public** repository named `choresy/choresy`
   (or `<your-org>/choresy` — note the name you choose, it becomes `IMAGE_NAME`
   in the workflow).
2. Go to **Account Settings → Robot Accounts → Create Robot Account**.
   Name it something like `choresy_ci`.
3. Grant the robot **Write** permission on the `choresy/choresy` repository.
4. Copy the robot account's **username** and **password/token** — you will add
   these as GitHub secrets (`QUAY_USERNAME`, `QUAY_PASSWORD`) in Phase 3.

---

## Phase 2 — Create `compose.server.yaml`

This file is the production compose template. The CI/CD workflow reads it,
substitutes the image digest, and SCPs it to `/opt/choresy/compose.yaml` on the
server.

Create `compose.server.yaml` at the repository root. Use the template below,
adapting it to what you found in Phase 0 (remove services choresy does not need,
add env vars choresy does need, fix the container port):

```yaml
# Production compose file — deployed to /opt/choresy/compose.yaml by CI/CD
services:
  app:
    image: quay.io/nabu/nabu:latest   # CI substitutes digest at deploy time
    ports:
      - "8081:PORT"                          # replace PORT with container port from Phase 0
    environment:
      - SERVER_HOST=0.0.0.0
      - SERVER_PORT=PORT                     # match container port above
      - APP_ENV=production
      - APP_BASE_URL=https://nabu-app.com
      # Add all required env vars. Reference secrets from .env with ${VAR} syntax:
      # - DB_HOST=postgres
      # - DB_PORT=5432
      # - DB_USER=choresy
      # - DB_PASSWORD=${DB_PASSWORD}
      # - DB_NAME=choresy
      # - DB_SSLMODE=disable
      # - REDIS_HOST=redis
      # - REDIS_PORT=6379
      # - REDIS_PASSWORD=${REDIS_PASSWORD}
    depends_on:
      # Uncomment as needed:
      # postgres:
      #   condition: service_healthy
      # redis:
      #   condition: service_healthy
    restart: unless-stopped

  # Uncomment if choresy needs postgres:
  # postgres:
  #   image: docker.io/library/postgres:16-alpine
  #   environment:
  #     - POSTGRES_USER=choresy
  #     - POSTGRES_PASSWORD=${DB_PASSWORD}
  #     - POSTGRES_DB=choresy
  #   volumes:
  #     - /mnt/data/choresy/postgres:/var/lib/postgresql/data
  #   healthcheck:
  #     test: ["CMD-SHELL", "pg_isready -U choresy -d choresy"]
  #     interval: 5s
  #     timeout: 5s
  #     retries: 5
  #   restart: unless-stopped

  # Uncomment if choresy needs redis:
  # redis:
  #   image: docker.io/library/redis:7-alpine
  #   command: redis-server --requirepass ${REDIS_PASSWORD}
  #   volumes:
  #     - /mnt/data/choresy/redis:/data
  #   healthcheck:
  #     test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
  #     interval: 5s
  #     timeout: 5s
  #     retries: 5
  #   restart: unless-stopped
```

---

## Phase 3 — Set up GitHub repository secrets

In the choresy GitHub repository go to **Settings → Secrets and variables →
Actions** and create the following secrets. Values marked `[SAME AS YEAROFBINGO]`
can be copied from the yearofbingo repo's secrets — they reference the same
server and same external accounts.

| Secret name | Where to get the value |
|-------------|------------------------|
| `SSH_PRIVATE_KEY` | The private key for the `deploy` user on the server. This is the same key used by yearofbingo. Get the private key content from `~/.ssh/hetzner_yearofbingo_ci` on your local machine (the full PEM content including `-----BEGIN...` and `-----END...` lines). `[SAME AS YEAROFBINGO]` |
| `QUAY_USERNAME` | Robot account username from Phase 1 (format: `choresy+choresy_ci`) |
| `QUAY_PASSWORD` | Robot account token from Phase 1 |
| `CODECOV_TOKEN` | From https://codecov.io — add the choresy repo and copy its upload token (optional; remove the Codecov steps from the workflow if not using it) |
| `DB_PASSWORD` | Strong random password for choresy's postgres (if used). Generate with `openssl rand -base64 32`. Must match what is already in `/opt/choresy/.env` on the server. |
| `REDIS_PASSWORD` | Strong random password for choresy's redis (if used). Must match server. |
| *(add any other app-specific secrets choresy needs)* | From Phase 0 env var list |

---

## Phase 4 — Create the GitHub Actions workflow

Create `.github/workflows/ci.yaml`. Base it on yearofbingo's workflow but adapt
it to choresy's actual test suites and env vars (found in Phase 0).

The key structural differences from yearofbingo:
- `IMAGE_NAME` is `choresy/choresy` (or your Quay.io org/repo name)
- The deploy job writes to `/opt/choresy/` and uses `compose.server.yaml`
- SSH hostname is still `ssh.yearofbingo.com` (same server)
- Remove test jobs that don't apply (e.g. `test-js` if no JS tests, `e2e` if no E2E tests)
- Remove env vars and secrets from the deploy job that choresy doesn't need
- Adjust the `APP_BASE_URL` to `https://nabu-app.com`

Full workflow template — edit every `# ADAPT:` comment before committing:

```yaml
name: CI

on:
  push:
    branches: [main]
    tags:
      - 'v*'
  pull_request:
    branches: [main]
  workflow_dispatch:
    inputs:
      deploy_only:
        description: 'Skip build, deploy existing image'
        type: boolean
        default: false

env:
  GO_VERSION: "1.24"   # ADAPT: match go.mod
  REGISTRY: quay.io
  IMAGE_NAME: choresy/choresy   # ADAPT: must match Quay.io repo from Phase 1

jobs:
  changes:
    name: Detect Changes
    runs-on: ubuntu-latest
    if: github.event_name != 'push' || startsWith(github.ref, 'refs/tags/v')
    outputs:
      code: ${{ startsWith(github.ref, 'refs/tags/v') || steps.filter.outputs.code }}
      tests: ${{ steps.filter.outputs.tests }}
    steps:
      - uses: actions/checkout@v6
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            code:
              - '**/*.go'
              - '!**/*_test.go'
              - 'go.mod'
              - 'go.sum'
              - 'web/**'
              - 'migrations/**'
              - 'Containerfile'
              - 'compose*.yaml'
              - '.github/workflows/**'
              - 'Makefile'
              - 'scripts/**'
            tests:
              - '**/*_test.go'

  secrets:
    name: Secret Scan
    runs-on: ubuntu-latest
    if: github.event_name != 'push' || startsWith(github.ref, 'refs/tags/v')
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0
      - uses: gitleaks/gitleaks-action@v2
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  lint:
    name: Lint
    runs-on: ubuntu-latest
    needs: [changes]
    if: needs.changes.outputs.code == 'true'
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: ${{ env.GO_VERSION }}
      - uses: golangci/golangci-lint-action@v9
        with:
          version: v2.6.2

  test-go:
    name: Go Tests
    runs-on: ubuntu-latest
    needs: [changes]
    if: needs.changes.outputs.code == 'true' || needs.changes.outputs.tests == 'true'
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: ${{ env.GO_VERSION }}
      - run: go mod download
      - run: go test -v -race -coverprofile=coverage.out ./...
      # ADAPT: remove the Codecov steps if not using Codecov
      - uses: codecov/codecov-action@v5
        with:
          files: coverage.out
          flags: go
          token: ${{ secrets.CODECOV_TOKEN }}

  # ADAPT: add a test-js job here if choresy has JavaScript tests (see yearofbingo ci.yaml for the pattern)
  # ADAPT: add an e2e job here if choresy has Playwright tests (see yearofbingo ci.yaml for the pattern)

  build-image:
    name: Build Image (${{ matrix.arch }})
    runs-on: ${{ matrix.runner }}
    needs: [changes, secrets, lint, test-go]
    # ADAPT: add test-js and/or e2e to the needs list and the if condition below
    #        if you added those jobs above
    if: >
      needs.changes.outputs.code == 'true' &&
      needs.secrets.result == 'success' &&
      needs.lint.result == 'success' &&
      needs.test-go.result == 'success' &&
      github.actor != 'dependabot[bot]'
    strategy:
      matrix:
        include:
          - arch: amd64
            runner: ubuntu-latest
          - arch: arm64
            runner: ubuntu-24.04-arm
    steps:
      - uses: actions/checkout@v6
      - uses: docker/setup-buildx-action@v3

      - name: Log in to Quay.io
        if: startsWith(github.ref, 'refs/tags/v')
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ secrets.QUAY_USERNAME }}
          password: ${{ secrets.QUAY_PASSWORD }}

      - name: Build image (no push, PR/main)
        if: "!startsWith(github.ref, 'refs/tags/v')"
        uses: docker/build-push-action@v6
        with:
          context: .
          file: ./Containerfile
          platforms: linux/${{ matrix.arch }}
          cache-from: type=gha,scope=${{ matrix.arch }}
          cache-to: type=gha,mode=max,scope=${{ matrix.arch }}

      - name: Build and push by digest (tags only)
        if: startsWith(github.ref, 'refs/tags/v')
        id: build
        uses: docker/build-push-action@v6
        with:
          context: .
          file: ./Containerfile
          platforms: linux/${{ matrix.arch }}
          outputs: type=image,name=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }},push-by-digest=true,name-canonical=true,push=true
          cache-from: type=gha,scope=${{ matrix.arch }}
          cache-to: type=gha,mode=max,scope=${{ matrix.arch }}

      - name: Export digest
        if: startsWith(github.ref, 'refs/tags/v')
        run: |
          mkdir -p /tmp/digests
          digest="${{ steps.build.outputs.digest }}"
          touch "/tmp/digests/${digest#sha256:}"

      - name: Upload digest
        if: startsWith(github.ref, 'refs/tags/v')
        uses: actions/upload-artifact@v6
        with:
          name: digests-${{ matrix.arch }}
          path: /tmp/digests/*
          if-no-files-found: error
          retention-days: 1

  scan-and-push:
    name: Scan, Sign & Push Multi-Arch Manifest
    runs-on: ubuntu-latest
    needs: [build-image]
    if: >
      github.event_name == 'push' &&
      startsWith(github.ref, 'refs/tags/v') &&
      github.actor != 'dependabot[bot]'
    outputs:
      digest: ${{ steps.digest.outputs.digest }}
      tag: ${{ steps.tag.outputs.tag }}
    permissions:
      contents: read
      packages: write
      id-token: write
    steps:
      - uses: actions/download-artifact@v7
        with:
          path: /tmp/digests
          pattern: digests-*
          merge-multiple: true

      - uses: docker/setup-buildx-action@v3

      - name: Log in to Quay.io
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ secrets.QUAY_USERNAME }}
          password: ${{ secrets.QUAY_PASSWORD }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=sha,prefix=
            type=raw,value=latest,enable={{is_default_branch}}
            type=semver,pattern={{version}}

      - name: Create and push manifest
        working-directory: /tmp/digests
        run: |
          docker buildx imagetools create $(jq -cr '.tags | map("-t " + .) | join(" ")' <<< "$DOCKER_METADATA_OUTPUT_JSON") \
            $(printf '${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@sha256:%s ' *)

      - name: Get manifest digest
        id: digest
        run: |
          DIGEST=$(docker buildx imagetools inspect ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest --format '{{json .Manifest.Digest}}' | tr -d '"')
          echo "digest=$DIGEST" >> $GITHUB_OUTPUT

      - name: Determine image tag
        id: tag
        run: |
          if [[ "${{ github.ref }}" == refs/tags/v* ]]; then
            TAG="${GITHUB_REF#refs/tags/v}"
          else
            TAG="latest"
          fi
          echo "tag=$TAG" >> $GITHUB_OUTPUT

      - name: Run Trivy vulnerability scanner
        uses: aquasecurity/trivy-action@0.33.1
        with:
          image-ref: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest
          format: "table"
          exit-code: "1"
          ignore-unfixed: true
          vuln-type: "os,library"
          severity: "CRITICAL,HIGH"

      - name: Install Cosign
        uses: sigstore/cosign-installer@v3

      - name: Sign image with Cosign (keyless)
        env:
          COSIGN_EXPERIMENTAL: "true"
        run: |
          cosign sign --yes ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@${{ steps.digest.outputs.digest }}

  release:
    name: Create GitHub Release
    runs-on: ubuntu-latest
    needs: [scan-and-push]
    if: startsWith(github.ref, 'refs/tags/v')
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v6
      - name: Get version
        id: version
        run: echo "version=${GITHUB_REF_NAME#v}" >> $GITHUB_OUTPUT
      - uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
          body: |
            ## Container Image

            ```bash
            podman pull quay.io/nabu/nabu:${{ steps.version.outputs.version }}
            ```

            **Available tags:**
            - `quay.io/nabu/nabu:${{ steps.version.outputs.version }}`
            - `quay.io/nabu/nabu:latest`

  deploy:
    name: Deploy to Production
    runs-on: ubuntu-latest
    needs: [scan-and-push]
    # ADAPT: add `e2e` to needs and the if condition below if you have E2E tests
    if: >
      (
        github.event_name == 'workflow_dispatch' &&
        needs.scan-and-push.result == 'success'
      ) ||
      (
        github.event_name == 'push' &&
        startsWith(github.ref, 'refs/tags/v') &&
        needs.scan-and-push.result == 'success'
      )
    steps:
      - uses: actions/checkout@v6

      - name: Install cloudflared
        run: |
          CLOUDFLARED_VERSION="2026.1.2"
          CLOUDFLARED_SHA256="e157c54e929cc289cbd53860453168c2fe3439eb55e2e965a56579252585d9c1"
          curl -fsSL "https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-amd64" -o cloudflared
          echo "${CLOUDFLARED_SHA256}  cloudflared" | sha256sum -c -
          chmod +x cloudflared
          sudo mv cloudflared /usr/local/bin/

      - name: Deploy to server
        env:
          SSH_PRIVATE_KEY: ${{ secrets.SSH_PRIVATE_KEY }}
          IMAGE_DIGEST: ${{ needs.scan-and-push.outputs.digest }}
          IMAGE_TAG: ${{ needs.scan-and-push.outputs.tag }}
          # ADAPT: list every secret choresy needs at runtime
          DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
          REDIS_PASSWORD: ${{ secrets.REDIS_PASSWORD }}
          # add more secrets here as needed
        run: |
          mkdir -p ~/.ssh
          echo "$SSH_PRIVATE_KEY" > ~/.ssh/id_ed25519
          chmod 600 ~/.ssh/id_ed25519

          cat >> ~/.ssh/config << 'SSHCONFIG'
          Host ssh.yearofbingo.com
            User deploy
            ProxyCommand cloudflared access ssh --hostname %h
            StrictHostKeyChecking accept-new
          SSHCONFIG

          # Substitute image digest into compose file
          if [ -n "${IMAGE_DIGEST}" ]; then
            sed "s|image: quay.io/nabu/nabu:latest|# tag: ${IMAGE_TAG}\n    image: quay.io/nabu/nabu@${IMAGE_DIGEST}|" \
              compose.server.yaml > compose.server.deploy.yaml
          else
            cp compose.server.yaml compose.server.deploy.yaml
          fi

          # Copy compose file to server
          scp compose.server.deploy.yaml ssh.yearofbingo.com:/opt/choresy/compose.yaml

          # Deploy
          ssh ssh.yearofbingo.com << EOF
            cd /opt/choresy

            # Write .env (ADAPT: add/remove vars to match choresy's secrets above)
            cat > .env << 'ENVFILE'
          DB_PASSWORD=${DB_PASSWORD}
          REDIS_PASSWORD=${REDIS_PASSWORD}
          ENVFILE
            chmod 600 .env

            # Pull new image and restart
            podman-compose pull
            podman-compose down || true
            podman-compose up -d
          EOF
```

---

## Phase 5 — Create `.github/dependabot.yml`

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 10

  - package-ecosystem: "docker"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 10

  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 10
```

---

## Phase 6 — Create `.github/workflows/codeql.yml` (optional but recommended)

```yaml
name: CodeQL

on:
  pull_request:
    branches: [main]
  push:
    tags:
      - 'v*'

jobs:
  analyze:
    name: Analyze (${{ matrix.language }})
    runs-on: ubuntu-latest
    permissions:
      actions: read
      contents: read
      security-events: write
    strategy:
      fail-fast: false
      matrix:
        language: ['go']   # ADAPT: add 'javascript' if choresy has JS
    steps:
      - uses: actions/checkout@v6
      - uses: github/codeql-action/init@v4
        with:
          languages: ${{ matrix.language }}
      - uses: github/codeql-action/autobuild@v4
      - uses: github/codeql-action/analyze@v4
```

---

## Phase 7 — Update the server to expect the Quay.io image

The server currently has `/opt/choresy/compose.yaml` referencing
`localhost/choresy_app:latest` (from the manual deploy). The CI/CD workflow
will overwrite this file on first deploy with the Quay.io image reference.
No server changes are needed — the deploy job handles it.

However, verify the server can pull from Quay.io (it should be able to for
public repositories without any credentials):

```bash
ssh -i ~/.ssh/hetzner_yearofbingo_ci \
    -o ProxyCommand="cloudflared access ssh --hostname ssh.yearofbingo.com" \
    deploy@ssh.yearofbingo.com \
    "podman pull quay.io/nabu/nabu:latest 2>&1 | tail -5"
```

If the repository is private, configure Quay.io credentials on the server:
```bash
podman login quay.io --username <robot-username> --password <robot-token>
```
(Credentials persist in `/home/deploy/.config/containers/auth.json`.)

---

## Phase 8 — Commit, push, and tag

```bash
git add compose.server.yaml .github/
git commit -m "ci: add GitHub Actions CI/CD pipeline"
git push origin main

# Create the first release tag to trigger the full pipeline
git tag v0.1.0
git push origin v0.1.0
```

Watch the Actions tab in GitHub. The pipeline runs in this order:
1. `changes`, `secrets` → in parallel
2. `lint`, `test-go` (and any other test jobs) → after `changes`
3. `build-image` (amd64 + arm64 in parallel) → after all tests pass
4. `scan-and-push` → after both arch builds complete
5. `release` and `deploy` → after `scan-and-push`

Total wall time is typically 8–15 minutes depending on build cache warmth.

---

## Phase 9 — Verify the deployment

```bash
# App is live at the correct URL
curl -sf https://nabu-app.com/health

# yearofbingo is still healthy (must not have been disrupted)
curl -sf https://yearofbingo.com/health
```

On the server, confirm both stacks are running:
```bash
ssh -i ~/.ssh/hetzner_yearofbingo_ci \
    -o ProxyCommand="cloudflared access ssh --hostname ssh.yearofbingo.com" \
    deploy@ssh.yearofbingo.com \
    "podman ps --format 'table {{.Names}}\t{{.Status}}' && sudo systemctl status choresy.service yearofbingo.service --no-pager"
```

---

## How to deploy going forward

Push a version tag:
```bash
git tag v1.2.3
git push origin v1.2.3
```

That's it. The full pipeline runs automatically and deploys on success.

To re-deploy the current image without a code change (e.g. to rotate a secret),
use the **workflow_dispatch** trigger in the GitHub Actions UI with
`deploy_only: true` — this skips the build and redeploys the existing image
with fresh secrets.

---

## Key facts

| Item | Value |
|------|-------|
| SSH hostname | `ssh.yearofbingo.com` (Cloudflare Tunnel) |
| SSH user | `deploy` |
| SSH key secret | `SSH_PRIVATE_KEY` (same key as yearofbingo) |
| Container registry | `quay.io` |
| Image name | `quay.io/nabu/nabu` (adjust to your org) |
| Server app dir | `/opt/choresy/` |
| Server host port | `8081` |
| Production URL | `https://nabu-app.com` |
| Deploy trigger | Push a `v*` tag to `main` |
| yearofbingo must stay healthy | Yes — verify at end of every deploy |
