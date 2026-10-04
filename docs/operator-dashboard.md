# Private operator dashboard

The platform report is available at `/operator` only when `OPERATOR_OWNER_USER_ID`
is set to the numeric ID of a verified user. The flag is empty by default. It
requires PostgreSQL. A household owner or admin does not gain platform access.
The dashboard shows current registered accounts, verification, recent personal
log activity, household activity, daily registrations, and household totals.
It does not display log notes or other chore content.

## Local validation

Use a disposable PostgreSQL stack. The operator E2E creates the first account
and expects it to have ID 1; it also creates a second account and test activity.
`make local-fresh` removes the local Compose volume, so only use it when that
data can be discarded.

```sh
OPERATOR_OWNER_USER_ID=1 make local-fresh
OPERATOR_E2E=1 MAILPIT_URL=http://localhost:8025 pnpm exec playwright test tests/e2e/operator-dashboard.spec.js
```

Open `http://localhost:8080/operator` after the test and sign in as
`operator-e2e@nabu.local` / `operator-local-password`. The test verifies the
account through Mailpit and sets a reusable password, so the report is ready
to inspect after the test. It also checks
that an anonymous user and another registered user cannot enter the page,
that a key cannot be created without a valid CSRF token, and that scoped and
revoked keys behave as expected. To save a browser image outside the repo, set
`OPERATOR_SCREENSHOT=/tmp/nabu-operator.png` on the Playwright command.

With an existing database, set `OPERATOR_OWNER_USER_ID` to the ID of the
intended verified owner and restart the app. The page requires that owner's
normal session. If the setting is empty, the page returns 404 and the API is
unregistered. Do not set an email or a household ID here.

## Metric definitions

- Registration and verification counts cover current users, including users
  without a household. The users table is paginated by user ID.
- Recent activity uses the time a log was created in Nabu, not a backdated
  completion time. Windows are trailing 7 and 30 days. Chart days are UTC.
- An active account is a distinct current user whose ID is attached to a
  recently created log. Older logs without an actor ID are excluded from that
  count. The dashboard reports how many recent logs carry an actor ID.
- Household activity counts all logs in households where a user is currently
  a member. Several members may see the same household activity. Household
  totals and last-log times cover each household regardless of author ID.
- Last session seen comes from the current session table. Session expiry or
  deletion may remove that history; it is not a lifetime login audit.
- Deleted users and households are absent from these current-state reports.

## API and keys

Owner sessions can call `GET /api/operator/v1/summary`, `/activity?days=30`
or `90`, `/users?after=0&limit=50`, and `/households?after=0&limit=50`.
Users and households return `nextCursor` when another page is available.
The owner session can manage keys at `GET`/`POST /api/operator/v1/keys` and
`DELETE /api/operator/v1/keys/{id}`. Key creation takes JSON with `name`,
`scope` (`summary` or `full`), and `days` (7, 30, or 90). The response shows
the bearer token once. Creation requires a session signed in within the last
10 minutes; sign out and in again when that window has passed.

For a script, send `Authorization: Bearer <token>`. A `summary` key can only
read summary and activity; a `full` key can also read email-bearing user and
household pages. Keys cannot manage keys or mutate app data. At most five
active keys are allowed. They can be revoked from the dashboard, expire on
schedule, and become invalid after the owner's auth version changes (for
example, password recovery). Only a SHA-256 digest is stored. Keep cleartext
keys in a private secret store; never put them in frontend code, URLs, logs, or
a public dashboard. The report responses use `Cache-Control: no-store`.

For production, provision the owner ID in the deployment environment and
restart after confirming the account's ID and verified status. The setting is
passed through by `compose.server.yaml` but has no production default. Restrict
the `/operator` and `/api/operator/` paths at the edge to the owner's identity
or trusted network as an additional boundary. The app still checks the owner
session or key on every report request. Avoid putting a service token in a
public browser; a separate private server can hold it and render its own view.
