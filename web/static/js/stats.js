import { apiFetch } from "./api.js";
import { escapeHTML, localDateStr } from "./utils.js";

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

export async function loadChoreStats({ start, end } = {}) {
  const params = new URLSearchParams();
  if (start) params.set("start", start);
  if (end) params.set("end", end);
  const qs = params.toString();
  const url = qs ? `/api/stats/chores?${qs}` : "/api/stats/chores";
  const { data } = await apiFetch(url);
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
    return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
  };
  return `${fmt(start)} – ${fmt(end)}`;
}

function currentWeekLabel() {
  const now = new Date();
  const day = now.getDay();
  const monday = new Date(now);
  monday.setDate(now.getDate() - ((day + 6) % 7));
  const sunday = new Date(monday);
  sunday.setDate(monday.getDate() + 6);
  const fmt = (d) => d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
  return `${fmt(monday)} – ${fmt(sunday)}`;
}

// Shared Day/Week/Month/All period toggle used by the Leaderboard and Top
// Chores sections. Mirrors the styling of the baby care Daily/Weekly/Monthly
// toggle (`renderBabyPeriodToggle`).
const STATS_PERIODS = ["day", "week", "month", "all"];
const STATS_PERIOD_LABELS = { day: "Day", week: "Week", month: "Month", all: "All" };

function renderStatsPeriodToggle(activePeriod, section) {
  const period = STATS_PERIODS.includes(activePeriod) ? activePeriod : "week";
  return `<div class="period-toggle" role="group" aria-label="Time period for ${escapeHTML(section)}">
    ${STATS_PERIODS.map(p => {
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

  const order = resolveStatsLayout(
    state.stats?.sectionOrder,
    state.stats?.sectionHidden,
  );

  const sections = {
    overview: `<div class="chart-period-toggle mt-2 mb-3">${renderOverviewCards(todayCount, totalThisWeek, streaks, topChoreName, state.user?.id)}</div>`,
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
      <h3>Categories</h3>
      ${renderWeekDateRange()}
      ${renderCategoryBars(overview.breakdown || [])}
    </div>`,
    chores: `<div class="card mb-3">
      <h3>Chores</h3>
      <div class="busy-hours-date-filters">
        <input type="date" class="busy-hours-filter" data-action="chore-stats-filter" data-filter="start"
          value="${state.stats?.choreStatsFilter?.start || state.stats?.choreStatsStart || ""}">
        <input type="date" class="busy-hours-filter" data-action="chore-stats-filter" data-filter="end"
          value="${state.stats?.choreStatsFilter?.end || state.stats?.choreStatsEnd || ""}">
      </div>
      ${renderChoreStatsList(choreStats, choreMap)}
    </div>`,
    recap: recap.totalChores > 0 ? `<div class="card mb-3">
      <h3>Weekly Recap</h3>
      <p>This week you completed <strong>${recap.totalChores}</strong> chores.</p>
      <p class="mt-1">Most active: <strong>${recap.mostActiveDay || 'N/A'}</strong></p>
    </div>` : "",
  };

  const body = order
    .map(k => sections[k])
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
    ${body}
  </div>`;
}

function renderCustomizePanel(state) {
  const hidden = new Set(state.stats?.sectionHidden || []);
  const ordered = resolveStatsLayout(state.stats?.sectionOrder, []);
  const allKeys = [...ordered, ...STATS_SECTIONS.filter(k => !ordered.includes(k))];
  const rows = allKeys.map((k) => {
    const isHidden = hidden.has(k);
    const label = SECTION_LABELS[k];
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

function heatmapColor(count, maxCount) {
  if (count === 0) return "#e8e5df";
  const intensity = maxCount > 0 ? count / maxCount : 0;
  if (intensity <= 0.25) return "#c6e48b";
  if (intensity <= 0.5) return "#7bc96f";
  if (intensity <= 0.75) return "#239a3b";
  return "#196127";
}

function renderHeatmapGrid(heatmap) {
  if (!heatmap || heatmap.length === 0) {
    return '<p class="text-secondary text-center">No activity data yet</p>';
  }

  const cellMap = {};
  heatmap.forEach(c => { cellMap[c.date] = c.count; });

  const maxCount = Math.max(0, ...Object.values(cellMap));

  // Build a GitHub-style grid: columns = weeks, rows = days (Sun-Sat)
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const dayOfWeek = today.getDay();
  const endDate = new Date(today);
  const startDate = new Date(today);
  startDate.setDate(startDate.getDate() - (dayOfWeek + 19 * 7));

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
  let html = '<div class="heatmap-grid">';
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
      const title = `${cell.date}: ${cell.count} chores`;
      html += `<span class="heatmap-cell" style="background:${heatmapColor(cell.count, maxCount)}" title="${title}"></span>`;
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
  html += '</div>';
  return html;
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
  const members = state.members || [];
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

function renderChoreStatsList(choreStats, choreMap) {
  if (!choreStats || choreStats.length === 0) {
    return '<p class="text-secondary text-center">No chore data yet</p>';
  }

  const filtered = choreStats.filter(cs => (cs.totalThisWeek || 0) > 0 || (cs.totalThisMonth || 0) > 0);

  const items = filtered.map(cs => {
    const chore = choreMap[cs.choreId];
    const icon = cs.choreIcon || (chore ? chore.icon : "✓");
    const totalThisWeek = cs.totalThisWeek || 0;
    const totalThisMonth = cs.totalThisMonth || 0;

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
        return `<div class="vol-bar-wrap"><div class="vol-bar" style="height:${h}px" title="${v.date}: ${v.totalML}mL"></div></div>`;
      }).join("");

      let avgStr = "";
      if (cs.avgVolume != null) {
        avgStr = `<span class="text-secondary">Avg ${Math.round(cs.avgVolume)}mL / feed</span>`;
      }

      detailParts.push(`<div class="chore-stat-detail">
        <span class="chore-stat-detail-label">Volume (30d)</span>
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
          <span class="chore-stat-week">${totalThisWeek}/wk</span>
          <span class="chore-stat-month">${totalThisMonth}/mo</span>
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
  const members = state.members || [];

  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m; });

  const feedBaby = babyTimeSeries.feedBaby;
  const changeBaby = babyTimeSeries.changeBaby;
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
      ${feedingGaps.length > 0 ? renderFeedingGapsColumn(feedingGaps, explainerVisible, gapsStart, gapsEnd) : ""}
    </div>
  </div>`;
}

function renderBabyPeriodToggle(activePeriod, type) {
  const periodLabel = { daily: "Daily", weekly: "Weekly", monthly: "Monthly" };
  const labelName = type === "feed" ? "Feed Baby" : "Change Baby";
  return `<div class="period-toggle" role="group" aria-label="Time period for ${labelName}">
    ${["daily", "weekly", "monthly"].map(p => {
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
      <p><strong>Dot colors:</strong><br>
        <strong>Pink</strong> = small top-off &mdash; the follow-up was &le;&nbsp;50% of the preceding feed (tiny snack).<br>
        <strong>Orange</strong> = close feed &mdash; within 3&nbsp;hours and not a clear growth spike. This includes feeds &le;&nbsp;100% of the preceding feed, or any follow-up to a pink top-off (since the chain started from an unsatisfying snack).<br>
        <strong>Blue</strong> = growing or spaced feed &mdash; either &gt;&nbsp;3&nbsp;hours apart, or closer together but the baby took more than last time and the preceding feed wasn&rsquo;t itself a top-off.</p>
    </div>
    <div class="baby-chart">${chartHTML}</div>
  </div>`;
}

function isQuickActive(dateStart, dateEnd, days) {
  if (!dateStart || !dateEnd) return days === 7;
  const endDate = new Date(dateEnd + "T00:00:00");
  const expectedStart = new Date(endDate);
  expectedStart.setDate(expectedStart.getDate() - (days - 1));
  return dateStart === expectedStart.toISOString().slice(0, 10);
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
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="#e5e7eb" stroke-width="0.5"/>`;
    const label = m === 0 ? "0" : `${m / 60}h`;
    svg += `<text x="${leftM - 4}" y="${y + 3}" text-anchor="end" font-size="9" fill="#9ca3af" font-family="system-ui, sans-serif">${label}</text>`;
  }

  const twoHY = yPos(120);
  svg += `<line x1="${leftM}" y1="${twoHY}" x2="${totalW - rightM}" y2="${twoHY}" stroke="#EF4444" stroke-width="1.5"/>`;

  for (let h = 0; h < 24; h += 3) {
    const x = xCenter(h);
    svg += `<text x="${x}" y="${topM + chartH + 12}" text-anchor="middle" font-size="8" fill="#9ca3af" font-family="system-ui, sans-serif">${formatHour(h)}</text>`;
  }

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="#d1d5db" stroke-width="1"/>`;

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
      const volLabel = `${g.precedingVolume}\u202fmL \u2192 ${g.followUpVolume}\u202fmL`;
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
      const volLabel = `${g.precedingVolume}\u202fmL \u2192 ${g.followUpVolume}\u202fmL`;
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
      const volLabel = `${g.precedingVolume}\u202fmL \u2192 ${g.followUpVolume}\u202fmL`;
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
  svg += `<text x="${leftM + 11}" y="${legendY}" font-size="8" fill="#6b7280" font-family="system-ui, sans-serif">full feed</text>`;
  svg += `<circle cx="${leftM + 80}" cy="${legendY - 2}" r="4" fill="#F97316" opacity="0.6"/>`;
  svg += `<text x="${leftM + 87}" y="${legendY}" font-size="8" fill="#6b7280" font-family="system-ui, sans-serif">close feed</text>`;
  svg += `<circle cx="${leftM + 160}" cy="${legendY - 2}" r="4" fill="#EC4899" opacity="0.6"/>`;
  svg += `<text x="${leftM + 167}" y="${legendY}" font-size="8" fill="#6b7280" font-family="system-ui, sans-serif">small top-off</text>`;

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
      <h4 class="baby-col-title">${ts.choreIcon} ${escapeHTML(ts.choreName)}</h4>
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
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="#e5e7eb" stroke-width="0.5"/>`;
    svg += `<text x="${leftM - 4}" y="${y + 4}" text-anchor="end" font-size="9" fill="#9ca3af" font-family="system-ui, sans-serif">${t}</text>`;
  });

  svg += `<text x="12" y="${topM + chartH / 2}" text-anchor="middle" font-size="9" fill="#9ca3af" font-family="system-ui, sans-serif" transform="rotate(-90, 12, ${topM + chartH / 2})">mL</text>`;

  const stackColors = { "🍼 formula": "#EC4899", "🤱 breast": "#F59E0B" };
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
      if (ml > 0) parts.push(`${escapeHTML(key)} ${ml}mL`);
    });
    const unlabeledML = p.totalML - attributedML;
    if (unlabeledML > 0) parts.push(`unlabeled ${unlabeledML}mL`);

    const valText = parts.join(", ") || (p.totalML > 0 ? `${p.totalML} mL` : "");
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
        const color = stackColors[key] || "#6B7280";
        svg += `<rect x="${x + 2}" y="${baseY - offset - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" fill="${color}" opacity="0.85"/>`;
        offset += segH;
      });
      if (unlabeledML > 0) {
        const segH = Math.round((unlabeledML / maxML) * chartH);
        svg += `<rect x="${x + 2}" y="${baseY - offset - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" rx="2" fill="#d1d5db" opacity="0.6"/>`;
      }
    } else {
      svg += `<rect x="${x + 2}" y="${baseY - barH}" width="${colW - 4}" height="${barH}" rx="2" fill="#EC4899" opacity="0.85"/>`;
    }

    svg += `</g>`;

    labelData.push({ i, valText, labelX, labelY, labelAnchor });

    const labelInt = period === "daily" ? 2 : 1;
    if (i % labelInt === 0) {
      const xl = formatXLabel(p, period);
      svg += `<text x="${x + colW / 2}" y="${topM + chartH + 13}" text-anchor="middle" font-size="8" fill="#9ca3af" font-family="system-ui, sans-serif">${xl}</text>`;
    }
  });

  labelData.forEach(d => {
    svg += `<text class="chart-bar-val" data-bar="${d.i}" x="${d.labelX}" y="${d.labelY}" text-anchor="${d.labelAnchor}" font-size="10" fill="#fff" stroke="#374151" stroke-width="1.5" paint-order="stroke fill" font-weight="700" font-family="system-ui, sans-serif">${d.valText}</text>`;
  });

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="#d1d5db" stroke-width="1"/>`;

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
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="#6b7280" font-family="system-ui, sans-serif">🍼 ${formulaTotal} total</text>`;
      lx += 72;
    }
    if (breastTotal > 0) {
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="#F59E0B" opacity="0.85"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="#6b7280" font-family="system-ui, sans-serif">🤱 ${breastTotal} total</text>`;
      lx += 72;
    }
    if (unlabeledTotalML > 0) {
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="#d1d5db" opacity="0.6"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="#9ca3af" font-family="system-ui, sans-serif">unlabeled ${unlabeledTotalML}mL</text>`;
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

  const indicatorColors = { "💩 poo": "#8B4513", "💛 pee": "#FACC15", "🍼 formula": "#EC4899", "🤱 breast": "#F59E0B" };

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
    svg += `<line x1="${leftM}" y1="${y}" x2="${totalW - rightM}" y2="${y}" stroke="#e5e7eb" stroke-width="0.5"/>`;
    svg += `<text x="${leftM - 4}" y="${y + 4}" text-anchor="end" font-size="9" fill="#9ca3af" font-family="system-ui, sans-serif">${t}</text>`;
  });

  svg += `<text x="12" y="${topM + chartH / 2}" text-anchor="middle" font-size="9" fill="#9ca3af" font-family="system-ui, sans-serif" transform="rotate(-90, 12, ${topM + chartH / 2})">count</text>`;

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
        const color = indicatorColors[key] || "#6B7280";
        svg += `<rect x="${leftM + i * colW + 2}" y="${baseY - offset - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" fill="${color}" opacity="0.85"/>`;
        offset += segH;
      });
    } else if (indicatorKeys.length === 1) {
      const key = indicatorKeys[0];
      const count = p.indicators?.[key] || 0;
      const segH = Math.round((count / maxCount) * chartH);
      const color = indicatorColors[key] || "#6B7280";
      svg += `<rect x="${leftM + i * colW + 2}" y="${baseY - segH}" width="${colW - 4}" height="${Math.max(segH, 0.5)}" rx="2" fill="${color}" opacity="0.85"/>`;
    }

    svg += `</g>`;

    ilabelData.push({ i, valText, labelX, labelY, labelAnchor });

    const labelInt = period === "daily" ? 2 : 1;
    if (i % labelInt === 0) {
      const xl = formatXLabel(p, period);
      svg += `<text x="${leftM + i * colW + colW / 2}" y="${topM + chartH + 13}" text-anchor="middle" font-size="8" fill="#9ca3af" font-family="system-ui, sans-serif">${xl}</text>`;
    }
  });

  ilabelData.forEach(d => {
    svg += `<text class="chart-bar-val" data-bar="${d.i}" x="${d.labelX}" y="${d.labelY}" text-anchor="${d.labelAnchor}" font-size="10" fill="#fff" stroke="#374151" stroke-width="1.5" paint-order="stroke fill" font-weight="700" font-family="system-ui, sans-serif">${d.valText}</text>`;
  });

  svg += `<line x1="${leftM}" y1="${topM + chartH}" x2="${totalW - rightM}" y2="${topM + chartH}" stroke="#d1d5db" stroke-width="1"/>`;

  if (indicatorKeys.length > 0) {
    const ly = totalH - legendH + 14;
    indicatorKeys.forEach((key, ki) => {
      const lx = leftM + ki * 90;
      const color = indicatorColors[key] || "#6B7280";
      const total = periods.reduce((s, p) => s + (p.indicators?.[key] || 0), 0);
      svg += `<rect x="${lx}" y="${ly - 8}" width="8" height="8" rx="2" fill="${color}" opacity="0.85"/>`;
      svg += `<text x="${lx + 11}" y="${ly}" font-size="8" fill="#6b7280" font-family="system-ui, sans-serif">${escapeHTML(key)} ${total} total</text>`;
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
  const members = state.members || [];

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
