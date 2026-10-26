# Phone preview (Podman + Tailscale)

`make preview` builds the current checkout and runs it on your Mac as a Podman pod (app, PostgreSQL, Mailpit). It then publishes it to your tailnet with Tailscale Serve, so you can use it from your phone over HTTPS. Run it from any worktree to preview that branch.

## Requirements

- Podman with a running machine (`podman machine start`).
- Tailscale on the Mac and the phone, signed in to the same tailnet.
- Go, once: the first run generates a Web Push (VAPID) key pair with `go run ./cmd/vapid-keygen` and stores it in `~/.config/nabu/preview.env`, outside the repo.

## Use

```bash
make preview          # build this checkout, (re)start the pod, print the URLs
make preview-status   # containers and URLs
make preview-logs     # follow the app log
make preview-down     # stop the pod and remove the Tailscale Serve mappings
scripts/preview.sh reset --yes   # also delete the preview database
```

`make preview` prints:

- **App:** `https://<mac>.<tailnet>.ts.net:18443`. Open it on the phone. For push notifications on iOS, add it to the Home Screen first.
- **Mailpit:** `https://<mac>.<tailnet>.ts.net:18444`, for sign-in links and verification emails.
- **Local:** `http://127.0.0.1:18080` (Mailpit `http://127.0.0.1:18025`).

Both HTTPS URLs are tailnet only, never the public internet; the pod's ports bind to `127.0.0.1`. Rerunning `make preview` rebuilds and restarts the app but keeps the data in the `nabu-preview-pgdata` volume. Each build gets a new version string, so the phone never runs stale JavaScript.

To point an iOS build at the preview, add `-nabuBaseURL https://<mac>.<tailnet>.ts.net:18443` under the Xcode scheme's Run → Arguments.

## Notes

- The preview runs with `SERVER_SECURE=false`. With it on, the app sends HSTS for the whole tailnet hostname, which would stop the phone from reaching any plain-HTTP service on that Mac. Tailscale Serve already provides HTTPS.
- Change the ports or names with `NABU_PREVIEW_PORT`, `NABU_PREVIEW_MAIL_PORT`, `NABU_PREVIEW_HTTPS_PORT`, `NABU_PREVIEW_MAIL_HTTPS_PORT` and `NABU_PREVIEW_NAME`. Set `NABU_PREVIEW_TAILSCALE=0` to skip Tailscale.
- The preview doesn't use the fixed ports of `make local` (8080, 5432, 8025, 1025), so both can run at once.
