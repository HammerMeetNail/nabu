---
name: client-parity
description: Ensure PWA and native iOS app stay in sync. Load this when making changes to frontend code, API endpoints, or any feature that affects the user experience. Required before finishing any work that touches web/static/js/, web/templates/, ios/, or internal/handlers/.
license: MIT
compatibility: opencode
metadata:
  audience: all
  workflow: check-before-commit
---

## What I do

Remind you to check both clients (PWA and iOS) when making changes.

## When to load

Load this skill whenever the user asks you to:
- Add a feature that has a UI component
- Fix a bug in the PWA or iOS app
- Change an API endpoint or its request/response shape
- Update validation logic or business rules

## Instructions

Before finalizing any change, you must:

1. **Run the parity check script** to see what's pending:
   ```bash
   bash scripts/check-parity.sh
   ```

2. **Check the parity matrix** at `docs/plans/client-parity.md` — if your change matches a row that says "iOS pending" or "PWA pending", you MUST update the corresponding client.

3. **For PWA changes** (`web/static/js/`, `web/templates/`, `web/static/css/`):
   - Check if `ios/Nabu/` needs the same change (models, views, store calls)
   - If the iOS app needs changes, make them in the same PR
   - If the change is truly PWA-only, explain why in the PR description

4. **For iOS changes** (`ios/`):
   - Check if `web/static/js/` needs the same change
   - If the PWA needs changes, make them in the same PR
   - If the change is truly iOS-only, explain why in the PR description

5. **For API/backend changes** (`internal/`):
   - Check if both client model files need updates (Swift `Models.swift` / `RequestModels.swift` and JS `api.js` / state models)
   - Check if both clients' store/service files need new API calls
   - Check if fixture files under `ios/NabuTests/Fixtures/` need updates

6. **PR description** must include one of these three statements verbatim:
   - "PWA and iOS both updated."
   - "PWA-only change; iOS not affected because [reason]."
   - "iOS-only change; PWA not affected because [reason]."

7. **Update the parity matrix** if you add a new feature row or change the status of an existing row.

## Key files to check

| PWA | iOS | Both consume |
|-----|-----|-------------|
| `web/static/js/app.js` | `ios/Nabu/ContentView.swift` | API endpoints |
| `web/static/js/state.js` | `ios/Nabu/App/AppState.swift` | Request/response models |
| `web/static/js/api.js` | `ios/Nabu/API/APIClient.swift` | `/api/` routes |
| `web/static/js/notifications.js` | `ios/Nabu/Views/NotificationPreferencesView.swift` | `/api/notification-preferences` |
| `web/static/js/schedule.js` | `ios/Nabu/Views/ScheduleView.swift` | `/api/schedules` |
| `web/static/js/chores.js` | `ios/Nabu/Views/ChoreEditView.swift` | `/api/chores` |
| `web/static/js/today.js` | `ios/Nabu/Views/LogSheet.swift` | `/api/logs` |
| `web/static/js/home.js` | `ios/Nabu/Views/HomeView.swift` | `/api/logs/latest-per-chore` |
| `web/static/js/calendar.js` | `ios/Nabu/Views/ActivityView.swift` | `/api/logs/today`, `/api/logs/week` |
| `web/static/js/stats.js` | `ios/Nabu/Views/StatsView.swift` | `/api/stats/*` |
| `web/static/js/household.js` | `ios/Nabu/Views/HouseholdView.swift` | `/api/household*` |
| `web/static/js/preferences.js` | `ios/Nabu/API/Data/PreferencesDataLoader.swift` | `/api/preferences` |
