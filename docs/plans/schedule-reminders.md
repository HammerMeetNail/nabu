# Schedule Reminder Notifications

## Goal
Push notifications when a scheduled chore's time arrives, with per-member per-chore opt-in, configurable lead time, and quiet hours.

## Design

### Data model

**New table** `chore_reminder_prefs`:
```
user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE
chore_id       INTEGER NOT NULL REFERENCES chores(id) ON DELETE CASCADE
enabled        BOOLEAN NOT NULL DEFAULT false
lead_minutes   INTEGER NOT NULL DEFAULT 10
UNIQUE (user_id, chore_id)
```

**New notification type** `"schedule_reminder"` — added to the `availableTypes` returned by the notification preferences endpoint, alongside `chore_logged` and `household_joined`.

**Deduplication table** `schedule_reminders`:
```
schedule_id    INTEGER NOT NULL REFERENCES chore_schedules(id) ON DELETE CASCADE
user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE
reminded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
UNIQUE (schedule_id, user_id)
```

### Scheduler (server-side)

Background goroutine running every 30 seconds:
1. Query active schedules where the reminder window overlaps the current time:
   - Compute `remind_at = specific_time - lead_minutes` for each opted-in user
   - Filter for recurring schedules that are active today
2. For each match where `schedule_reminders` doesn't already have a row for this schedule+user:
   - Send push notification to the user
   - Insert into `schedule_reminders` (deduplication)
3. Purge old rows from `schedule_reminders` periodically (older than 7 days)

### Per-chore opt-in

- **Global default**: in notification settings, set default lead time (5/10/15/30/60 min)
- **Per-chore override**: in the chore edit sheet, toggle "Remind me" + lead time dropdown
- **Opt-in for all**: in settings, a "Remind me for all scheduled chores" toggle that bulk-enables all chores

### Quiet hours

Respect existing `quiet_hours_start` / `quiet_hours_end` from `reminder_preferences`. No push during quiet hours — defer to end of quiet period.

### API

```
GET  /api/chore-reminder-prefs          → list user's prefs per chore
PATCH /api/chore-reminder-prefs/:choreId → update enabled / lead_minutes
```

### Frontend

- **Settings → Notifications**: new `schedule_reminder` type toggle (master kill switch), default lead time selector
- **Chore edit sheet** (manage tab): "Remind me" toggle + lead time dropdown. Only visible if global `schedule_reminder` type is enabled
- **Push only** (no in-app notification for schedule reminders — that would be noise)

### Push payload

```json
{
  "title": "🔔 Feed Baby",
  "body": "Due at 2:00 PM"
}
```

### Assignment

Only the user assigned to the schedule slot (`assignedToUserId`) gets the reminder. If unassigned, only users who explicitly opted into that chore get it.

## Status
- [ ] Planning
- [ ] Migration: chore_reminder_prefs + schedule_reminders tables
- [ ] Backend: models, store, service, scheduler, handler
- [ ] Frontend: settings toggles, chore edit sheet toggle + lead time
- [ ] E2E tests
- [ ] iOS parity
