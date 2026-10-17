# Personal API keys

Nabu users can script their own activity logging (for example, backfilling
feeds and diaper changes from photos of a paper log) with a personal API key.

## Creating and revoking keys

Open **Settings → API Keys**, name the key, and select **Create key**. The full
key (`nabu_pat_<id>.<secret>`) is shown once; copy it before selecting
**Done**. The same card lists each key with its creation and last-used dates
and a **Revoke** button. Revocation takes effect on the next request.

- Up to 10 active keys per user.
- Keys never expire on their own. Changing or resetting your password turns
  every existing key off (shown as "Off (password changed)").
- Deleting your account deletes its keys.
- Keys can only be created, listed, or revoked from a signed-in browser
  session, never with another key.

Only a SHA-256 digest of each key is stored.

## What a key can do

A key acts as the user who created it, in that user's active household, with
exactly the permissions that user has in the app. It is limited to activity
logs and the lookups needed to write them:

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/logs` | Create a log |
| `PATCH` | `/api/logs/{id}` | Edit a log |
| `DELETE` | `/api/logs/{id}` | Delete a log |
| `GET` | `/api/logs/history`, `/api/logs/today`, `/api/logs/week`, `/api/logs/month` | Read logs |
| `GET` | `/api/chores` | Find activity IDs (`choreId`) |
| `GET` | `/api/household` | Find member IDs for "Done by" (`userId`); invites are omitted |
| `GET` | `/api/me` | Check which account a key belongs to |

Any other route returns `403`. An invalid, revoked, or invalidated key returns
`401`, even if the request also carries a session cookie. Key requests need no
CSRF token.

## Example: backfilling a diaper change

```sh
KEY=nabu_pat_...
BASE=https://nabu-app.com

# Find the activity and the household member who did it.
curl -s -H "Authorization: Bearer $KEY" $BASE/api/chores | jq '.chores[] | {id, name}'
curl -s -H "Authorization: Bearer $KEY" $BASE/api/household | jq '.members[] | {userId, displayName}'

# Log it at the right time. `date` and `hour` place it on the calendar;
# `completedAt` is the exact time. `userId` sets "Done by".
curl -s -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  $BASE/api/logs -d '{
    "choreId": 12,
    "userId": 34,
    "date": "2026-10-15",
    "hour": 3,
    "completedAt": "2026-10-15T03:10:00-04:00",
    "indicators": ["💛 pee"],
    "idempotencyKey": "backfill-2026-10-15-0310-change"
  }'
```

`indicators` must match the activity's `indicatorLabels` from `/api/chores`.
`idempotencyKey` (up to 64 characters) makes retries safe: sending the same
request again returns the original log instead of creating a duplicate.
