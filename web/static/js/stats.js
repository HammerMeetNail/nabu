import { apiFetch } from "./api.js";
import { escapeHTML, localDateStr, shiftDateStr, formatVolume, mlToOz } from "./utils.js";
import { formatTimeAgo } from "./home.js";

// The stats page renders volumes in the user's preferred unit. Volumes are
// stored canonically in mL; `currentVolumeUnit` is set once at the top of
// renderStatsPage and read by the volume-display helpers below. This avoids
// threading the unit through every chart builder.
let currentVolumeUnit = "ml";

// fmtVol renders a canonical mL amount with its unit suffix ("60 mL"/"2 oz").
function fmtVol(ml) {
  return formatVolume(ml, currentVolumeUnit);
}

// volAxisTick renders a bare numeric axis tick in the active unit (no suffix).
function volAxisTick(ml) {
  if (currentVolumeUnit === "oz") {
    return String(Number(mlToOz(ml).toFixed(1))).replace(/\.0$/, "");
  }
  return String(ml);
}

// volUnitLabel is the short axis unit label.
function volUnitLabel() {
  return currentVolumeUnit === "oz" ? "oz" : "mL";
}

// Canonical section list and default order. Must match
// internal/userprefs/sections.go exactly. When you add a new section,
// append it to the END of this list.
export const STATS_SECTIONS = [
  "overview",
  "last-done",
  "baby",
  "activity",
  "busy-hours",
  "leaderboard",
  "top-chores",
  "categories",
  "chores",
  "recap",
];

const SECTION_LABELS = {
  overview: "Overview cards",
  "last-done": "Last done",
  baby: "Baby care",
  activity: "Activity (heatmap)",
  "busy-hours": "Busy hours",
  leaderboard: "Leaderboard",
  "top-chores": "Top chores",
  categories: "Categories",
  chores: "Chores",
  recap: "Weekly recap",
};

// Dynamic per-entity section keys (Phase 3/4). A section key is either a
// static canonical key (above), a per-chore analytics section "chore:<id>",
// or a user-defined widget "widget:<uuid>".
export function choreSectionKey(id) { return `chore:${id}`; }
export function widgetSectionKey(id) { return `widget:${id}`; }

export function isChoreSectionKey(k) { return /^chore:\d+$/.test(k); }
export function isWidgetSectionKey(k) { return /^widget:[A-Za-z0-9_-]{1,64}$/.test(k); }
export function isDynamicSectionKey(k) { return isChoreSectionKey(k) || isWidgetSectionKey(k); }

// choreHasAnalytics reports whether a chore is rich enough to warrant its own
// generalized analytics section: it tracks a metric or has indicator labels.
// The two dedicated baby chores are excluded because the "baby" section already
// renders them.
export function choreHasAnalytics(c) {
  if (!c) return false;
  if (c.name === "Feed Baby" || c.name === "Change Baby") return false;
  const hasMetric = c.metricType && c.metricType !== "none";
  const hasIndicators = (c.indicatorLabels || []).length > 0;
  return Boolean(hasMetric || hasIndicators);
}

// eligibleChoreSectionKeys returns the ordered list of per-chore section keys
// that should be auto-available on the stats page for the given chores.
export function eligibleChoreSectionKeys(chores) {
  return (chores || []).filter(choreHasAnalytics).map(c => choreSectionKey(c.id));
}

// resolveStatsLayout merges the user's stored order with the canonical
// registry plus any dynamic keys (per-chore/widget). Static keys not present
// in the user's order are appended; then eligible dynamic keys (passed in) are
// appended so newly-configured chores/widgets appear automatically. Hidden
// sections are excluded. Stored dynamic keys that are no longer eligible are
// dropped.
export function resolveStatsLayout(userOrder, userHidden, dynamicKeys = []) {
  const hidden = new Set(userHidden || []);
  const dynamic = new Set(dynamicKeys || []);
  const valid = (k) => STATS_SECTIONS.includes(k) || dynamic.has(k);
  const seen = new Set();
  const out = [];
  for (const k of userOrder || []) {
    if (valid(k) && !hidden.has(k) && !seen.has(k)) {
      out.push(k); seen.add(k);
    }
  }
  for (const k of STATS_SECTIONS) {
    if (!seen.has(k) && !hidden.has(k)) {
      out.push(k); seen.add(k);
    }
  }
  for (const k of dynamicKeys || []) {
    if (!seen.has(k) && !hidden.has(k)) {
      out.push(k); seen.add(k);
    }
  }
  return out;
}

export async function loadOverview() {
  const { data } = await apiFetch("/api/stats/overview");
  return data;
}

export async function loadHeatmap() {
  const { data } = await apiFetch("/api/stats/heatmap");
  return data;
}

export async function loadBusyHours({ choreId, userId, start, end } = {}) {
  const params = new URLSearchParams();
  if (choreId) params.set("choreId", choreId);
  if (userId) params.set("userId", userId);
  if (start) params.set("start", start);
  if (end) params.set("end", end);
  const qs = params.toString();
  const url = qs ? `/api/stats/busy-hours?${qs}` : "/api/stats/busy-hours";
  const { data } = await apiFetch(url);
  return data;
}

export async function loadChoreStats({ start, end, period } = {}) {
  const params = new URLSearchParams();
  if (period) {
    params.set("period", period);
  } else {
    if (start) params.set("start", start);
    if (end) params.set("end", end);
  }
  const qs = params.toString();
  const url = qs ? `/api/stats/chores?${qs}` : "/api/stats/chores";
  const { data } = await apiFetch(url);
  return data;
}

export async function loadCategoryBreakdown(period) {
  const { data } = await apiFetch(`/api/stats/breakdown?period=${period || "week"}`);
  return data;
}

export async function loadTopChores(userId, period) {
  const params = new URLSearchParams();
  if (userId) params.set("userId", userId);
  if (period) params.set("period", period);
  const qs = params.toString();
  const url = qs ? `/api/stats/top-chores?${qs}` : "/api/stats/top-chores";
  const { data } = await apiFetch(url);
  return data;
}

export async function loadLeaderboard(period) {
  const { data } = await apiFetch(`/api/stats/leaderboard?period=${period || "week"}`);
  return data;
}

export async function loadChoreTimeSeries(choreId, period) {
  const { data } = await apiFetch(
    `/api/stats/chores/${choreId}/time-series?period=${period || "daily"}`
  );
  return data;
}

// loadChoreSummary fetches a period-scoped aggregate (count/amount/duration +
// per-member split) for one chore. Powers period-correct widgets (Phase 4).
export async function loadChoreSummary(choreId, period) {
  const { data } = await apiFetch(
    `/api/stats/chores/${choreId}/summary?period=${period || "week"}`
  );
  return data;
}

export async function loadFeedingGaps(start, end) {
  const params = new URLSearchParams();
  if (start) params.set("start", start);
  if (end) params.set("end", end);
  const qs = params.toString();
  const url = `/api/stats/feeding-gaps${qs ? "?" + qs : ""}`;
  const { data } = await apiFetch(url);
  return data;
}

function formatRangeLabel(start, end) {
  if (!start || !end) return "";
  const fmt = (s) => {
    const d = new Date(s + "T00:00:00");
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  };
  return `${fmt(start)} – ${fmt(end)}`;
}

function currentWeekLabel() {
  const now = new Date();
  const day = now.getDay();
  const sunday = new Date(now);
  sunday.setDate(now.getDate() - day);
  const saturday = new Date(sunday);
  saturday.setDate(sunday.getDate() + 6);
  const fmt = (d) => d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return `${fmt(sunday)} – ${fmt(saturday)}`;
}

// Shared Day/Week/Month/All period toggle used by the Leaderboard and Top
// Chores sections. Mirrors the styling of the baby care Daily/Weekly/Monthly
// toggle (`renderBabyPeriodToggle`).
const STATS_PERIODS = ["day", "week", "month", "all"];
const STATS_PERIOD_LABELS = { day: "Day", week: "Week", month: "Month", all: "All" };

function renderStatsPeriodToggle(activePeriod, section, includeAll = true) {
  const periods = includeAll ? STATS_PERIODS : STATS_PERIODS.filter(p => p !== "all");
  const period = periods.includes(activePeriod) ? activePeriod : (includeAll ? "week" : periods[0]);
  return `<div class="period-toggle" role="group" aria-label="Time period for ${escapeHTML(section)}">
    ${periods.map(p => {
      const active = p === period ? " period-toggle--active" : "";
      const label = STATS_PERIOD_LABELS[p];
      return `<button class="period-toggle-btn${active}" data-action="stats-period" data-section="${escapeHTML(section)}" data-period="${p}" aria-pressed="${p === period}">${label}</button>`;
    }).join("")}
  </div>`;
}

function renderLeaderboardDateRange(period, start, end) {
  if (period === "all") return `<div class="stats-date-range">All time</div>`;
  const label = formatRangeLabel(start, end);
  if (!label) return "";
  return `<div class="stats-date-range">${label}</div>`;
}

function formatHour(h) {
  if (h === 0) return "12a";
  if (h < 12) return h + "a";
  if (h === 12) return "12p";
  return (h - 12) + "p";
}

export function renderStatsPage(state) {
  currentVolumeUnit = state.volumeUnit === "oz" ? "oz" : "ml";
  const stats = state.stats || {};
  const overview = stats.overview || {};
  const streaks = overview.streaks || {};
  const recap = overview.recap || {};
  const heatmap = stats.heatmap || [];
  const busyHours = stats.busyHours || [];
  const choreStats = stats.choreStats || [];
  const chores = state.chores || [];
  const members = state.members || [];

  const choreMap = {};
  chores.forEach(c => { choreMap[c.id] = c; });

  const todayCount = stats.todayCount || "-";
  const totalThisWeek = recap.totalChores || 0;

  const topChoreName = (() => {
    if (choreStats.length > 0 && choreStats[0].totalThisWeek > 0) {
      return choreStats[0].choreName;
    }
    return "-";
  })();

  const choreTimeSeries = state.stats?.choreTimeSeries || {};
  const widgets = state.stats?.widgets || [];
  const dynamicKeys = [
    ...eligibleChoreSectionKeys(chores),
    ...widgets.map(w => widgetSectionKey(w.id)),
  ];
  const order = resolveStatsLayout(
    state.stats?.sectionOrder,
    state.stats?.sectionHidden,
    dynamicKeys,
  );

  const sections = {
    overview: `<div class="chart-period-toggle mt-2 mb-3">${renderOverviewCards(todayCount, totalThisWeek, streaks, topChoreName, state.user?.id)}</div>`,
    "last-done": renderLastDoneSection(chores, state.latestLogs || {}),
    baby: renderBabyCareSection(state),
    activity: `<div class="card mb-3"><h3>Activity</h3>${renderHeatmapGrid(heatmap)}</div>`,
    "busy-hours": `<div class="card mb-3">
      <h3>Busy Hours</h3>
      ${renderBusyHoursDateRange(stats.busyHoursStart, stats.busyHoursEnd)}
      <div class="busy-hours-filters">
        <select class="busy-hours-filter" data-action="busy-hours-filter" data-filter="choreId">
          <option value="">All chores</option>
          ${chores.map(c =>
            `<option value="${c.id}"${state.stats?.busyHoursFilter?.choreId === c.id ? " selected" : ""}>${escapeHTML(c.name)}</option>`
          ).join("")}
        </select>
        <select class="busy-hours-filter" data-action="busy-hours-filter" data-filter="userId">
          <option value="">All members</option>
          ${members.map(m =>
            `<option value="${m.userId}"${state.stats?.busyHoursFilter?.userId === m.userId ? " selected" : ""}>${escapeHTML(m.displayName || m.email)}</option>`
          ).join("")}
        </select>
      </div>
      <div class="busy-hours-date-filters">
        <input type="date" class="busy-hours-filter" data-action="busy-hours-filter" data-filter="start"
          value="${state.stats?.busyHoursFilter?.start || state.stats?.busyHoursStart || ""}">
        <input type="date" class="busy-hours-filter" data-action="busy-hours-filter" data-filter="end"
          value="${state.stats?.busyHoursFilter?.end || state.stats?.busyHoursEnd || ""}">
      </div>
      ${renderBusyHoursChart(busyHours)}
    </div>`,
    leaderboard: renderLeaderboardSection(state),
    "top-chores": renderTopChoresSection(state),
    categories: `<div class="card mb-3">
      <div class="stats-section-header">
        <h3>Categories</h3>
        ${renderStatsPeriodToggle(stats.categoriesPeriod || "week", "categories", false)}
      </div>
      ${renderCategoryBars(stats.categoriesBreakdown || overview.breakdown || [])}
    </div>`,
    chores: `<div class="card mb-3">
      <div class="stats-section-header">
        <h3>Chores</h3>
        ${renderStatsPeriodToggle(stats.choreStatsPeriod || "month", "chores", false)}
      </div>
      ${renderChoreStatsList(choreStats, choreMap, stats.choreStatsPeriod || "month")}
    </div>`,
    recap: recap.totalChores > 0 ? `<div class="card mb-3">
      <h3>Weekly Recap</h3>
      <p>This week you completed <strong>${recap.totalChores}</strong> chores.</p>
      <p class="mt-1">Most active: <strong>${recap.mostActiveDay || 'N/A'}</strong></p>
    </div>` : "",
  };

  const body = order
    .map(k => {
      if (sections[k] != null) return sections[k];
      if (isChoreSectionKey(k)) {
        const id = parseInt(k.slice("chore:".length), 10);
        const chore = choreMap[id];
        if (!chore) return "";
        return renderChoreAnalyticsSection(chore, choreTimeSeries[id], members, state.stats?.choreAnalyticsPeriod?.[id]);
      }
      if (isWidgetSectionKey(k)) {
        const w = widgets.find(x => widgetSectionKey(x.id) === k);
        if (!w) return "";
        return renderWidgetSection(w, state);
      }
      return "";
    })
    .filter(html => html && html.trim().length > 0)
    .join("\n");

  return `<div class="stats-page">
    <div class="stats-header-row">
      <h2>Stats</h2>
      <button class="stats-customize-btn"
              data-action="toggle-customize-stats"
              aria-label="${state.stats?.customizeOpen ? "Close customize" : "Customize stats"}">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2" aria-hidden="true">
          <circle cx="12" cy="12" r="3"/>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
        </svg>
      </button>
    </div>
    ${state.stats?.customizeOpen ? renderCustomizePanel(state) : ""}
    ${Object.values(stats.errors || {}).some(Boolean) ? '<p role="status" class="form-error">Some charts could not be refreshed. <button type="button" class="btn btn-sm" data-action="retry-stats">Retry charts</button></p>' : ''}
    ${Object.values(stats.loading || {}).some(Boolean) ? '<p role="status" class="text-secondary">Loading charts…</p>' : ''}
    ${body}
  </div>`;
}

// sectionLabel resolves the display label for a section key, including the
// dynamic per-chore ("chore:<id>") and widget ("widget:<uuid>") keys.
export function sectionLabel(k, chores, widgets) {
  if (SECTION_LABELS[k]) return SECTION_LABELS[k];
  if (isChoreSectionKey(k)) {
    const id = parseInt(k.slice("chore:".length), 10);
    const c = (chores || []).find(x => x.id === id);
    return c ? `${c.icon} ${c.name}` : "Chore";
  }
  if (isWidgetSectionKey(k)) {
    const w = (widgets || []).find(x => widgetSectionKey(x.id) === k);
    return w ? (w.title || "Widget") : "Widget";
  }
  return k;
}

function renderCustomizePanel(state) {
  const chores = state.chores || [];
  const widgets = state.stats?.widgets || [];
  const hidden = new Set(state.stats?.sectionHidden || []);
  const dynamicKeys = [
    ...eligibleChoreSectionKeys(chores),
    ...widgets.map(w => widgetSectionKey(w.id)),
  ];
  const ordered = resolveStatsLayout(state.stats?.sectionOrder, [], dynamicKeys);
  const allKeys = [
    ...ordered,
    ...STATS_SECTIONS.filter(k => !ordered.includes(k)),
    ...dynamicKeys.filter(k => !ordered.includes(k)),
  ];
  const rows = allKeys.map((k) => {
    const isHidden = hidden.has(k);
    const label = sectionLabel(k, chores, widgets);
    return `<div class="customize-row" draggable="true" data-section="${k}">
      <span class="drag-handle" aria-hidden="true">⠿</span>
      <label class="customize-check">
        <input type="checkbox" data-action="toggle-stats-section"
               data-section="${k}" ${!isHidden ? "checked" : ""}>
        <span>${escapeHTML(label)}</span>
      </label>
    </div>`;
  }).join("");
  return `<div class="card customize-panel">
    <div class="customize-panel-header">
      <h3>Customize Stats</h3>
      <button class="customize-done-btn"
              data-action="toggle-customize-stats"
              aria-label="Done">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <polyline points="20 6 9 17 4 12"/>
        </svg>
      </button>
    </div>
    ${rows}
    <button class="btn btn-outline btn-sm mt-2" data-action="widget-add">+ Add widget</button>
  </div>`;
}

function renderOverviewCards(todayCount, totalThisWeek, streaks, topChoreName, userId) {
  return `<div class="overview-cards">
    <div class="overview-card">
      <div class="overview-card-value">${todayCount}</div>
      <div class="overview-card-label">Today</div>
    </div>
    <div class="overview-card">
      <div class="overview-card-value">${totalThisWeek}</div>
      <div class="overview-card-label">This Week</div>
    </div>
    <div class="overview-card">
      <div class="overview-card-value">${streaks.current || 0}</div>
      <div class="overview-card-label">Day Streak</div>
    </div>
    <div class="overview-card">
      <div class="overview-card-value overview-card-value--small">${escapeHTML(topChoreName)}</div>
      <div class="overview-card-label">Top Chore</div>
    </div>
  </div>`;
}

// renderLastDoneSection shows time-since-last-log per chore, most recent
// first — the single most-checked datum for the baby use case ("how long
// since the last feed?"). Data comes from latest-per-chore already in state;
// no new endpoint. Chores never logged are listed last as "never".
function renderLastDoneSection(chores, latestLogs) {
  if (!chores || chores.length === 0) {
    return `<div class="card mb-3"><h3>Last done</h3>
      <p class="text-secondary text-center">No chores yet</p></div>`;
  }
  const rows = chores
    .map(c => {
      const latest = latestLogs[c.id];
      const ts = latest?.completedAt ? new Date(latest.completedAt).getTime() : 0;
      return { chore: c, ts, ago: latest?.completedAt ? formatTimeAgo(latest.completedAt) : "" };
    })
    .sort((a, b) => b.ts - a.ts)
    .map(({ chore, ago }) => {
      const agoHTML = ago
        ? `<span class="last-done-ago">${escapeHTML(ago)}</span>`
        : `<span class="last-done-ago last-done-ago--never">never</span>`;
      return `<div class="last-done-row">
        <span class="last-done-icon" style="--chore-color:${escapeHTML(chore.color)}">${escapeHTML(chore.icon)}</span>
        <span class="last-done-name">${escapeHTML(chore.name)}</span>
        ${agoHTML}
      </div>`;
    })
    .join("");
  return `<div class="card mb-3"><h3>Last done</h3>
    <div class="last-done-list">${rows}</div>
  </div>`;
}

// Stable colors for indicator/stack labels. The four predefined baby-care
// labels keep their historical colors for continuity; any other label
// (including user-defined chore indicator labels) gets a stable color from a
// distinct palette via a hash of the label text, so two custom labels never
// collapse to the same gray. Kept in sync with the iOS `IndicatorColor`
// mapping — see docs/plans/client-parity.md.
const KNOWN_INDICATOR_COLORS = {
  "🍼 formula": "#EC4899",
  "🤱 breast": "#F59E0B",
  "💩 poo": "#8B4513",
  "💛 pee": "#FACC15",
};

const INDICATOR_PALETTE = [
  "#2E86AB", "#A23B72", "#F18F01", "#386641", "#8B5CF6",
  "#0EA5E9", "#DB2777", "#65A30D", "#D97706", "#0D9488",
  "#7C3AED", "#059669",
];

function hashLabel(s) {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (Math.imul(h, 31) + s.charCodeAt(i)) | 0;
  }
  return Math.abs(h);
}

export function colorForIndicator(label) {
  if (label == null) return INDICATOR_PALETTE[0];
  if (KNOWN_INDICATOR_COLORS[label]) return KNOWN_INDICATOR_COLORS[label];
  return INDICATOR_PALETTE[hashLabel(String(label)) % INDICATOR_PALETTE.length];
}

function heatmapColor(count, maxCount) {
  if (count === 0) return "var(--heatmap-empty)";
  const intensity = maxCount > 0 ? count / maxCount : 0;
  if (intensity <= 0.25) return "var(--heatmap-1)";
  if (intensity <= 0.5) return "var(--heatmap-2)";
  if (intensity <= 0.75) return "var(--heatmap-3)";
  return "var(--heatmap-4)";
}

function renderHeatmapGrid(heatmap) {
  if (!heatmap || heatmap.length === 0) {
    return '<p class="text-secondary text-center">No activity data yet</p>';
  }

  const cellMap = {};
  heatmap.forEach(c => { cellMap[c.date] = c.count; });

  const maxCount = Math.max(0, ...Object.values(cellMap));

  // Build a GitHub-style grid: columns = weeks, rows = days (Sun-Sat).
  // Sunday-start matches the server's week definition (internal/stats
  // wkStart) and the "This Week" range label, so heatmap columns line up
  // with leaderboard/recap weeks.
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  // Days since the most recent Sunday: JS getDay() is Sun=0..Sat=6.
  const sundayIndex = today.getDay();
  const endDate = new Date(today);
  const startDate = new Date(today);
  startDate.setDate(startDate.getDate() - (sundayIndex + 19 * 7));

  const weeks = [];
  let current = new Date(startDate);
  while (current <= endDate) {
    const week = [];
    for (let d = 0; d < 7; d++) {
      const dateStr = localDateStr(current);
      const count = cellMap[dateStr] || 0;
      week.push({ date: dateStr, count });
      current.setDate(current.getDate() + 1);
    }
    weeks.push(week);
  }

  const dayLabels = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  // Wrapper is position:relative so the tap-to-reveal tooltip (touch devices
  // can't hover a title attribute) can be positioned over the tapped cell.
  let html = '<div class="heatmap-wrap">';
  html += '<div class="heatmap-tooltip" role="status" aria-live="polite" aria-hidden="true"></div>';
  html += '<div class="heatmap-grid">';
  html += '<div class="heatmap-inner">';
  html += '<div class="heatmap-day-labels">';
  dayLabels.forEach(l => {
    html += `<span class="heatmap-day-label">${l}</span>`;
  });
  html += '</div>';
  html += '<div class="heatmap-weeks">';
  weeks.forEach((week, wi) => {
    html += '<div class="heatmap-week">';
    week.forEach((cell, di) => {
      const label = heatmapCellLabel(cell.date, cell.count);
      html += `<span class="heatmap-cell" style="background:${heatmapColor(cell.count, maxCount)}" data-action="heatmap-tap" data-date="${cell.date}" data-count="${cell.count}" role="button" tabindex="0" aria-label="${escapeHTML(label)}" title="${escapeHTML(label)}"></span>`;
    });
    html += '</div>';
  });
  html += '</div>';
  html += '</div>';
  html += '<div class="heatmap-legend">';
  html += '<span>Less</span>';
  const legendMax = Math.max(4, maxCount);
  [0, Math.ceil(legendMax * 0.25), Math.ceil(legendMax * 0.5), Math.ceil(legendMax * 0.75), legendMax].forEach(n => {
    html += `<span class="heatmap-legend-cell" style="background:${heatmapColor(n, legendMax)}"></span>`;
  });
  html += '<span>More</span>';
  html += '</div>';
  html += '</div>'; // .heatmap-grid
  html += '</div>'; // .heatmap-wrap
  return html;
}

// heatmapCellLabel builds the "Tue, Jul 1 · 3 chores" readout shown on a
// cell's tap tooltip and aria-label. Uses the device locale (no hard-coded
// en-US) per the locale-consistency cleanup.
function heatmapCellLabel(dateStr, count) {
  let datePart = dateStr;
  const d = new Date(dateStr + "T00:00:00");
  if (!isNaN(d.getTime())) {
    datePart = d.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
  }
  return `${datePart} · ${count} chore${count === 1 ? "" : "s"}`;
}

function renderBusyHoursDateRange(start, end) {
  const label = formatRangeLabel(start, end);
  if (!label) return "";
  return `<div class="stats-date-range">${label}</div>`;
}

function renderWeekDateRange() {
  return `<div class="stats-date-range">${currentWeekLabel()}</div>`;
}

function renderBusyHoursChart(busyHours) {
  if (!busyHours || busyHours.length === 0) {
    return '<p class="text-secondary text-center">No activity data yet</p>';
  }

  const maxCount = Math.max(1, ...busyHours.map(h => h.count));

  const bars = busyHours.map(h => {
    const pct = (h.count / maxCount) * 100;
    return `<div class="busy-hour-row">
      <span class="busy-hour-label">${formatHour(h.hour)}</span>
      <div class="busy-hour-track"><div class="busy-hour-fill" style="width:${pct}%"></div></div>
      <span class="busy-hour-count">${h.count}</span>
    </div>`;
  }).join("");

  return `<div class="busy-hours-chart">${bars}</div>`;
}

function renderLeaderboardList(leaderboard, memberMap, period) {
  const emptyLabel = period === "all"
    ? "No chores logged yet"
    : period === "day"
      ? "No chores today"
      : period === "month"
        ? "No chores this month"
        : "No chores this week";
  const lbItems = leaderboard.map((entry) => {
    const member = memberMap[entry.userId];
    const name = member ? (member.displayName || member.email) : `User ${entry.userId}`;
    const initial = name.charAt(0).toUpperCase();
    const color = member ? member.avatarColor : "#19323C";
    return `<li class="stat-item">
      <span class="avatar-circle-sm" style="background:${color}">${initial}</span>
      <span>${escapeHTML(name)}</span>
      <span class="text-secondary">${entry.count} chores</span>
    </li>`;
  }).join("") || `<p class="text-secondary text-center">${emptyLabel}</p>`;

  return `<ul class="stat-list">${lbItems}</ul>`;
}

function renderLeaderboardSection(state) {
  const stats = state.stats || {};
  const members = [...(state.members || []), ...(state.historicalMembers || [])];
  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m; });

  const period = stats.leaderboardPeriod || "week";
  const cache = stats.leaderboardByPeriod || {};
  const leaderboard = cache[period] ?? stats.overview?.leaderboard ?? [];
  const start = stats.leaderboardRangeByPeriod?.[period]?.start || "";
  const end = stats.leaderboardRangeByPeriod?.[period]?.end || "";

  return `<div class="card mb-3">
    <div class="stats-section-header">
      <h3>Leaderboard</h3>
      ${renderStatsPeriodToggle(period, "leaderboard")}
    </div>
    ${renderLeaderboardDateRange(period, start, end)}
    ${renderLeaderboardList(leaderboard, memberMap, period)}
  </div>`;
}

function renderCategoryBars(breakdown) {
  if (!breakdown || breakdown.length === 0) {
    return '<p class="text-secondary text-center">No data yet</p>';
  }

  const barMax = Math.max(1, ...breakdown.map(b => b.count));
  const bars = breakdown.map(b => {
    const pct = (b.count / barMax) * 100;
    return `<div class="stat-bar-row mb-2">
      <span class="stat-bar-label">${escapeHTML(b.category)}</span>
      <div class="stat-bar-track"><div class="stat-bar-fill" style="width:${pct}%"></div></div>
      <span class="stat-bar-count">${b.count}</span>
    </div>`;
  }).join("");

  return bars;
}

function renderTopChoresList(topChores, period) {
  if (!topChores || topChores.length === 0) {
    const empty = period === "all"
      ? "No chores logged yet"
      : period === "day"
        ? "No chores today"
        : period === "month"
          ? "No chores this month"
          : "No chores this week";
    return `<div class="top-chore-list"><p class="text-secondary text-center">${empty}</p></div>`;
  }

  const maxCount = Math.max(1, ...topChores.map(c => c.count));

  const rows = topChores.map((c, i) => {
    const pct = (c.count / maxCount) * 100;
    const icon = c.choreIcon || "✓";
    return `<div class="top-chore-row">
      <span class="top-chore-rank">${i + 1}</span>
      <span class="top-chore-icon">${icon}</span>
      <span class="top-chore-name">${escapeHTML(c.choreName)}</span>
      <div class="top-chore-bar-track">
        <div class="top-chore-bar-fill" style="width:${pct}%"></div>
      </div>
      <span class="top-chore-count" title="${escapeHTML(STATS_PERIOD_LABELS[period] || "Count")}">${c.count}</span>
    </div>`;
  }).join("");

  return `<div class="top-chore-list">
    ${rows}
  </div>`;
}

function renderTopChoresSection(state) {
  const stats = state.stats || {};
  const members = state.members || [];
  const period = stats.topChoresPeriod || "month";
  const topChoresUserId = stats.topChoresUserId;
  const cacheKey = `${topChoresUserId}-${period}`;
  const topChores = stats.topChoresByUserAndPeriod?.[cacheKey] || [];

  const userPills = members.map(m => {
    const active = m.userId === topChoresUserId ? " top-chore-pill--active" : "";
    const initial = (m.displayName || m.email).charAt(0).toUpperCase();
    return `<button class="top-chore-pill${active}" data-action="top-chores-user" data-user-id="${m.userId}" aria-pressed="${m.userId === topChoresUserId}">
      <span class="avatar-circle-sm" style="background:${m.avatarColor || "#19323C"}">${initial}</span>
      <span>${escapeHTML(m.displayName || m.email)}</span>
    </button>`;
  }).join("");

  return `<div class="card mb-3">
    <div class="stats-section-header">
      <h3>Top Chores</h3>
      ${renderStatsPeriodToggle(period, "top-chores")}
    </div>
    <div class="top-chore-pills" role="group" aria-label="Select user">${userPills}</div>
    ${renderTopChoresList(topChores, period)}
  </div>`;
}

function renderChoreStatsList(choreStats, choreMap, period) {
  if (!choreStats || choreStats.length === 0) {
    return '<p class="text-secondary text-center">No chore data yet</p>';
  }

  const filtered = choreStats.filter(cs => (cs.totalInRange || 0) > 0);

  const periodLabels = { day: "today", week: "this week", month: "this month" };
  const periodLabel = periodLabels[period] || period;

  const items = filtered.map(cs => {
    const chore = choreMap[cs.choreId];
    const icon = cs.choreIcon || (chore ? chore.icon : "✓");
    const count = cs.totalInRange || 0;

    let detailHTML = "";
    const detailParts = [];

    if (cs.hasIndicators && cs.indicatorCounts && Object.keys(cs.indicatorCounts).length > 0) {
      const indItems = Object.entries(cs.indicatorCounts).map(([label, count]) => {
        return `<span class="ind-tag">${escapeHTML(label)}: ${count}</span>`;
      }).join("");
      detailParts.push(`<div class="chore-stat-detail"><span class="chore-stat-detail-label">Indicators</span> ${indItems}</div>`);
    }

    if (cs.hasVolume && cs.volumeHistory && cs.volumeHistory.length > 0) {
      const maxVol = Math.max(1, ...cs.volumeHistory.map(v => v.totalML));
      const volBars = cs.volumeHistory.map(v => {
        const h = maxVol > 0 ? (v.totalML / maxVol) * 40 : 0;
        return `<div class="vol-bar-wrap"><div class="vol-bar" style="height:${h}px" title="${v.date}: ${fmtVol(v.totalML)}"></div></div>`;
      }).join("");

      let avgStr = "";
      if (cs.avgVolume != null) {
        avgStr = `<span class="text-secondary">Avg ${fmtVol(Math.round(cs.avgVolume))} / feed</span>`;
      }

      const volLabel = period === "day" ? "Volume" : period === "week" ? `Volume (${periodLabel})` : `Volume (${periodLabel})`;
      detailParts.push(`<div class="chore-stat-detail">
        <span class="chore-stat-detail-label">${volLabel}</span>
        <div class="vol-chart">${volBars}</div>
        ${avgStr}
      </div>`);
    }

    const expandable = detailParts.length > 0;
    if (expandable) {
      detailHTML = `<div class="chore-stat-details">${detailParts.join("")}</div>`;
    }

    const chevron = expandable
      ? `<svg class="chore-stat-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><polyline points="6 9 12 15 18 9"></polyline></svg>`
      : "";

    return `<details class="chore-stat-card"${expandable ? "" : " open"}>
      <summary class="chore-stat-summary">
        <span class="chore-stat-icon">${icon}</span>
        <span class="chore-stat-name">${escapeHTML(cs.choreName)}</span>
        <span class="chore-stat-counts">
          <span class="chore-stat-week">${count} ${periodLabel}</span>
        </span>
        ${chevron}
      </summary>
      ${detailHTML}
    </details>`;
  }).join("");

  return items || '<p class="text-secondary text-center">No chores logged this month</p>';
}

export function renderBabyCareSection(state) {
  const stats = state.stats || {};
  const feedBabyPeriod = stats.feedBabyPeriod || "daily";
  const changeBabyPeriod = stats.changeBabyPeriod || "daily";
  const babyTimeSeries = stats.babyTimeSeries || {};
  const members = [...(state.members || []), ...(state.historicalMembers || [])];

  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m; });

  const placeholder = name => {
    const chore = (state.chores || []).find(c => c.name === name);
    return chore ? {choreName:chore.name,choreIcon:chore.icon,periods:[],byMember:[]} : null;
  };
  const feedBaby = babyTimeSeries.feedBaby || placeholder('Feed Baby');
  const changeBaby = babyTimeSeries.changeBaby || placeholder('Change Baby');
  const feedingGaps = stats.feedingGaps || [];
  const explainerVisible = stats.feedingGapsExplainerVisible || false;
  const gapsStart = stats.feedingGapsStart || "";
  const gapsEnd = stats.feedingGapsEnd || "";

  if (!feedBaby && !changeBaby) return "";

  return `<div class="card mb-3">
    <div class="baby-care-header">
      <h3>Baby</h3>
    </div>
    <div class="baby-care-columns">
      ${feedBaby ? renderBabyColumn(feedBaby, memberMap, feedBabyPeriod, "feed") : ""}
      ${changeBaby ? renderBabyColumn(changeBaby, memberMap, changeBabyPeriod, "change") : ""}
      ${feedBaby ? renderFeedingGapsColumn(feedingGaps, explainerVisible, gapsStart, gapsEnd) : ""}
    </div>
  </div>`;
}

// choreAnalyticsGrain maps a per-chore section's day/week/month period to the
// time-series grain the endpoint understands (daily/weekly/monthly buckets),
// so the toggle picks how the chart is bucketed.
export function choreAnalyticsGrain(period) {
  switch (period) {
    case "week": return "weekly";
    case "month": return "monthly";
    default: return "daily";
  }
}

// renderChoreAnalyticsPeriodToggle renders the day/week/month segmented control
// on a per-chore analytics card, matching the other stats sections. The period
// selects the chart's bucket grain (see choreAnalyticsGrain).
function renderChoreAnalyticsPeriodToggle(chore, activePeriod) {
  const periods = [
    { value: "day", label: "Day" },
    { value: "week", label: "Week" },
    { value: "month", label: "Month" },
  ];
  return `<div class="period-toggle" role="group" aria-label="Time period for ${escapeHTML(chore.name)}">
    ${periods.map(p => {
      const on = p.value === activePeriod ? " period-toggle--active" : "";
      return `<button class="period-toggle-btn${on}" data-action="chore-analytics-period" data-chore-id="${chore.id}" data-period="${p.value}" aria-pressed="${p.value === activePeriod}">${p.label}</button>`;
    }).join("")}
  </div>`;
}

// renderChoreAnalyticsSection renders the generalized per-chore analytics
// (Phase 3): a member split plus a metric-appropriate chart, for any chore that
// tracks a metric or has indicator labels. Reuses the same chart primitives as
// the baby section. `ts` is the chore's time-series (at `period`'s grain) or
// undefined; `period` is the day/week/month selection driving the toggle.
export function renderChoreAnalyticsSection(chore, ts, members, period) {
  const memberMap = {};
  (members || []).forEach(m => { memberMap[m.userId] = m; });
  const periods = ts?.periods || [];
  const metricType = chore.metricType || "none";
  const activePeriod = period || "day";
  const grain = choreAnalyticsGrain(activePeriod);

  let chartHTML;
  if (metricType === "amount") {
    const unit = chore.metricUnit || "";
    chartHTML = renderSimpleMetricChart(periods, {
      valueFn: p => p.totalML || 0,
      unitLabel: unit,
      fmt: v => `${v}${unit ? " " + unit : ""}`,
      grain,
    });
  } else if (metricType === "duration") {
    chartHTML = renderSimpleMetricChart(periods, {
      valueFn: p => Math.round((p.totalDuration || 0) / 60),
      unitLabel: "min",
      fmt: v => `${v} min`,
      grain,
    });
  } else if ((chore.indicatorLabels || []).length > 0) {
    chartHTML = renderIndicatorChart(periods, grain);
  } else {
    chartHTML = renderSimpleMetricChart(periods, {
      valueFn: p => p.count || 0,
      unitLabel: "count",
      fmt: v => `${v}`,
      grain,
    });
  }

  return `<div class="card mb-3">
    <div class="baby-col-header">
      <h3 class="baby-col-title">${chore.icon} ${escapeHTML(chore.name)}</h3>
      ${renderChoreAnalyticsPeriodToggle(chore, activePeriod)}
    </div>
    ${renderMemberList(ts?.byMember, memberMap)}
    <div class="baby-chart">${chartHTML}</div>
  </div>`;
}

// ─── User-defined widgets (Phase 4) ─────────────────────────────────────────

// widgetGrain resolves the time-series grain a widget's data is fetched at,
// derived from its period so month/all pull enough history: day/week use the
// daily series (14 days), month/all use the monthly series (6 months).
export function widgetGrain(widget) {
  const p = widget?.period;
  return (p === "month" || p === "all") ? "monthly" : "daily";
}

// widgetMetricValue extracts the numeric value for a widget's chosen metric
// from a bucket/summary carrying totalML/totalDuration/count. Amount uses the
// stored total; duration is in minutes; everything else is the occurrence count.
function widgetMetricValue(src, metric) {
  switch (metric) {
    case "amount": return src.totalML || 0;
    case "duration": return Math.round((src.totalDuration || 0) / 60);
    default: return src.count || 0;
  }
}

function widgetMetricUnit(widget, src) {
  switch (widget.metric) {
    case "amount": return src?.metricUnit || "";
    case "duration": return "min";
    default: return "";
  }
}

// renderWidgetPeriodToggle renders a day/week/month segmented control on a
// widget card (matching the other stats sections). last-done has no period.
function renderWidgetPeriodToggle(widget) {
  const active = widget.period || "week";
  const periods = [
    { value: "day", label: "Day" },
    { value: "week", label: "Week" },
    { value: "month", label: "Month" },
  ];
  return `<div class="period-toggle" role="group" aria-label="Widget period">
    ${periods.map(p => {
      const on = p.value === active ? " period-toggle--active" : "";
      return `<button class="period-toggle-btn${on}" data-action="widget-period" data-widget-id="${escapeHTML(widget.id)}" data-period="${p.value}" aria-pressed="${p.value === active}">${p.label}</button>`;
    }).join("")}
  </div>`;
}

// renderWidgetSection renders one user-defined widget. Data comes from
// state.stats.widgetData[widget.id] (an array of per-chore time-series loaded by
// app.js) plus state.latestLogs for the last-done type. The widget title is
// always escaped — widgets carry no markup.
export function renderWidgetSection(widget, state) {
  const title = escapeHTML(widget.title || "Widget");
  const data = (state.stats?.widgetData && state.stats.widgetData[widget.id]) || [];
  const chores = state.chores || [];
  const members = [...(state.members || []), ...(state.historicalMembers || [])];
  const choreMap = {};
  chores.forEach(c => { choreMap[c.id] = c; });

  let bodyHTML = "";
  if (widget.type === "last-done") {
    const latest = state.latestLogs || {};
    const rows = (widget.choreIds || []).map(id => {
      const c = choreMap[id];
      if (!c) return "";
      const l = latest[id];
      const ago = l?.completedAt ? formatTimeAgo(l.completedAt) : "";
      const agoHTML = ago
        ? `<span class="last-done-ago">${escapeHTML(ago)}</span>`
        : `<span class="last-done-ago last-done-ago--never">never</span>`;
      return `<div class="last-done-row">
        <span class="last-done-icon" style="--chore-color:${escapeHTML(c.color)}">${escapeHTML(c.icon)}</span>
        <span class="last-done-name">${escapeHTML(c.name)}</span>
        ${agoHTML}
      </div>`;
    }).join("");
    bodyHTML = `<div class="last-done-list">${rows || '<p class="text-secondary text-center">No chores</p>'}</div>`;
  } else if (widget.type === "member-split") {
    const memberMap = {};
    members.forEach(m => { memberMap[m.userId] = m; });
    // Period-scoped byMember comes from the summary endpoint.
    const merged = {};
    data.forEach(d => (d.summary?.byMember || []).forEach(e => { merged[e.userId] = (merged[e.userId] || 0) + e.count; }));
    const byMember = Object.entries(merged)
      .map(([userId, count]) => ({ userId: parseInt(userId, 10), count }))
      .sort((a, b) => b.count - a.count);
    bodyHTML = renderMemberList(byMember, memberMap);
  } else if (widget.type === "timeseries") {
    const ts = data[0]?.ts;
    const periods = ts?.periods || [];
    const unit = widgetMetricUnit(widget, ts);
    bodyHTML = renderSimpleMetricChart(periods, {
      valueFn: p => widgetMetricValue(p, widget.metric),
      unitLabel: unit || (widget.metric === "count" ? "count" : ""),
      fmt: v => `${v}${unit ? " " + unit : ""}`,
    });
  } else {
    // "total" (and any other type) → a big-number, period-scoped aggregate from
    // the summary endpoint (the server bounds it to the widget's period).
    let total = 0;
    data.forEach(d => { if (d.summary) total += widgetMetricValue(d.summary, widget.metric); });
    const unit = data.length ? widgetMetricUnit(widget, data[0].summary) : "";
    bodyHTML = `<div class="widget-big-number">${total}${unit ? ` <span class="widget-big-unit">${escapeHTML(unit)}</span>` : ""}</div>`;
  }

  // Period toggle for the period-scoped types (last-done has no period). It
  // lives in the card header — sized to content next to the remove button —
  // so it reads as the same compact segmented control as the other sections.
  const periodToggle = widget.type === "last-done" ? "" : renderWidgetPeriodToggle(widget);

  return `<div class="card mb-3 widget-card">
    <div class="widget-card-header">
      <h3>${title}</h3>
      <div class="widget-card-header-actions">
        ${periodToggle}
        <button class="widget-remove-btn" data-action="widget-remove" data-widget-id="${escapeHTML(widget.id)}" aria-label="Remove widget">×</button>
      </div>
    </div>
    ${bodyHTML}
  </div>`;
}

// renderWidgetWizard renders the "Add widget" bottom sheet. `draft` holds the
// in-progress selection.
export function renderWidgetWizard(state, draft) {
  const chores = state.chores || [];
  const d = draft || {};
  const selChores = new Set(d.choreIds || []);
  const type = d.type || "total";
  const metric = d.metric || "count";
  // Period is not chosen at create time — new widgets default to "week" and
  // expose a day/week/month toggle on the rendered card (like other sections).

  const presentations = [
    { value: "total", label: "Big number" },
    { value: "timeseries", label: "Bar chart" },
    { value: "member-split", label: "Member split" },
    { value: "last-done", label: "Last done" },
  ];
  const metrics = [
    { value: "count", label: "Count" },
    { value: "amount", label: "Amount" },
    { value: "duration", label: "Duration" },
  ];

  const choreChecks = chores.map(c =>
    `<label class="widget-chore-check">
      <input type="checkbox" data-action="widget-draft-chore" data-chore-id="${c.id}" ${selChores.has(c.id) ? "checked" : ""}>
      <span>${escapeHTML(c.icon)} ${escapeHTML(c.name)}</span>
    </label>`
  ).join("");

  const opt = (arr, sel) => arr.map(o => `<option value="${o.value}"${o.value === sel ? " selected" : ""}>${escapeHTML(o.label)}</option>`).join("");

  return `<div class="bottom-sheet widget-wizard-sheet">
    <div class="sheet-handle"></div>
    <div class="sheet-title">Add widget</div>

    <div class="chore-edit-field">
      <label class="chore-edit-label" for="widget-title">Name</label>
      <input id="widget-title" type="text" class="input" maxlength="60" placeholder="e.g. Bottles this week" value="${escapeHTML(d.title || "")}" />
    </div>

    <div class="chore-edit-field">
      <label class="chore-edit-label">Chores</label>
      <div class="widget-chore-list">${choreChecks || '<p class="text-secondary">No chores yet</p>'}</div>
    </div>

    <div class="chore-edit-field">
      <label class="chore-edit-label" for="widget-presentation">Show as</label>
      <select id="widget-presentation" class="input" data-action="widget-draft-field" data-field="type">${opt(presentations, type)}</select>
    </div>

    <div class="chore-edit-field">
      <label class="chore-edit-label" for="widget-metric">Value</label>
      <select id="widget-metric" class="input" data-action="widget-draft-field" data-field="metric">${opt(metrics, metric)}</select>
    </div>

    <div class="chore-sheet-footer">
      <div class="chore-sheet-footer-left"></div>
      <div class="chore-sheet-footer-right">
        <button type="button" class="btn btn-outline" data-action="close-sheet">Cancel</button>
        <button type="button" class="btn btn-primary" data-action="widget-save">Add</button>
      </div>
    </div>
  </div>`;
}

// renderSimpleMetricChart draws a single-series vertical bar chart over daily
// period buckets. Used for generic amount/duration/count per-chore analytics.
function renderSimpleMetricChart(periods, opts) {
  if (!periods || periods.length === 0) {
    return '<p class="text-secondary text-sm text-center mt-2">No data</p>';
  }
  const valueFn = opts.valueFn;
  const fmt = opts.fmt || (v => String(v));
  const grain = opts.grain || "daily";
  const values = periods.map(valueFn);
  const maxV = Math.max(1, ...values);

  const leftM = 38, rightM = 6, topM = 8, bottomM = 30, chartH = 120, colW = 22;
  const totalW = leftM + periods.length * colW + rightM;
  const totalH = topM + chartH + bottomM;

  const step = niceAxisStep(maxV);
  const ticks = [];
  for (let v = 0; v <= maxV + step / 2; v += step) ticks.push(v);

  let svg = `<svg viewBox="0 0 ${totalW} ${totalH}" class="baby-svg-chart" role="img" aria-label="${escapeHTML(opts.unitLabel || "value")} chart">`;
  ticks.forEach(t => {
    const y = topM + chartH - Math.round((t / maxV) * chartH);
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="var(--chart-grid)" stroke-width="0.5"/>`;
    svg += `<text x="${leftM - 4}" y="${y + 4}" text-anchor="end" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif">${t}</text>`;
  });
  if (opts.unitLabel) {
    svg += `<text x="12" y="${topM + chartH / 2}" text-anchor="middle" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif" transform="rotate(-90, 12, ${topM + chartH / 2})">${escapeHTML(opts.unitLabel)}</text>`;
  }

  periods.forEach((p, i) => {
    const v = values[i];
    const x = leftM + i * colW;
    const baseY = topM + chartH;
    const barH = v > 0 ? Math.max(Math.round((v / maxV) * chartH), 0.5) : 0;
    const label = formatPeriodLabel(p, grain);
    svg += `<g data-action="chart-tap" data-bar="${i}" role="button" aria-label="${label}: ${fmt(v)}">`;
    if (v > 0) {
      svg += `<rect x="${x + 2}" y="${baseY - barH}" width="${colW - 4}" height="${barH}" rx="2" fill="var(--chart-bar, #2E86AB)" opacity="0.85"/>`;
    }
    svg += `</g>`;
    if (i % 2 === 0) {
      svg += `<text x="${x + colW / 2}" y="${topM + chartH + 13}" text-anchor="middle" font-size="8" fill="var(--chart-label)" font-family="system-ui, sans-serif">${formatXLabel(p, grain)}</text>`;
    }
  });

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="var(--chart-axis)" stroke-width="1"/>`;
  svg += `</svg>`;
  return svg;
}

function renderBabyPeriodToggle(activePeriod, type) {
  const periodLabel = { daily: "Daily", weekly: "Weekly", monthly: "Monthly", all: "All" };
  const labelName = type === "feed" ? "Feed Baby" : "Change Baby";
  return `<div class="period-toggle" role="group" aria-label="Time period for ${labelName}">
    ${["daily", "weekly", "monthly", "all"].map(p => {
      const active = p === activePeriod ? " period-toggle--active" : "";
      const label = periodLabel[p];
      return `<button class="period-toggle-btn${active}" data-action="stats-baby-period" data-period="${p}" data-type="${type}" aria-pressed="${p === activePeriod}">${label}</button>`;
    }).join("")}
  </div>`;
}

function renderFeedingGapsColumn(gaps, explainerVisible, dateStart, dateEnd) {
  const chartHTML = renderClusterGapScatter(gaps);
  const explainerClass = explainerVisible ? " feeding-gaps-explainer--visible" : "";

  return `<div class="baby-care-column">
    <div class="feeding-gaps-header">
      <h4 class="baby-col-title">🕐 Cluster Feeding
        <button class="feeding-gaps-info-btn" data-action="toggle-feeding-gaps-info" aria-label="How to read this chart" aria-expanded="${explainerVisible}">&#9432;</button>
      </h4>
      <div class="feeding-gaps-quick">
        <button class="period-toggle-btn${isQuickActive(dateStart, dateEnd, 1) ? " period-toggle--active" : ""}" data-action="stats-feeding-gaps-quick" data-days="1">Day</button>
        <button class="period-toggle-btn${isQuickActive(dateStart, dateEnd, 7) ? " period-toggle--active" : ""}" data-action="stats-feeding-gaps-quick" data-days="7">Week</button>
        <button class="period-toggle-btn${isQuickActive(dateStart, dateEnd, 14) ? " period-toggle--active" : ""}" data-action="stats-feeding-gaps-quick" data-days="14">2 Weeks</button>
      </div>
    </div>
    <div class="feeding-gaps-dates">
      <input type="date" class="feeding-gaps-date" data-action="stats-feeding-gaps-date" data-field="start" value="${dateStart || ""}" aria-label="Start date">
      <span class="feeding-gaps-date-sep">&ndash;</span>
      <input type="date" class="feeding-gaps-date" data-action="stats-feeding-gaps-date" data-field="end" value="${dateEnd || ""}" aria-label="End date">
    </div>
    <div class="feeding-gaps-explainer${explainerClass}">
      <p><strong>Cluster feeding = 2+ feeds within 2 hours.</strong> Each dot is one inter-feed gap. The dashed&nbsp;line marks 2&nbsp;hours: dots <em>below</em> it are short gaps, dots <em>above</em> it are typical spacing.</p>
      <table class="feeding-gaps-legend">
        <tbody>
          <tr>
            <td><span class="fg-legend-dot" style="background:#EC4899"></span></td>
            <td><strong>Small top-off</strong></td>
            <td>Follow-up was &le;&nbsp;50% of the preceding feed (tiny snack).</td>
          </tr>
          <tr>
            <td><span class="fg-legend-dot" style="background:#F97316"></span></td>
            <td><strong>Close feed</strong></td>
            <td>Within 3&nbsp;hours and not a clear growth spike (&le;&nbsp;the preceding feed, or a follow-up to a top-off).</td>
          </tr>
          <tr>
            <td><span class="fg-legend-dot" style="background:#2E86AB"></span></td>
            <td><strong>Growing / spaced</strong></td>
            <td>&gt;&nbsp;3&nbsp;hours apart, or baby took more than last time.</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="baby-chart">${chartHTML}</div>
  </div>`;
}

function isQuickActive(dateStart, dateEnd, days) {
  if (!dateStart || !dateEnd) return days === 7;
  return dateStart === shiftDateStr(dateEnd, -(days - 1));
}

function renderClusterGapScatter(gaps) {
  if (!gaps || gaps.length === 0) return '<p class="text-secondary text-sm text-center mt-2">No data</p>';

  const smallTopOff = (g) => g.precedingVolume > 0 && g.followUpVolume <= g.precedingVolume * 0.5;

  const leftM = 28;
  const rightM = 6;
  const topM = 8;
  const bottomM = 28;
  const chartW = 306;
  const chartH = 120;
  const hourW = chartW / 24;
  const totalW = leftM + chartW + rightM;
  const totalH = topM + chartH + bottomM;

  const maxY = 300;
  const yPos = (mins) => topM + chartH - Math.round((Math.min(mins, maxY) / maxY) * chartH);
  const xCenter = (h) => leftM + h * hourW + hourW / 2;
  const jitter = (seed) => ((seed * 137.508) % 1 - 0.5) * hourW * 0.65;

  let svg = `<svg viewBox="0 0 ${totalW} ${totalH}" class="feeding-gaps-chart" role="img" aria-label="Cluster feeding gap scatter">`;

  for (let m = 0; m <= maxY; m += 60) {
    const y = yPos(m);
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="var(--chart-grid)" stroke-width="0.5"/>`;
    const label = m === 0 ? "0" : `${m / 60}h`;
    svg += `<text x="${leftM - 4}" y="${y + 3}" text-anchor="end" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif">${label}</text>`;
  }

  const twoHY = yPos(120);
  svg += `<line x1="${leftM}" y1="${twoHY}" x2="${totalW - rightM}" y2="${twoHY}" stroke="#EF4444" stroke-width="1.5"/>`;

  for (let h = 0; h < 24; h += 3) {
    const x = xCenter(h);
    svg += `<text x="${x}" y="${topM + chartH + 12}" text-anchor="middle" font-size="8" fill="var(--chart-label)" font-family="system-ui, sans-serif">${formatHour(h)}</text>`;
  }

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="var(--chart-axis)" stroke-width="1"/>`;

  gaps.forEach((g, i) => {
    const isPrecedingTopOff = i > 0 && smallTopOff(gaps[i - 1]);
    const seed = g.hour * 1000 + g.gapMinutes;
    const x = xCenter(g.hour) + jitter(seed);
    const y = yPos(g.gapMinutes);
    const isPink = smallTopOff(g);
    const isOrange = !isPink && g.precedingVolume > 0 && g.gapMinutes <= 180
      && (g.followUpVolume <= g.precedingVolume || isPrecedingTopOff);

    if (isPink) {
      const idx = g.hour * 1000 + g.gapMinutes;
      const dateStr = formatScatterDate(g.date);
      const volLabel = `${fmtVol(g.precedingVolume).replace(" ", "\u202f")} \u2192 ${fmtVol(g.followUpVolume).replace(" ", "\u202f")}`;
      const clampTipX = Math.min(Math.max(x, leftM + 24), totalW - rightM - 24);
      const tipY = Math.max(y - 14, topM + 10);
      svg += `<g data-action="scatter-tap" data-gap="${idx}" role="button" aria-label="${dateStr}: ${volLabel}">`;
      svg += `<circle cx="${x}" cy="${y}" r="20" fill="transparent" stroke="none"/>`;
      svg += `<circle cx="${x}" cy="${y}" r="4" fill="#EC4899" opacity="0.6"/>`;
      svg += `<text class="scatter-tooltip" data-gap="${idx}" x="${clampTipX}" y="${tipY}" text-anchor="middle" fill="var(--text)" font-family="system-ui, sans-serif" font-size="9" display="none">
        <tspan x="${clampTipX}" dy="0">${dateStr}</tspan>
        <tspan x="${clampTipX}" dy="10">${volLabel}</tspan>
      </text>`;
      svg += `</g>`;
    } else if (isOrange) {
      const idx = g.hour * 1000 + g.gapMinutes;
      const dateStr = formatScatterDate(g.date);
      const volLabel = `${fmtVol(g.precedingVolume).replace(" ", "\u202f")} \u2192 ${fmtVol(g.followUpVolume).replace(" ", "\u202f")}`;
      const clampTipX = Math.min(Math.max(x, leftM + 24), totalW - rightM - 24);
      const tipY = Math.max(y - 14, topM + 10);
      svg += `<g data-action="scatter-tap" data-gap="${idx}" role="button" aria-label="${dateStr}: ${volLabel}">`;
      svg += `<circle cx="${x}" cy="${y}" r="20" fill="transparent" stroke="none"/>`;
      svg += `<circle cx="${x}" cy="${y}" r="4" fill="#F97316" opacity="0.6"/>`;
      svg += `<text class="scatter-tooltip" data-gap="${idx}" x="${clampTipX}" y="${tipY}" text-anchor="middle" fill="var(--text)" font-family="system-ui, sans-serif" font-size="9" display="none">
        <tspan x="${clampTipX}" dy="0">${dateStr}</tspan>
        <tspan x="${clampTipX}" dy="10">${volLabel}</tspan>
      </text>`;
      svg += `</g>`;
    } else {
      const idx = g.hour * 1000 + g.gapMinutes;
      const dateStr = formatScatterDate(g.date);
      const volLabel = `${fmtVol(g.precedingVolume).replace(" ", "\u202f")} \u2192 ${fmtVol(g.followUpVolume).replace(" ", "\u202f")}`;
      const clampTipX = Math.min(Math.max(x, leftM + 24), totalW - rightM - 24);
      const tipY = Math.max(y - 14, topM + 10);
      svg += `<g data-action="scatter-tap" data-gap="${idx}" role="button" aria-label="${dateStr}: ${volLabel}">`;
      svg += `<circle cx="${x}" cy="${y}" r="20" fill="transparent" stroke="none"/>`;
      svg += `<circle cx="${x}" cy="${y}" r="4" fill="#2E86AB" opacity="0.6"/>`;
      svg += `<text class="scatter-tooltip" data-gap="${idx}" x="${clampTipX}" y="${tipY}" text-anchor="middle" fill="var(--text)" font-family="system-ui, sans-serif" font-size="9" display="none">
        <tspan x="${clampTipX}" dy="0">${dateStr}</tspan>
        <tspan x="${clampTipX}" dy="10">${volLabel}</tspan>
      </text>`;
      svg += `</g>`;
    }
  });

  const legendY = topM + chartH + 24;
  svg += `<circle cx="${leftM + 4}" cy="${legendY - 2}" r="4" fill="#2E86AB" opacity="0.6"/>`;
  svg += `<text x="${leftM + 11}" y="${legendY}" font-size="8" fill="var(--chart-label-strong)" font-family="system-ui, sans-serif">full feed</text>`;
  svg += `<circle cx="${leftM + 80}" cy="${legendY - 2}" r="4" fill="#F97316" opacity="0.6"/>`;
  svg += `<text x="${leftM + 87}" y="${legendY}" font-size="8" fill="var(--chart-label-strong)" font-family="system-ui, sans-serif">close feed</text>`;
  svg += `<circle cx="${leftM + 160}" cy="${legendY - 2}" r="4" fill="#EC4899" opacity="0.6"/>`;
  svg += `<text x="${leftM + 167}" y="${legendY}" font-size="8" fill="var(--chart-label-strong)" font-family="system-ui, sans-serif">small top-off</text>`;

  svg += `</svg>`;
  return svg;
}

function formatScatterDate(d) {
  if (!d) return "";
  const m = d.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (!m) return d;
  const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  const mo = months[parseInt(m[2], 10) - 1];
  return `${mo} ${parseInt(m[3], 10)}`;
}

function renderBabyColumn(ts, memberMap, period, type) {
  const isVolume = type === "feed";
  const membersHTML = renderMemberList(ts.byMember, memberMap);
  const chartHTML = isVolume
    ? renderVolumeChart(ts.periods, period)
    : renderIndicatorChart(ts.periods, period);

  return `<div class="baby-care-column">
    <div class="baby-col-header">
      <h4 class="baby-col-title">${escapeHTML(ts.choreIcon)} ${escapeHTML(ts.choreName)}</h4>
      ${renderBabyPeriodToggle(period, type)}
    </div>
    ${membersHTML}
    <div class="baby-chart">${chartHTML}</div>
  </div>`;
}

function renderMemberList(byMember, memberMap) {
  if (!byMember || byMember.length === 0) return '<p class="text-secondary text-sm">No data</p>';

  const maxCount = byMember[0]?.count || 1;
  return `<div class="baby-member-list">
    ${byMember.map(entry => {
      const member = memberMap[entry.userId];
      const name = member ? (member.displayName || member.email) : `User ${entry.userId}`;
      const initial = name.charAt(0).toUpperCase();
      const color = member ? member.avatarColor : "#19323C";
      const pct = maxCount > 0 ? (entry.count / maxCount) * 100 : 0;
      return `<div class="baby-member-row">
        <span class="avatar-circle-sm" style="background:${color}">${initial}</span>
        <span class="baby-member-name">${escapeHTML(name)}</span>
        <div class="baby-member-bar-track">
          <div class="baby-member-bar-fill" style="width:${pct}%"></div>
        </div>
        <span class="baby-member-count">${entry.count}</span>
      </div>`;
    }).join("")}
  </div>`;
}

function renderVolumeChart(periods, period) {
  if (!periods || periods.length === 0) return '<p class="text-secondary text-sm text-center mt-2">No data</p>';

  const maxML = Math.max(1, ...periods.map(p => p.totalML || 0));

  const leftM = 38;
  const rightM = 6;
  const topM = 8;
  const bottomM = 30;
  const legendH = 20;
  const chartH = 120;
  const colW = 22;
  const totalW = leftM + periods.length * colW + rightM;
  const totalH = topM + chartH + bottomM + legendH;

  const step = niceAxisStep(maxML);
  const ticks = [];
  for (let v = 0; v <= maxML + step / 2; v += step) ticks.push(v);

  let svg = `<svg viewBox="0 0 ${totalW} ${totalH}" class="baby-svg-chart" role="img" aria-label="Feed Baby volume chart">`;

  ticks.forEach(t => {
    const y = topM + chartH - Math.round((t / maxML) * chartH);
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="var(--chart-grid)" stroke-width="0.5"/>`;
    svg += `<text x="${leftM - 4}" y="${y + 4}" text-anchor="end" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif">${volAxisTick(t)}</text>`;
  });

  svg += `<text x="12" y="${topM + chartH / 2}" text-anchor="middle" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif" transform="rotate(-90, 12, ${topM + chartH / 2})">${volUnitLabel()}</text>`;

  const stackKeys = [];

  periods.forEach(p => {
    if (p.volumeByIndicator) {
      Object.keys(p.volumeByIndicator).forEach(k => {
        if (!stackKeys.includes(k)) stackKeys.push(k);
      });
    }
  });
  if (stackKeys.length === 0) stackKeys.push("total");

  const labelData = [];

  periods.forEach((p, i) => {
    const x = leftM + i * colW;
    const totalH_ = Math.round((p.totalML / maxML) * chartH);
    const baseY = topM + chartH;
    let offset = 0;

    const parts = [];
    let attributedML = 0;
    stackKeys.forEach(key => {
      const ml = p.volumeByIndicator?.[key] || 0;
      attributedML += ml;
      if (ml > 0) parts.push(`${escapeHTML(key)} ${fmtVol(ml)}`);
    });
    const unlabeledML = p.totalML - attributedML;
    if (unlabeledML > 0) parts.push(`unlabeled ${fmtVol(unlabeledML)}`);

    const valText = parts.join(", ") || (p.totalML > 0 ? fmtVol(p.totalML) : "");
    const fullLabel = formatPeriodLabel(p, period);
    const barH = Math.max(totalH_, 0.5);
    const estWidth = valText.length * 7;
    let labelX = x + colW / 2;
    let labelAnchor = "middle";
    if (labelX + estWidth / 2 > totalW - rightM) {
      labelAnchor = "end";
      labelX = totalW - rightM - 4;
    } else if (labelX - estWidth / 2 < leftM) {
      labelAnchor = "start";
      labelX = leftM + 4;
    }
    const labelY = Math.max(topM + 10, baseY - barH - 4);

    svg += `<g data-action="chart-tap" data-bar="${i}" role="button" aria-label="${fullLabel}: ${p.totalML} mL">`;

    if (p.totalML > 0) {
      stackKeys.forEach(key => {
        const ml = p.volumeByIndicator?.[key] || 0;
        if (ml <= 0) return;
        const segH = Math.round((ml / maxML) * chartH);
        const color = colorForIndicator(key);
        svg += `<rect x="${x + 2}" y="${baseY - offset - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" fill="${color}" opacity="0.85"/>`;
        offset += segH;
      });
      if (unlabeledML > 0) {
        const segH = Math.round((unlabeledML / maxML) * chartH);
        svg += `<rect x="${x + 2}" y="${baseY - offset - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" rx="2" fill="var(--chart-axis)" opacity="0.6"/>`;
      }
    } else {
      svg += `<rect x="${x + 2}" y="${baseY - barH}" width="${colW - 4}" height="${barH}" rx="2" fill="#EC4899" opacity="0.85"/>`;
    }

    svg += `</g>`;

    labelData.push({ i, valText, labelX, labelY, labelAnchor });

    const labelInt = period === "daily" ? 2 : 1;
    if (i % labelInt === 0) {
      const xl = formatXLabel(p, period);
      svg += `<text x="${x + colW / 2}" y="${topM + chartH + 13}" text-anchor="middle" font-size="8" fill="var(--chart-label)" font-family="system-ui, sans-serif">${xl}</text>`;
    }
  });

  labelData.forEach(d => {
    svg += `<text class="chart-bar-val" data-bar="${d.i}" x="${d.labelX}" y="${d.labelY}" text-anchor="${d.labelAnchor}" font-size="10" fill="#fff" stroke="var(--chart-bar-outline)" stroke-width="1.5" paint-order="stroke fill" font-weight="700" font-family="system-ui, sans-serif">${d.valText}</text>`;
  });

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="var(--chart-axis)" stroke-width="1"/>`;

  const formulaTotal = periods.reduce((s, p) => s + (p.indicators?.["🍼 formula"] || 0), 0);
  const breastTotal = periods.reduce((s, p) => s + (p.indicators?.["🤱 breast"] || 0), 0);
  const unlabeledTotalML = periods.reduce((s, p) => {
    let attr = 0;
    if (p.volumeByIndicator) {
      Object.values(p.volumeByIndicator).forEach(v => { attr += v; });
    }
    return s + (p.totalML || 0) - attr;
  }, 0);

  if (formulaTotal > 0 || breastTotal > 0 || unlabeledTotalML > 0) {
    const ly = totalH - legendH + 14;
    let lx = leftM;
    if (formulaTotal > 0) {
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="#EC4899" opacity="0.85"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="var(--chart-label-strong)" font-family="system-ui, sans-serif">🍼 ${formulaTotal} total</text>`;
      lx += 72;
    }
    if (breastTotal > 0) {
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="#F59E0B" opacity="0.85"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="var(--chart-label-strong)" font-family="system-ui, sans-serif">🤱 ${breastTotal} total</text>`;
      lx += 72;
    }
    if (unlabeledTotalML > 0) {
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="var(--chart-axis)" opacity="0.6"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="var(--chart-label)" font-family="system-ui, sans-serif">unlabeled ${fmtVol(unlabeledTotalML)}</text>`;
    }
  }

  svg += `</svg>`;
  return svg;
}

function renderIndicatorChart(periods, period) {
  if (!periods || periods.length === 0) return '<p class="text-secondary text-sm text-center mt-2">No data</p>';

  const indicatorKeys = [];
  const seen = new Set();
  periods.forEach(p => {
    if (p.indicators) {
      Object.keys(p.indicators).forEach(k => {
        if (!seen.has(k)) { seen.add(k); indicatorKeys.push(k); }
      });
    }
  });

  const maxCount = Math.max(1, ...periods.map(p => {
    let sum = 0;
    if (p.indicators) {
      indicatorKeys.forEach(k => { sum += p.indicators[k] || 0; });
    }
    return sum;
  }));

  const leftM = 38;
  const rightM = 6;
  const topM = 8;
  const bottomM = 30;
  const legendH = indicatorKeys.length > 0 ? 22 : 0;
  const chartH = 120;
  const colW = 22;
  const totalW = leftM + periods.length * colW + rightM;
  const totalH = topM + chartH + bottomM + legendH;

  const step = niceAxisStep(maxCount);
  const ticks = [];
  for (let v = 0; v <= maxCount + step / 2; v += step) ticks.push(v);

  let svg = `<svg viewBox="0 0 ${totalW} ${totalH}" class="baby-svg-chart" role="img" aria-label="Indicator chart">`;

  ticks.forEach(t => {
    const y = topM + chartH - Math.round((t / maxCount) * chartH);
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="var(--chart-grid)" stroke-width="0.5"/>`;
    svg += `<text x="${leftM - 4}" y="${y + 4}" text-anchor="end" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif">${t}</text>`;
  });

  svg += `<text x="12" y="${topM + chartH / 2}" text-anchor="middle" font-size="9" fill="var(--chart-label)" font-family="system-ui, sans-serif" transform="rotate(-90, 12, ${topM + chartH / 2})">count</text>`;

  const ilabelData = [];

  periods.forEach((p, i) => {
    const baseY = topM + chartH;
    let offset = 0;
    const parts = [];
    let periodTotal = 0;
    indicatorKeys.forEach(key => {
      const c = p.indicators?.[key] || 0;
      periodTotal += c;
      if (c > 0) parts.push(`${escapeHTML(key)} ${c}`);
    });
    const valText = parts.join(", ");
    const fullLabel = formatPeriodLabel(p, period);
    const totalH_ = Math.round((periodTotal / maxCount) * chartH);
    const estWidth = valText.length * 7;
    let labelX = leftM + i * colW + colW / 2;
    let labelAnchor = "middle";
    if (labelX + estWidth / 2 > totalW - rightM) {
      labelAnchor = "end";
      labelX = totalW - rightM - 4;
    } else if (labelX - estWidth / 2 < leftM) {
      labelAnchor = "start";
      labelX = leftM + 4;
    }
    const labelY = Math.max(topM + 10, baseY - totalH_ - 4);

    svg += `<g data-action="chart-tap" data-bar="${i}" role="button" aria-label="${fullLabel}: ${valText || '0'}">`;

    if (indicatorKeys.length > 1) {
      indicatorKeys.forEach(key => {
        const count = p.indicators?.[key] || 0;
        if (count <= 0) return;
        const segH = Math.round((count / maxCount) * chartH);
        const color = colorForIndicator(key);
        svg += `<rect x="${leftM + i * colW + 2}" y="${baseY - offset - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" fill="${color}" opacity="0.85"/>`;
        offset += segH;
      });
    } else if (indicatorKeys.length === 1) {
      const key = indicatorKeys[0];
      const count = p.indicators?.[key] || 0;
      const segH = Math.round((count / maxCount) * chartH);
      const color = colorForIndicator(key);
      svg += `<rect x="${leftM + i * colW + 2}" y="${baseY - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" rx="2" fill="${color}" opacity="0.85"/>`;
    }

    svg += `</g>`;

    ilabelData.push({ i, valText, labelX, labelY, labelAnchor });

    const labelInt = period === "daily" ? 2 : 1;
    if (i % labelInt === 0) {
      const xl = formatXLabel(p, period);
      svg += `<text x="${leftM + i * colW + colW / 2}" y="${topM + chartH + 13}" text-anchor="middle" font-size="8" fill="var(--chart-label)" font-family="system-ui, sans-serif">${xl}</text>`;
    }
  });

  ilabelData.forEach(d => {
    svg += `<text class="chart-bar-val" data-bar="${d.i}" x="${d.labelX}" y="${d.labelY}" text-anchor="${d.labelAnchor}" font-size="10" fill="#fff" stroke="var(--chart-bar-outline)" stroke-width="1.5" paint-order="stroke fill" font-weight="700" font-family="system-ui, sans-serif">${d.valText}</text>`;
  });

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="var(--chart-axis)" stroke-width="1"/>`;

  if (indicatorKeys.length > 0) {
    const ly = totalH - legendH + 14;
    indicatorKeys.forEach((key, ki) => {
      const lx = leftM + ki * 90;
      const color = colorForIndicator(key);
      const total = periods.reduce((s, p) => s + (p.indicators?.[key] || 0), 0);
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="${color}" opacity="0.85"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="var(--chart-label-strong)" font-family="system-ui, sans-serif">${escapeHTML(key)} ${total} total</text>`;
    });
  }

  svg += `</svg>`;
  return svg;
}

function niceAxisStep(max) {
  if (max <= 2) return 1;
  if (max <= 10) return 2;
  if (max <= 25) return 5;
  if (max <= 100) return 25;
  const magnitude = Math.pow(10, Math.floor(Math.log10(max)));
  const residual = max / magnitude;
  if (residual <= 2) return magnitude / 2;
  if (residual <= 5) return magnitude;
  return magnitude * 2;
}

function formatXLabel(p, period) {
  if (period === "daily") {
    const d = new Date(p.start + "T00:00:00");
    return d.getDate().toString();
  }
  if (period === "weekly") {
    const d = new Date(p.start + "T00:00:00");
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  }
  const d = new Date(p.start + "T00:00:00");
  return d.toLocaleDateString(undefined, { month: "short" });
}

function formatPeriodLabel(p, period) {
  if (period === "daily") {
    const d = new Date(p.start + "T00:00:00");
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  }
  if (period === "weekly") {
    const s = new Date(p.start + "T00:00:00");
    const e = new Date(p.end + "T00:00:00");
    e.setDate(e.getDate() - 1);
    return `${s.toLocaleDateString(undefined, { month: "short", day: "numeric" })}–${e.toLocaleDateString(undefined, { month: "short", day: "numeric" })}`;
  }
  const d = new Date(p.start + "T00:00:00");
  return d.toLocaleDateString(undefined, { month: "short", year: "numeric" });
}

export function renderStatsView(state) {
  const stats = state.stats || {};
  const leaderboard = stats.overview?.leaderboard || [];
  const streaks = stats.overview?.streaks || {};
  const breakdown = stats.overview?.breakdown || [];
  const recap = stats.overview?.recap || {};
  const members = [...(state.members || []), ...(state.historicalMembers || [])];

  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m; });

  const lbItems = leaderboard.map((entry, i) => {
    const member = memberMap[entry.userId];
    const name = member ? (member.displayName || member.email) : `User ${entry.userId}`;
    const initial = name.charAt(0).toUpperCase();
    const color = member ? member.avatarColor : "#19323C";
    return `<li class="stat-item">
      <span class="avatar-circle-sm" style="background:${color}">${initial}</span>
      <span>${escapeHTML(name)}</span>
      <span class="text-secondary">${entry.count} chores</span>
    </li>`;
  }).join("") || '<p class="text-secondary text-center">No data yet</p>';

  const barMax = Math.max(1, ...breakdown.map(b => b.count));
  const bars = breakdown.map(b => {
    const pct = (b.count / barMax) * 100;
    return `<div class="stat-bar-row mb-2">
      <span class="stat-bar-label">${escapeHTML(b.category)}</span>
      <div class="stat-bar-track"><div class="stat-bar-fill" style="width:${pct}%"></div></div>
      <span class="stat-bar-count">${b.count}</span>
    </div>`;
  }).join("") || '<p class="text-secondary text-center">No data yet</p>';

  return `<div class="stats-view">
    <h2>Stats</h2>

    <div class="card mb-3">
      <h3>Streaks</h3>
      <div class="streak-display mt-2">
        <div class="streak-num">${streaks.current || 0}</div>
        <div class="streak-label">day streak</div>
      </div>
      <p class="text-secondary text-center mt-1">Longest: ${streaks.longest || 0} days</p>
    </div>

    <div class="card mb-3">
      <h3>Leaderboard</h3>
      <ul class="stat-list">${lbItems}</ul>
    </div>

    <div class="card mb-3">
      <h3>Categories</h3>
      ${bars}
    </div>

    ${recap.totalChores > 0 ? `<div class="card mb-3">
      <h3>Weekly Recap</h3>
      <p>This week you completed <strong>${recap.totalChores}</strong> chores.</p>
      <p class="mt-1">Most active: <strong>${recap.mostActiveDay || 'N/A'}</strong></p>
    </div>` : ''}
  </div>`;
}
