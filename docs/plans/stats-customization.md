# Plan: Customizable Stats Sections

## Goal

Give users control over which sections appear on the Stats page and in what order. Today the stats page hard-codes 9 sections in a fixed order, with the Baby section privileged at the top. Users without babies cannot remove it, and readers/parents who care about other data (books, movies, leaderboard, etc.) can't reorder what they see.

This plan adds per-user **reorder** + **hide/show** of the existing stats sections, persisted alongside the existing per-user preferences (chore order, hidden home chores, timezone). No new section types are introduced.

## Current State

### Sections (hard-coded order in `renderStatsPage()`)

`web/static/js/stats.js:91-191` renders exactly these sections, in this order, with no reordering or visibility preference:

| # | Section key (proposed) | Heading | Render function | Notes |
|---|------------------------|---------|-----------------|-------|
| 1 | `overview` | (4 overview cards) | `renderOverviewCards()` | Always rendered; considered the page header. |
| 2 | `baby` | "Baby" | `renderBabyCareSection()` | Conditionally rendered if "Feed Baby" or "Change Baby" chores exist with data. Has a Daily/Weekly/Monthly toggle and feeding-gaps plot. |
| 3 | `activity` | "Activity" | inline heatmap | `renderHeatmapGrid(heatmap)` |
| 4 | `busy-hours` | "Busy Hours" | inline chart + filters | `renderBusyHoursChart()` + chore/member/date-range filters |
| 5 | `leaderboard` | "Leaderboard" | `renderLeaderboardList()` | Week/Month toggle |
| 6 | `top-chores` | (no heading) | `renderTopChoresSection()` | Top 5 per user |
| 7 | `categories` | "Categories" | `renderCategoryBars()` | |
| 8 | `chores` | "Chores" | `renderChoreStatsList()` | Per-chore expandable `<details>` |
| 9 | `recap` | "Weekly Recap" | inline | Conditional: only if `recap.totalChores > 0` |

### Preferences model

Per-user, stored in the `user_preferences` table:

```sql
user_id              BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE
chore_order          JSONB NOT NULL DEFAULT '[]'
hidden_home_chore_ids JSONB NOT NULL DEFAULT '[]'
timezone             TEXT NOT NULL DEFAULT ''
updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

Go struct (`internal/userprefs/store.go:6-21`):

```go
type Preferences struct {
    ChoreOrder         []int64 `json:"choreOrder"`
    HiddenHomeChoreIDs []int64 `json:"hiddenHomeChoreIds"`
    Timezone           string  `json:"timezone"`
}
```

Three stores implement the same `Store` interface: `postgres_store.go`, `memory_store.go`, and (only used in tests) stubs.

Service layer (`internal/userprefs/service.go`) wraps the store with `GetPreferences`, `UpdateChoreOrder`, `UpdateHiddenHomeChores`, `UpdateTimezone`. Each `Update*` reads-then-upserts so other fields are preserved.

Handler (`internal/handlers/preferences.go`) exposes `GET /api/preferences` and `PATCH /api/preferences` (PATCH accepts partial updates: only the fields present in the JSON body are written).

Frontend (`web/static/js/preferences.js`) ships `loadPreferences(state)`, `saveChoreOrder`, `saveHiddenHomeChores`, `syncTimezone`, and the `sortChoresByOrder` utility. Each save function does optimistic UI: mutate `state` first, then `apiFetch` PATCH, then reconcile with the server echo.

## Design Decisions

Confirmed with the user:

1. **Customization level**: Reorder + hide/show. Users drag-to-reorder sections AND toggle each section visible/hidden via a "Customize Stats" panel. No new section types; no editing section contents.
2. **Baby section treatment**: The baby section becomes a regular, reorderable, hideable section (loses its hard-coded "always near the top" status) BUT it stays **visible by default** for users who currently have baby data. New users without baby chores never see it (same as today — it's already conditionally rendered when there's no baby data).
3. **Layout scope**: Per-user. Each household member has their own stats layout. Stored in the existing `user_preferences` table, consistent with chore order / timezone being per-user.
4. **New section handling**: Any future section a developer ships is **appended at the bottom** of the user's existing list and is **visible by default**. Existing layouts are untouched.

## Data Model

### New migration: `migrations/033_stats_section_prefs.sql`

Add two JSONB columns to `user_preferences`:

```sql
ALTER TABLE user_preferences
  ADD COLUMN IF NOT EXISTS stats_section_order JSONB NOT NULL DEFAULT '[]';
ALTER TABLE user_preferences
  ADD COLUMN IF NOT EXISTS stats_section_hidden JSONB NOT NULL DEFAULT '[]';
```

- `stats_section_order`: ordered array of canonical section keys (strings), e.g. `["baby","overview","leaderboard","activity","busy-hours","top-chores","categories","chores","recap"]`.
- `stats_section_hidden`: array of section keys the user has hidden, e.g. `["baby"]`.

Both default to `'[]'`. An empty `stats_section_order` means "use the canonical default order" (see Layout Resolution below). This keeps existing rows and new users working without changes.

### Go struct changes (`internal/userprefs/store.go`)

Add two fields to `Preferences`:

```go
type Preferences struct {
    ChoreOrder         []int64 `json:"choreOrder"`
    HiddenHomeChoreIDs []int64 `json:"hiddenHomeChoreIds"`
    Timezone           string  `json:"timezone"`

    // StatsSectionOrder is the user's preferred ordering of stats page
    // sections, expressed as an ordered list of canonical section keys.
    // Missing or empty means "use the canonical default order".
    StatsSectionOrder []string `json:"statsSectionOrder"`

    // StatsSectionHidden is the set of section keys the user has removed
    // from the stats page. Hidden sections are not rendered.
    StatsSectionHidden []string `json:"statsSectionHidden"`
}
```

## Canonical Section Registry

Define a single source of truth for section keys and their default order. Place this **both** in the backend (for validation) and the frontend (for rendering):

### Backend (`internal/userprefs/sections.go` — new file)

```go
package userprefs

// StatsSections lists every stats section in canonical default order.
// This is the single source of truth for section key names.
//
// When you add a new stats section, append it to the END of this list.
// Existing users will see the new section appear (visible by default)
// below their existing sections, per the layout-resolution algorithm.
var StatsSections = []string{
    "overview",
    "baby",
    "activity",
    "busy-hours",
    "leaderboard",
    "top-chores",
    "categories",
    "chores",
    "recap",
}

// IsKnownStatsSection reports whether key is a recognized stats section.
func IsKnownStatsSection(key string) bool {
    for _, s := range StatsSections {
        if s == key {
            return true
        }
    }
    return false
}

// DefaultStatsSectionOrder returns a copy of the canonical order.
func DefaultStatsSectionOrder() []string {
    out := make([]string, len(StatsSections))
    copy(out, StatsSections)
    return out
}
```

### Frontend (`web/static/js/stats.js` — new export)

```js
// Canonical section list and default order. Must match
// internal/userprefs/sections.go exactly. When you add a new section,
// append it to the END of this list.
export const STATS_SECTIONS = [
  "overview",
  "baby",
  "activity",
  "busy-hours",
  "leaderboard",
  "top-chores",
  "categories",
  "chores",
  "recap",
];
```

**Invariant**: The two lists MUST contain the same keys in the same order. A future lab should add a JS unit test asserting parity with a hard-coded copy of the backend list (see Testing below).

### Section key rules

- Lowercase kebab-case strings, e.g. `busy-hours`, `top-chores`.
- Stable forever — once a key ships, it must never be renamed or removed without a migration that rewrites user prefs.
- Adding a new section = append a new key to the END of `StatsSections` and `STATS_SECTIONS` only.

## Layout Resolution Algorithm

When rendering the stats page, the frontend computes the effective section list by merging the user's stored order with the canonical registry. This algorithm lives in a pure function `resolveStatsLayout(userOrder, userHidden)` exported from `stats.js`:

```
1. Start with canonical STATS_SECTIONS (the registry).
2. Build an ordered list:
   a. For each key in userOrder (in the user's order): include it if it
      exists in the canonical registry AND is not in userHidden.
   b. For each key in the canonical registry not already included:
      include it (at the end), unless it's in userHidden.
      (These are new sections the user hasn't ordered yet — they appear
      at the bottom, visible by default, per the user's design choice.)
3. The result is the ordered, visible section list to render.
```

Notes:
- Unknown keys in `userOrder` (renamed/removed sections from old data) are silently dropped by step 2a.
- A section in `userHidden` is excluded regardless of whether it's in `userOrder`.
- An empty `userOrder` returns the canonical list minus hidden — i.e. the default. This is the existing behavior for new users.
- The baby section's existing "only render if baby chores have data" guard is **preserved**. Hiding via the new pref is a second, user-controlled hide; the data-driven guard still applies on top. Same with `recap`'s `totalChores > 0` guard.

## Backend Implementation

### `internal/userprefs/store.go`

Add `StatsSectionOrder []string` and `StatsSectionHidden []string` to the `Preferences` struct (shown above under Data Model).

### `internal/userprefs/postgres_store.go`

- **`Get`**: add `stats_section_order` and `stats_section_hidden` to the `SELECT`. Read each as `[]byte`, `json.Unmarshal` into `[]string`, normalize `nil` → `[]string{}` (matching the existing pattern for `chore_order` and `hidden_home_chore_ids`).
- **`Upsert`**: add both new columns to the `INSERT` and `ON CONFLICT ... DO UPDATE SET` clauses. `json.Marshal` the slices (or `[]string{}` if nil) into bind parameters, exactly as `chore_order` is handled today.

### `internal/userprefs/memory_store.go`

- **`Get`**: extend the zero-value fallback and the copy logic to include `StatsSectionOrder` and `StatsSectionHidden`. Initialize to `[]string{}` when missing. Use `make([]string, len(...))` + `copy` on the return path, exactly as `ChoreOrder` is handled today.
- **`Upsert`**: extend the defensive copy to include both new fields.

### `internal/userprefs/service.go`

Add two new methods mirroring `UpdateChoreOrder`:

```go
// UpdateStatsSectionOrder persists the user's preferred ordering of stats
// page sections. Keys must be drawn from the canonical StatsSections list;
// unknown keys are rejected. The list may omit or duplicate keys — the
// frontend's resolveStatsLayout function handles both cases.
func (s *Service) UpdateStatsSectionOrder(ctx context.Context, userID int64, order []string) error {
    if order == nil {
        order = []string{}
    }
    // Validate: every key must be a known section.
    for _, k := range order {
        if !IsKnownStatsSection(k) {
            return fmt.Errorf("unknown stats section: %q", k)
        }
    }
    prefs, err := s.store.Get(ctx, userID)
    if err != nil {
        return err
    }
    prefs.StatsSectionOrder = order
    return s.store.Upsert(ctx, userID, prefs)
}

// UpdateStatsSectionHidden persists the set of stats sections the user has
// hidden from the stats page. Keys must be drawn from the canonical
// StatsSections list.
func (s *Service) UpdateStatsSectionHidden(ctx context.Context, userID int64, hidden []string) error {
    if hidden == nil {
        hidden = []string{}
    }
    for _, k := range hidden {
        if !IsKnownStatsSection(k) {
            return fmt.Errorf("unknown stats section: %q", k)
        }
    }
    prefs, err := s.store.Get(ctx, userID)
    if err != nil {
        return err
    }
    prefs.StatsSectionHidden = hidden
    return s.store.Upsert(ctx, userID, prefs)
}
```

Validation note: we reject unknown keys at the service layer (defense in depth) — the handler does NOT rely on this alone.

### `internal/handlers/preferences.go`

Extend the PATCH request struct and add handling blocks (mirroring the existing fields):

```go
var req struct {
    ChoreOrder         *[]int64 `json:"choreOrder"`
    HiddenHomeChoreIDs *[]int64 `json:"hiddenHomeChoreIds"`
    Timezone           *string  `json:"timezone"`
    StatsSectionOrder  *[]string `json:"statsSectionOrder"`
    StatsSectionHidden *[]string `json:"statsSectionHidden"`
}
```

Add two handler blocks after the existing `Timezone` block:

```go
if req.StatsSectionOrder != nil {
    if err := h.service.UpdateStatsSectionOrder(r.Context(), user.ID, *req.StatsSectionOrder); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
}
if req.StatsSectionHidden != nil {
    if err := h.service.UpdateStatsSectionHidden(r.Context(), user.ID, *req.StatsSectionHidden); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
}
```

Note: use `http.StatusBadRequest` (400) for unknown keys (validation error), not 500 — this matches how a caller might pass garbage JSON.

No changes needed to `Get` — the existing `writeJSON(..., map[string]any{"preferences": prefs})` serializes the full struct including the new fields automatically.

### Route registration (`internal/app/server.go`)

No changes. `GET /api/preferences` and `PATCH /api/preferences` already exist and the new fields flow through transparently.

### Server wiring

No changes to `BuildServer` — the same `userPrefsStore` is already passed to the `PreferencesHandler`. Checked at `internal/app/server.go` near the `PreferencesHandler` construction.

## Frontend Implementation

### `web/static/js/state.js`

Add two new fields to the initial state returned by `createAppState()`:

```js
stats: {
  ...existingFields,
  sectionOrder: [],      // ordered array of section keys (user pref)
  sectionHidden: [],     // array of hidden section keys (user pref)
  customizeOpen: false, // whether the "Customize Stats" panel is open
},
```

Also set initial values on the top-level state (some prefs live there today, e.g. `state.timezone`). Mirror whichever pattern `state.js` already uses for `choreOrder` / `hiddenHomeChoreIDs`. Read `state.js` first and follow it exactly — do not introduce a new convention.

### `web/static/js/preferences.js`

Extend `loadPreferences(state)`:

```js
state.stats.sectionOrder  = data?.preferences?.statsSectionOrder  ?? [];
state.stats.sectionHidden = data?.preferences?.statsSectionHidden ?? [];
```

(Mirror the try/catch fallback to `[]` for both fields, exactly as `choreOrder` is handled.)

Add two new exports, mirroring `saveChoreOrder`:

```js
export async function saveStatsSectionOrder(state, order) {
  state.stats.sectionOrder = order;
  try {
    const { data } = await apiFetch("/api/preferences", {
      method: "PATCH",
      body: JSON.stringify({ statsSectionOrder: order }),
    });
    state.stats.sectionOrder = data?.preferences?.statsSectionOrder ?? order;
  } catch {
    // Keep the optimistic value.
  }
}

export async function saveStatsSectionHidden(state, hidden) {
  state.stats.sectionHidden = hidden;
  try {
    const { data } = await apiFetch("/api/preferences", {
      method: "PATCH",
      body: JSON.stringify({ statsSectionHidden: hidden }),
    });
    state.stats.sectionHidden = data?.preferences?.statsSectionHidden ?? hidden;
  } catch {
    // Keep the optimistic value.
  }
}
```

### `web/static/js/stats.js`

#### New: section registry + layout resolver (top of file, after imports)

```js
export const STATS_SECTIONS = [
  "overview","baby","activity","busy-hours","leaderboard",
  "top-chores","categories","chores","recap",
];

// resolveStatsLayout merges the user's stored order with the canonical
// registry. Unknown keys are dropped. Canonical keys missing from the
// user's order (e.g. newly-shipped sections) are appended at the end.
// Hidden sections are excluded.
export function resolveStatsLayout(userOrder, userHidden) {
  const hidden = new Set(userHidden || []);
  const seen = new Set();
  const out = [];
  for (const k of userOrder || []) {
    if (STATS_SECTIONS.includes(k) && !hidden.has(k) && !seen.has(k)) {
      out.push(k); seen.add(k);
    }
  }
  for (const k of STATS_SECTIONS) {
    if (!seen.has(k) && !hidden.has(k)) {
      out.push(k); seen.add(k);
    }
  }
  return out;
}
```

#### Refactor `renderStatsPage(state)`

Instead of returning one big template literal with the 9 sections hard-coded in order, build a registry keyed by section key, then iterate `resolveStatsLayout(...)`:

```js
export function renderStatsPage(state) {
  // ... existing setup code (lines 92-118, unchanged) ...

  const order = resolveStatsLayout(
    state.stats?.sectionOrder,
    state.stats?.sectionHidden,
  );

  // Build one HTML string per section. Use a map of key -> rendered HTML.
  // Each section's content is the SAME markup that lives inline today;
  // we're only changing how it's gathered and ordered.
  const sections = {
    "overview": renderOverviewCards(todayCount, totalThisWeek, streaks, topChoreName, stats.leaderboardPeriod, state.user?.id),
    "baby": renderBabyCareSection(state),
    "activity": `<div class="card mb-3"><h3>Activity</h3>${renderHeatmapGrid(heatmap)}</div>`,
    "busy-hours": /* existing busy-hours card markup */,
    "leaderboard": /* existing leaderboard card markup */,
    "top-chores": renderTopChoresSection(state),
    "categories": /* existing categories card markup */,
    "chores": /* existing chores card markup */,
    "recap": recap.totalChores > 0 ? /* existing recap card markup */ : "",
  };

  // Sections that produced empty HTML (baby section with no data, recap
  // with 0 chores) are skipped here — preserving the existing
  // data-driven conditional rendering.
  const body = order
    .map(k => sections[k])
    .filter(html => html && html.trim().length > 0)
    .join("\n");

  return `<div class="stats-page">
    <h2>Stats</h2>
    <div class="stats-header-row">
      <button class="btn-link" data-action="toggle-customize-stats">
        ${state.stats?.customizeOpen ? "Done" : "Customize"}
      </button>
    </div>
    ${state.stats?.customizeOpen ? renderCustomizePanel(state) : ""}
    ${body}
  </div>`;
}
```

Notes for the implementer:
- The `overview` section's wrapping `<div class="chart-period-toggle mt-2 mb-3">` is preserved inside the `sections.overview` string.
- The `recap` section already returns `""` when `recap.totalChores === 0`. The new `.filter(html => html && html.trim() !== "")` skips it. Same goes for the baby section when there's no data.
- Do NOT remove the existing data-driven guards. The user's `sectionHidden` pref is a second layer; the existing guards still apply.

#### New: `renderCustomizePanel(state)`

A panel shown above the stats content when `state.stats.customizeOpen === true`. It lists every canonical section with:
- A checkbox to toggle visibility (on = visible, off = hidden). Hidden sections show as "off".
- A drag handle (a `⋮⋮` or `⠿` glyph) next to each row.
- Disabled (greyed) checkbox for `overview` — the overview cards are treated as the page header and are always visible. Marking it hidden is rejected client-side and (defensively) server-side; show it as always-checked and disabled.

Markup sketch (adapt to existing CSS conventions — see "CSS" below):

```js
function renderCustomizePanel(state) {
  const hidden = new Set(state.stats?.sectionHidden || []);
  const ordered = resolveStatsLayout(state.stats?.sectionOrder, []); // show all, even hidden ones
  // Always ensure hidden-but-known sections appear at the end of the list:
  const allKeys = [...ordered, ...STATS_SECTIONS.filter(k => !ordered.includes(k))];
  const rows = allKeys.map((k, i) => {
    const isHidden = hidden.has(k);
    const isOverview = k === "overview";
    const label = SECTION_LABELS[k];
    return `<div class="customize-row" draggable="true" data-section="${k}">
      <span class="drag-handle" aria-hidden="true">⠿</span>
      <label class="customize-check">
        <input type="checkbox" data-action="toggle-stats-section"
               data-section="${k}" ${(!isHidden || isOverview) ? "checked" : ""}
               ${isOverview ? "disabled" : ""} />
        <span>${escapeHTML(label)}</span>
      </label>
    </div>`;
  }).join("");
  return `<div class="card mb-3 customize-panel">
    <h3>Customize Stats</h3>
    <p class="customize-hint">Drag to reorder. Uncheck to hide a section.</p>
    ${rows}
  </div>`;
}

const SECTION_LABELS = {
  overview: "Overview cards",
  baby: "Baby care",
  activity: "Activity (heatmap)",
  "busy-hours": "Busy hours",
  leaderboard: "Leaderboard",
  "top-chores": "Top chores",
  categories: "Categories",
  chores: "Chores",
  recap: "Weekly recap",
};
```

Use `escapeHTML()` from `utils.js` for the labels — labels are developer-controlled, so this is defense-in-depth rather than a live vulnerability, but follow the codebase rule (AGENTS.md: "Escape all user-controlled strings in HTML templates" and existing stats.js already imports `escapeHTML`).

#### Drag-to-reorder interaction

Implement native HTML5 drag-and-drop (no library — the codebase has no bundler).

- `dragstart` on `.customize-row`: store the dragged section key.
- `dragover` on `.customize-row`: `e.preventDefault()` to allow drop, add a `.drag-over` class for visual feedback.
- `drop` on `.customize-row`: reorder `state.stats.sectionOrder` so the dragged key sits at the drop position, then re-render.
- `dragleave` / `dragend`: remove `.drag-over` class.

Wire these in `app.js`'s event delegation block (the same `#app` container that handles every other stats interaction). Look at how the existing `busy-hours-filter` and `chore-stats-filter` actions are dispatched — follow that exact pattern.

#### Hide/show interaction

- The `toggle-stats-section` action handler reads `data-section` from the changed checkbox:
  - If now unchecked AND not `overview`: add the section to `state.stats.sectionHidden`, call `saveStatsSectionHidden(state, newHidden)`, re-render.
  - If now checked: remove from `state.stats.sectionHidden`, call save, re-render.
- `overview` checkbox is `disabled` so the handler should never fire for it; if it does (defensive), no-op.

#### Toggling the customize panel

- The `toggle-customize-stats` action handler flips `state.stats.customizeOpen` and re-renders.

#### "Reorder" via the customize panel

When the user reorders via drag, compute the new `sectionOrder` = list of keys as displayed in the panel (including hidden ones, so reordering stays stable across hide/show). Then call `saveStatsSectionOrder(state, newOrder)` and re-render.

### `web/static/js/app.js`

- On initial load (`loadStatsData` or wherever preferences are loaded today — check both `loadPreferences(state)` call site and `loadAllStatsData()`), ensure `state.stats.sectionOrder` and `state.stats.sectionHidden` are loaded before `renderStatsPage` is called. `loadPreferences` already runs early on app boot — confirm ordering by checking where it is invoked.
- Add event delegations for: `toggle-customize-stats`, `toggle-stats-section` (change event), and the DnD events on `.customize-row`.

Use the existing `loadAllStatsData()` entry point — no new fetch requests are needed because section prefs come via the existing `GET /api/preferences` (already called on boot).

### CSS (`web/static/css/app.css`)

Add styles grouped with the existing stats CSS (lines ~1170-1825). Suggestions:

```css
.stats-header-row { display: flex; justify-content: flex-end; }
.btn-link { background: none; border: none; color: var(--link, #2563eb);
            font-size: 0.875rem; cursor: pointer; padding: 0.25rem 0.5rem; }
.customize-panel .customize-hint { color: #6b7280; font-size: 0.8rem;
                                   margin: 0 0 0.75rem; }
.customize-row { display: flex; align-items: center; gap: 0.5rem;
                 padding: 0.5rem; border: 1px solid transparent;
                 border-radius: 6px; cursor: grab; }
.customize-row.drag-over { border-color: #2563eb; background: #eff6ff; }
.customize-row .drag-handle { color: #9ca3af; cursor: grab;
                              user-select: none; }
.customize-check { display: flex; align-items: center; gap: 0.5rem;
                   cursor: pointer; }
.customize-check input:disabled + span { color: #9ca3af; }
```

Always run `make local-fresh` after touching `web/static/` because the assets are embedded via `//go:embed` in `web/assets.go`.

## Default Behaviors

### New users (no preferences row yet)

`Get` returns `Preferences{StatsSectionOrder: []string{}, StatsSectionHidden: []string{}}`. `resolveStatsLayout([], [])` returns the canonical order — i.e. exactly today's behavior. The Baby section appears only if the household has Feed/Change baby chores (existing data-driven guard). No migration backfill needed.

### Existing users (row exists, new columns default `'[]'`)

The migration default of `'[]'` means `Get` reads `[]string{}` for both new fields. Same as new users — `resolveStatsLayout` falls back to canonical order. Existing users with baby data see the Baby section **at its canonical position** (currently second, after overview). Per the user's decision, the baby section is NOT pinned to the top anymore — but since canonical order already places it second (right after overview), the visible behavior is unchanged unless the user explicitly moves it.

### Existing users who want the baby section gone

They open Customize → uncheck "Baby care". `saveStatsSectionHidden(state, ["baby"])` → server PATCH → state updates → re-render without the baby section. No data is deleted; only the visibility pref.

## iOS Implementation (required for parity)

Per AGENTS.md, this UI behavior change requires both clients updated in the same PR. The iOS app DOES have a fully-built Stats view and must mirror the PWA behavior.

### Honest state of iOS stats code

The parity matrix (`docs/plans/client-parity.md:90-99`) lists many iOS test files — `StatsUITests.swift`, `StatsSnapshotTests.swift`, `BabyCareUITests.swift`, `TopChoresUITests.swift`, `BusyHoursUITests.swift`, `StatsTimezoneContractTests.swift`. **Most of these do not actually exist on disk.** At time of writing, only the production source is real:

| Real file | Purpose | LOC |
|-----------|---------|-----|
| `ios/Nabu/Views/StatsView.swift` | Full SwiftUI Stats view (all 9 sections rendered) | ~1355 |
| `ios/Nabu/API/Models.swift:341` | `UserPreferences` Codable (3 fields only — no stats prefs) | — |
| `ios/Nabu/API/RequestModels.swift:181` | `PatchUserPreferencesRequest` (3 fields only) | — |
| `ios/Nabu/API/Data/PreferencesDataLoader.swift` | Loads `choreOrder` + `hiddenHomeChoreIds` into `state`; ignores `timezone` | — |
| `ios/NabuTests/ModelDecodingTests.swift:516` | `testDecodeUserPreferences` decodes the 3 existing fields | — |

The 1355-line `StatsView.swift` body at `StatsView.swift:53-80` hard-codes a `VStack(spacing: 16)` in this order:

```
overviewRow
babyCareSection        (if feedBabyTS != nil || changeBabyTS != nil)
heatmapCard            (if !heatmap.isEmpty)
busyHoursCard          (if !busyHours.isEmpty)
leaderboardCard        (if ov != nil)
topChoresSection
categoriesCard         (if ov != nil && !breakdown.isEmpty)
choreStatsSection      (if !activeChoreStats.isEmpty)
recapCard              (if ov != nil && recap.totalChores > 0)
```

This mirrors the PWA's section list (with the same data-driven visibility guards), so the PWA's `STATS_SECTIONS` keys map cleanly onto a Swift port.

### Logic to share

The `resolveStatsLayout` algorithm (see PWA section above) must be ported to Swift. Place it as a free function in `StatsView.swift` or a small `ios/Nabu/Views/StatsSectionLayout.swift` helper:

```swift
let statsSectionsCanonical: [String] = [
    "overview","baby","activity","busy-hours","leaderboard",
    "top-chores","categories","chores","recap",
]

func resolveStatsLayout(userOrder: [String], hidden: [String]) -> [String] {
    let hiddenSet = Set(hidden)
    var seen = Set<String>()
    var out: [String] = []
    for k in userOrder where statsSectionsCanonical.contains(k)
                              && !hiddenSet.contains(k) && !seen.contains(k) {
        out.append(k); seen.insert(k)
    }
    for k in statsSectionsCanonical where !seen.contains(k) && !hiddenSet.contains(k) {
        out.append(k); seen.insert(k)
    }
    return out
}
```

**Parity invariant**: this Swift array MUST contain the same keys in the same order as the PWA's `STATS_SECTIONS`. Add an XCTest asserting deep equality with a hard-coded copy of the backend list (mirror the PWA parity test).

### iOS implementation steps

The implementing agent should:

1. **Models** (`ios/Nabu/API/Models.swift`):
   - Extend `UserPreferences` struct (line 341) with two optional `[String]?` fields — `statsSectionOrder` and `statsSectionHidden`. Use optionals so existing server responses that omit them decode as `nil` (Swift will fail decode if the fields are non-optional and missing).
   - Keep the struct `Codable, Equatable`.

2. **PATCH model** (`ios/Nabu/API/RequestModels.swift`):
   - Extend `PatchUserPreferencesRequest` (line 181) with two new optional `[String]?` fields, defaulting to `nil`. iOS sends `null` for omitted fields (per Swift's `encodeIfPresent`) — Go's `*[]string` pointer unmarshaling treats `null` as "no update" correctly.

3. **State** (`ios/Nabu/App/AppState.swift`):
   - Add two `@Published` (or observable-equivalent) fields: `statsSectionOrder: [String] = []` and `statsSectionHidden: [String] = []`. Mirror the existing pattern used for `choreOrder` / `hiddenHomeChoreIDs`.

4. **Data loader** (`ios/Nabu/API/Data/PreferencesDataLoader.swift`):
   - In `loadPreferences()` (line 13-21), populate `state.statsSectionOrder = data.preferences.statsSectionOrder ?? []` and `state.statsSectionHidden = data.preferences.statsSectionHidden ?? []`. Coalesce to `[]` (not `nil`) so the resolver and view always see a real array.
   - Add two new methods mirroring the existing `syncTimezone` flow: `func updateStatsSectionOrder(_ order: [String]) async` and `func updateStatsSectionHidden(_ hidden: [String]) async`. Each builds a `PatchUserPreferencesRequest` with only the relevant field set and PATCHes `/api/preferences`. Maintain optimistic updates (mutate `state` first, patch, ignore silent failures) — the existing `syncTimezone` is the pattern to copy.

5. **Refactor `StatsView.body`** (`ios/Nabu/Views/StatsView.swift:53-80`):
   - Build a `[String: () -> AnyView]` (or `@ViewBuilder`-based) lookup of section key → view-builder. Each entry wraps the existing `overviewRow`, `babyCareSection`, `heatmapCard`, etc.
   - Compute `let visible = resolveStatsLayout(userOrder: state.statsSectionOrder, hidden: state.statsSectionHidden)`.
   - Render via `ForEach(visible, id: \.self) { key in sectionBuilders[key]?() }` (provide a fallback empty view for unknown keys — should never happen since the resolver already filters).
   - **Keep the existing data-driven guards** per section (e.g. `feedBabyTS != nil || changeBabyTS != nil` for `baby`). The hidden-pref is an OUTER gate; if a section returns `EmptyView()` due to no data, that section just renders nothing.
   - Add a "Customize" toolbar button in the `NavigationStack` that toggles a sheet presenting a `StatsCustomizeView`.

6. **New `StatsCustomizeView`** (`ios/Nabu/Views/StatsCustomizeView.swift` — new file):
   - SwiftUI `List` with `EditMode.active` and `.onMove` for drag-to-reorder (use the standard iOS `List(...).onMove` modifier — no third-party library needed).
   - Each row: `Toggle` bound to "is this section NOT in `statsSectionHidden`", plus a drag handle. Disable the row for `"overview"` (always-visible, locked).
   - On any change (toggle flip or move), call the matching `updateStatsSectionOrder` / `updateStatsSectionHidden` on the data loader.
   - Dismiss button in the navigation bar.

7. **Decoding tests** (`ios/NabuTests/ModelDecodingTests.swift`):
   - Extend `testDecodeUserPreferences` (line 516) to add `statsSectionOrder` and `statsSectionHidden` to the JSON fixture and assert both decode.
   - Add a sibling test `testDecodeUserPreferencesMissingStatsFields` that decodes JSON without the new fields and asserts both decode to `nil` (or `[]` after the data-loader coalesces). This proves backwards compatibility with servers that haven't deployed the migration yet.

8. **XCUITest** (`ios/NabuUITests/NabuUITests.swift` — extend, or new file in the same target):
   - Following the existing `NabuHomeEndToEndUITests` style (which registers a fresh household via launch args and exercises flows against a local server):
     - Add a test that: opens the Stats tab, taps "Customize", hides "Leaderboard" via Toggle, dismisses, asserts the visible Stats view no longer shows the Leaderboard card.
     - Add a (separate) test that reorders a section via the iOS drag interaction and asserts the new order persists after app relaunch.
   - If `NabuUITests.swift` proves hard to extend, drop these tests and instead add a focused `XCTest` in `NabuTests/StatsCustomizeTests.swift` that exercises the `resolveStatsLayout` Swift function directly (input/output tuples). That at minimum guards algorithm parity.

9. **Parity matrix** (`docs/plans/client-parity.md`):
   - Add a new row under the **Preferences** section (around line 105-107), e.g.:
     `Stats section customization | preferences.js, stats-customize.spec.js | Views/StatsCustomizeView.swift, ModelDecodingTests.swift | /api/preferences | Done |`
   - If the iOS implementation is fully landed in the same PR, set parity `Done`. If iOS work is incomplete, set `iOS pending` and use the standard PR description variant (see below).

### PR description

This change touches both clients, so the PR description MUST say verbatim:

> **PWA and iOS both updated.**

Do not use "PWA-only change; iOS not affected because…" — that would be false.

### What the parity matrix already says (aspirational)

The matrix lists iOS test files that don't exist on disk (e.g. `StatsUITests.swift`, `StatsSnapshotTests.swift`). This is pre-existing debt; this plan does NOT require the implementing agent to create all of those. Only the work items in the section above are required.

If a CI parity job fails specifically because it expects one of those aspirational test files to exist with content, treat that as a pre-existing parity bug — file separately, do not block this feature on it.

## Testing Strategy

Run after EACH step, not just at the end. Pre-push: `go build ./...`, `go vet ./...`, `make test-go`, `make test-js`. Then `make local-fresh && make e2e`.

### Go unit tests (`internal/userprefs/userprefs_test.go` — extend)

Add cases (use the existing in-file style; the file already tests `MemoryStore` + `Service` together):

1. **`TestPreferencesStatsSectionRoundTrip`**: `Upsert` a `Preferences` with `StatsSectionOrder: []string{"baby","overview","leaderboard"}` and `StatsSectionHidden: []string{"recap"}`. `Get` returns the same values. Validates the memory store handles the new fields and the copy-on-Get path doesn't drop them.
2. **`TestPreferencesDefaultsEmptyStatsSections`**: A `Get` on an unknown user returns `StatsSectionOrder == []string{}` (not nil) and `StatsSectionHidden == []string{}`. Critical for the JS fallback `?? []`.
3. **`TestUpdateStatsSectionOrderRejectsUnknownKey`**: Calling `Service.UpdateStatsSectionOrder(ctx, uid, []string{"baby","nonexistent"})` returns a non-nil error and the stored prefs are unchanged.
4. **`TestUpdateStatsSectionHiddenRejectsUnknownKey`**: Same as above for `UpdateStatsSectionHidden`.
5. **`TestUpdateStatsSectionOrderPreservesOtherFields`**: After setting `StatsSectionOrder`, verify `ChoreOrder`, `HiddenHomeChoreIDs`, `Timezone`, and `StatsSectionHidden` are unchanged. Mirrors the existing `TestUpdateChoreOrderPreserves*` tests if present — check the file first.
6. **`TestIsKnownStatsSection`**: True for every key in `StatsSections`; false for `"nonexistent"`, `""`, `"Overview"` (case-sensitive).
7. **`TestDefaultStatsSectionOrderIsCopy`**: Mutating the returned slice does not mutate the package-level `StatsSections`.

### Go handler tests (`internal/handlers/preferences_test.go` — extend)

1. **`TestPatchStatsSectionOrder`**: PATCH `{"statsSectionOrder":["baby","overview"]}` → 200, response JSON has `statsSectionOrder` matching.
2. **`TestPatchStatsSectionUnknownKey`**: PATCH `{"statsSectionOrder":["baby","bogus"]}` → 400 with error mentioning `bogus`.
3. **`TestPatchStatsSectionHidden`**: PATCH `{"statsSectionHidden":["recap"]}` → 200, response reflects the value.
4. **`TestGetPreferencesIncludesNewFields`**: GET `/api/preferences` for a fresh user returns `statsSectionOrder: []` and `statsSectionHidden: []` (not `null`).

### JS unit tests (`web/static/js/tests/stats.test.js` — new file, or extend existing stats test file)

Use Node's built-in test runner (`node --test`), matching the convention noted in AGENTS.md. Locate the existing `web/static/js/tests/` directory first and follow its style.

1. **`resolveStatsLayout` empty inputs** → returns canonical list verbatim.
2. **`resolveStatsLayout` user-ordered** → returns user order minus hidden, with canonical-only keys.
3. **`resolveStatsLayout` unknown key in user order** → dropped silently.
4. **`resolveStatsLayout` new section not in user order** → appended after user's keys, still visible.
5. **`resolveStatsLayout` hidden section** → excluded even if present in user order.
6. **`resolveStatsLayout` duplicates in user order** → deduped.
7. **STATS_SECTIONS parity with backend**: hard-code the canonical list (copy from `internal/userprefs/sections.go`) into the test file as a constant, then assert `STATS_SECTIONS` from `stats.js` deeply equals it. This catches drift between frontend and backend that a future contributor might introduce.

### E2E tests (`tests/e2e/stats-customize.spec.js` — new file)

Copy `setupWithChores` and `uniqueEmail` from an existing stats spec (`tests/e2e/stats-tab.spec.js`).

1. **`stats customize panel toggles open and closed`**:
   - Navigate to `/stats`.
   - Click "Customize" button.
   - Expect `.customize-panel` to be visible.
   - Click "Done" (button text changes).
   - Expect `.customize-panel` to be hidden.
2. **`stats section can be hidden`**:
   - Open customize panel.
   - Uncheck "Leaderboard" checkbox.
   - Expect the Leaderboard card (`<h3>Leaderboard</h3>`) to disappear from the stats page.
   - Reload page → still hidden (persistence).
3. **`stats section can be reordered via checkbox / drag-drop`**:
   - Open customize panel.
   - Drag "Categories" row above "Activity" row (use `page.locator('.customize-row[data-section="categories"]')` as drag source).
   - Expect `.customize-row[data-section="categories"]` to appear before `.customize-row[data-section="activity"]`.
   - Close panel.
   - On the stats page, expect the Categories card to appear before the Activity card.
   - Reload → ordering preserved.
4. **`stats overview cannot be hidden`**:
   - Open customize panel.
   - Expect the "Overview cards" checkbox to be `disabled` and checked.
   - Expect clicking it has no effect.
5. **`stats baby section stays visible by default for existing baby-data users`**:
   - Set up a household with seeded defaults (which include Feed Baby / Change Baby).
   - Log one "Feed Baby" entry.
   - Navigate to `/stats`.
   - Expect the Baby section's heading (`<h3>Baby</h3>` or `.baby-care-header`) to be visible WITHOUT opening customize.
   - This asserts the migration backfill / default behavior doesn't accidentally hide the baby section.
6. **`stats baby section can be hidden and stays hidden`**:
   - Continuing from test 5:
   - Open customize. Uncheck "Baby care". Close.
   - Expect no Baby section on the page.
   - Reload → still hidden.
   - Re-open customize, check "Baby care" again. Baby section reappears.

For the drag-and-drop step, use Playwright's `locator.dragTo(target)` and assert final order via the relative position of two `.customize-row` elements. If `dragTo` proves flaky, fall back to manual `mouse.down()` / `mouse.move()` / `mouse.up()` — only do this if the simple API fails.

### iOS tests (XCTest + XCUITest)

The full iOS work-item list is in the **iOS Implementation (required for parity)** section above. The tests required for parity:

1. **`testDecodeUserPreferences` (extend)** at `ios/NabuTests/ModelDecodingTests.swift:516` — add `statsSectionOrder` and `statsSectionHidden` to the JSON fixture; assert both decode to the expected arrays.
2. **`testDecodeUserPreferencesMissingStatsFields` (new)** — decode JSON WITHOUT the new fields; assert both decode to `nil` (so older servers don't break decode). This proves backwards compatibility for a server that hasn't yet applied migration 033.
3. **`testResolveStatsLayout*` (new, in `NabuTests/StatsCustomizeTests.swift` or wherever the Swift port of `resolveStatsLayout` is unit-testable)** — port the same 7 cases listed for the JS unit tests above (empty inputs, unknown keys dropped, hidden excluded, new sections appended, dedupe). Use `XCTAssertEqual`.
4. **`testStatsCustomizeParity` (new)** — hard-code the backend's canonical list (copy from `internal/userprefs/sections.go`) as a Swift constant, then assert the iOS `statsSectionsCanonical` array deeply equals it. Catches drift across the three sources (Go / JS / Swift).
5. **XCUITest** in `ios/NabuUITests/NabuUITests.swift` (or new file in the same target), following the existing `NabuHomeEndToEndUITests` style:
   - Open Stats → tap Customize → flip a Toggle off → dismiss → assert the corresponding section card is no longer visible.
   - If drag-reorder is testable on the simulator, add a reorder test asserting the new order persists after app relaunch. If flaky, defer to a `resolveStatsLayout` unit test (item 3) and document the gap in the PR description.

Pre-push for iOS: run `xcodebuild test -project ios/Nabu.xcodeproj -scheme Nabu -destination 'platform=iOS Simulator,name=iPhone 16'`; expect green.

## Step-by-Step Implementation Order

Each step should result in a working, testable state. Commit at boundaries (or at least stop and run tests). Do not bundle unrelated changes.

**Step 1 — Migration + Go model plumbing (no behavior change).**
- Create `migrations/033_stats_section_prefs.sql`.
- Edit `internal/userprefs/store.go` — add two fields to `Preferences`.
- Edit `internal/userprefs/sections.go` (new file) — `StatsSections`, `IsKnownStatsSection`, `DefaultStatsSectionOrder`.
- Edit `internal/userprefs/memory_store.go` — Get/Upsert copy paths.
- Edit `internal/userprefs/postgres_store.go` — Get/Upsert SQL.
- Extend `internal/userprefs/userprefs_test.go` — tests 1, 2, 6, 7 from Go tests list.
- Run: `go build ./... && go vet ./... && go test ./internal/userprefs/...`.
- **Nothing user-facing changes yet.** Old GET `/api/preferences` simply returns two new `[]` arrays.

**Step 2 — Service + handlers.**
- Edit `internal/userprefs/service.go` — `UpdateStatsSectionOrder`, `UpdateStatsSectionHidden`.
- Edit `internal/handlers/preferences.go` — PATCH struct + two handler blocks.
- Extend `internal/userprefs/userprefs_test.go` — tests 3, 4, 5.
- Extend `internal/handlers/preferences_test.go` — tests 1–4.
- Run: `go build ./... && go vet ./...` and `go test ./internal/userprefs/... ./internal/handlers/...`.
- Run `make test-go` (catches Postgres-store integration tests if any reference the user_preferences schema).

**Step 3 — Frontend store + load/save.**
- Edit `web/static/js/state.js` — new state fields.
- Edit `web/static/js/preferences.js` — extend `loadPreferences`, add two save functions.
- Run `make test-js` — extend existing preferences tests if present.
- Run `make local-fresh` to rebuild with new assets.

**Step 4 — Stats rendering refactor + customize panel.**
- Edit `web/static/js/stats.js` — add `STATS_SECTIONS`, `resolveStatsLayout`, `SECTION_LABELS`; refactor `renderStatsPage` to use the registry; add `renderCustomizePanel`.
- Verify the stats page looks identical to today with no preference changes (open `/stats` — all 9 sections in the same order).
- Add JS unit tests for `resolveStatsLayout` and the section parity test (test 7 from JS tests list).
- Run `make test-js`.

**Step 5 — Event wiring + drag/drop + hide/show.**
- Edit `web/static/js/app.js` — wire `toggle-customize-stats`, `toggle-stats-section`, and HTML5 DnD handlers.
- Add `web/static/css/app.css` styles.
- Run `make local-fresh` and click through the customize panel by hand to sanity-check.

**Step 6 — E2E tests.**
- Create `tests/e2e/stats-customize.spec.js` with tests 1–6 from E2E list.
- Run `make local-fresh && make e2e`.
- All existing stats E2E specs (`stats-tab`, `stats-top-chores`, `stats-feeding-gaps`, `stats-busy-hours-filter`, `stats-timezone`) must still pass — they assert hard-coded section order sometimes (e.g. `toHaveText` on the first card). If any existing spec asserts layout that this change breaks, update the spec only if the assertion was incidental; if the assertion was load-bearing, reconsider the design.

**Step 7 — iOS implementation.**
- Apply the iOS implementation steps (1–6) from the iOS section above — extend models, request models, state, data loader; refactor `StatsView.body`; create `StatsCustomizeView`.
- Run the iOS test commands documented in `ios/AGENTS.md` (`xcodebuild test` against the iOS simulator).
- Extend `ModelDecodingTests.testDecodeUserPreferences` and add the missing-fields test.
- Add the XCUITest(s) for the customize flow (or the algorithm unit test if XCUITest proves impractical in the time available).

**Step 8 — Parity check + pre-push.**
- Run `bash scripts/check-parity.sh` — confirm the new "Stats section customization" row exists and is marked `Done` (or `iOS pending` with justification if iOS slipped).
- Run `make lint` and `make fmt`.
- Run the full pre-push checklist from AGENTS.md: `go build ./...`, `go vet ./...`, `make test-go`, `make test-js`.
- Run `make local-fresh && make e2e` one final time.
- Run `xcodebuild test -project ios/Nabu.xcodeproj -scheme Nabu -destination 'platform=iOS Simulator,name=iPhone 16'` and confirm green.

## Files to Create / Modify

| Action | File | Purpose |
|--------|------|---------|
| Create | `docs/plans/stats-customization.md` | This plan. |
| Create | `migrations/033_stats_section_prefs.sql` | Two new JSONB columns. |
| Create | `internal/userprefs/sections.go` | Canonical section registry + helpers. |
| Modify | `internal/userprefs/store.go` | Add 2 fields to `Preferences`. |
| Modify | `internal/userprefs/memory_store.go` | Get/Upsert copy paths. |
| Modify | `internal/userprefs/postgres_store.go` | Get/Upsert SQL. |
| Modify | `internal/userprefs/service.go` | Two new `Update*` methods. |
| Modify | `internal/userprefs/userprefs_test.go` | 7 new tests. |
| Modify | `internal/handlers/preferences.go` | PATCH struct + 2 handler blocks. |
| Modify | `internal/handlers/preferences_test.go` | 4 new tests. |
| Modify | `web/static/js/state.js` | 3 new state fields. |
| Modify | `web/static/js/preferences.js` | Load + 2 save functions. |
| Modify | `web/static/js/stats.js` | Registry, resolver, refactored render, customize panel |
| Modify | `web/static/js/app.js` | Event delegation for customize + DnD. |
| Modify | `web/static/css/app.css` | Customize panel + drag styles. |
| Create | `web/static/js/tests/stats.test.js` (or extend existing) | `resolveStatsLayout` + parity tests. |
| Create | `tests/e2e/stats-customize.spec.js` | 6 E2E tests. |
| Modify | `ios/Nabu/API/Models.swift` | Add 2 fields to `UserPreferences`. |
| Modify | `ios/Nabu/API/RequestModels.swift` | Add 2 fields to `PatchUserPreferencesRequest`. |
| Modify | `ios/Nabu/App/AppState.swift` | Add 2 published state fields. |
| Modify | `ios/Nabu/API/Data/PreferencesDataLoader.swift` | Load + 2 update methods. |
| Modify | `ios/Nabu/Views/StatsView.swift` | Refactor body to use resolver; add Customize toolbar. |
| Create | `ios/Nabu/Views/StatsCustomizeView.swift` | SwiftUI list with `.onMove` + Toggles. |
| Modify | `ios/NabuTests/ModelDecodingTests.swift` | Extend `testDecodeUserPreferences` + missing-fields test. |
| Modify | `ios/NabuUITests/NabuUITests.swift` (or new test file) | XCUITest for customize flow. |
| Modify | `docs/plans/client-parity.md` | Add "Stats section customization" row. |

The route table and `BuildServer` (`internal/app/server.go`) require **no changes** — both the `GET`/`PATCH /api/preferences` routes and the `PreferencesHandler` wiring already exist.

Migration number 033 is confirmed free at time of writing (latest existing is `032_follow_up_default_true.sql`). If a PR landed in the meantime, use the next free number.

## Acceptance Criteria

A reviewer (or the user) can verify the feature by:

1. **Defaults unchanged (PWA)**: Signing in as a fresh seeded household shows the same stats layout as today, in the same order. The Baby section appears if and only if baby chores have logs.
2. **Defaults unchanged (iOS)**: Same as above on the iOS app. The hardcoded StatsView VStack order matches the canonical order before any customization.
3. **Customize toggles (PWA)**: Clicking "Customize" opens a panel listing every section. Closing it removes the panel.
4. **Customize sheet (iOS)**: Tapping the toolbar Customize button in StatsView presents `StatsCustomizeView` as a sheet. Dismiss returns to Stats.
5. **Hide (PWA + iOS)**: Unchecking any non-overview section immediately hides it from the stats page, and the page re-renders. Refreshing / relaunching the app keeps it hidden.
6. **Show (PWA + iOS)**: Re-checking a hidden section brings it back.
7. **Reorder (PWA + iOS)**: Dragging a row in the customize panel/sheet reorders the stats sections below. The new order persists across reloads / app relaunches.
8. **Overview is locked (PWA + iOS)**: The "Overview cards" control is disabled and the section cannot be hidden.
9. **Baby as ordinary section (PWA + iOS)**: With baby data present, the Baby section can be hidden and reordered just like any other section. There is no "always-on-top" special case.
10. **New section future-proofing (PWA)**: Adding a fake key to the canonical `STATS_SECTIONS` list (in a scratch branch) and loading the page shows that section appended at the bottom, visible by default, without disturbing the user's existing custom order. (Encourage the implementer to actually try this — removing the fake key afterwards.)
11. **Backend parity**: `go test ./...` green, `make test-js` green, `make e2e` green.
12. **iOS parity**: `xcodebuild test` green; the extended `testDecodeUserPreferences` and (added) `testDecodeUserPreferencesMissingStatsFields` both pass; the XCUITest or unit test for the customize flow passes.
13. **Lint/build**: `make lint`, `go vet ./...`, `go build ./...` all clean.
14. **Parity matrix updated**: `docs/plans/client-parity.md` has a new "Stats section customization" row set to `Done`. `bash scripts/check-parity.sh` does not flag this feature as pending.
15. **PR description**: states verbatim "PWA and iOS both updated."

## Open Questions for Reviewer

None remaining — all design questions have been resolved with the user. If during implementation any of the following surface, escalate back rather than guessing:

- An existing E2E spec asserts the hard-coded section order (e.g. an `nth-child` selector). If so, decide whether to relax the assertion or treat it as a load-bearing invariant.
- The migration number 033 is already taken (a PR landed in the meantime). Use the next free number; do NOT reuse.
- HTML5 drag-and-drop proves too flaky on a particular browser. Defer to a "move up / move down" button pair as a fallback — but only after attempting the drag-drop approach, since the plan specifies it.
- The Swift `StatsView.body` refactor turns into spaghetti. If true, build the section-builder map as a computed property `var sectionBuilders: [String: () -> AnyView]` declared near the top of the struct, then iterate — this keeps the body readable.
- An iOS XCUITest for drag-reorder proves flaky on the simulator. Fall back to a focused XCTest of `resolveStatsLayout` plus a manual XCUITest of the hide/show Toggle (which is deterministic); document the drag-reorder manual test in the PR description.

## Glossary

- **Section** — a top-level card or row of cards on the `/stats` page. Identified by a stable kebab-case key.
- **Canonical order** — the order sections appear in for brand new users, defined by `STATS_SECTIONS` / `StatsSections`.
- **Layout resolution** — the merge of user prefs (`statsSectionOrder` + `statsSectionHidden`) with the canonical registry to produce the visible, ordered section list. Implemented in `resolveStatsLayout`.
- **Overview cards** — the row of 4 small summary cards (Today / This Week / Streak / Top Chore). Always visible; treated as the page header.
- **Baby section** — the section rendering time-series for "Feed Baby" and "Change Baby" chores plus a feeding-gaps scatter plot. Becomes a regular, reorderable, hideable section under this plan.
