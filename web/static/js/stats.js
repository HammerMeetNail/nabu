import { apiFetch } from "./api.js";
import { escapeHTML, localDateStr } from "./utils.js";

export async function loadOverview() {
  const { data } = await apiFetch("/api/stats/overview");
  return data;
}

export async function loadHeatmap() {
  const { data } = await apiFetch("/api/stats/heatmap");
  return data;
}

export async function loadBusyHours() {
  const { data } = await apiFetch("/api/stats/busy-hours");
  return data;
}

export async function loadChoreStats() {
  const { data } = await apiFetch("/api/stats/chores");
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

function formatHour(h) {
  if (h === 0) return "12a";
  if (h < 12) return h + "a";
  if (h === 12) return "12p";
  return (h - 12) + "p";
}

export function renderStatsPage(state) {
  const stats = state.stats || {};
  const overview = stats.overview || {};
  const leaderboard = overview.leaderboard || [];
  const streaks = overview.streaks || {};
  const breakdown = overview.breakdown || [];
  const recap = overview.recap || {};
  const heatmap = stats.heatmap || [];
  const busyHours = stats.busyHours || [];
  const choreStats = stats.choreStats || [];
  const chores = state.chores || [];
  const members = state.members || [];

  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m; });

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

  return `<div class="stats-page">
    <h2>Stats</h2>

    <div class="chart-period-toggle mt-2 mb-3">
      ${renderOverviewCards(todayCount, totalThisWeek, streaks, topChoreName, stats.leaderboardPeriod, state.user?.id)}
    </div>

    ${renderBabyCareSection(state)}

    <div class="card mb-3">
      <h3>Activity</h3>
      ${renderHeatmapGrid(heatmap)}
    </div>

    <div class="card mb-3">
      <h3>Busy Hours</h3>
      ${renderBusyHoursChart(busyHours)}
    </div>

    <div class="card mb-3">
      <h3>Leaderboard</h3>
      ${renderLeaderboardList(leaderboard, memberMap, stats.leaderboardPeriod)}
    </div>

    <div class="card mb-3">
      <h3>Categories</h3>
      ${renderCategoryBars(breakdown)}
    </div>

    <div class="card mb-3">
      <h3>Chores</h3>
      ${renderChoreStatsList(choreStats, choreMap)}
    </div>

    ${recap.totalChores > 0 ? `<div class="card mb-3">
      <h3>Weekly Recap</h3>
      <p>This week you completed <strong>${recap.totalChores}</strong> chores.</p>
      <p class="mt-1">Most active: <strong>${recap.mostActiveDay || 'N/A'}</strong></p>
    </div>` : ''}
  </div>`;
}

function renderOverviewCards(todayCount, totalThisWeek, streaks, topChoreName, period, userId) {
  const periodLabel = period === "month" ? "Month" : "Week";
  return `<div class="overview-cards">
    <div class="overview-card">
      <div class="overview-card-value">${todayCount}</div>
      <div class="overview-card-label">Today</div>
    </div>
    <div class="overview-card">
      <div class="overview-card-value">${totalThisWeek}</div>
      <div class="overview-card-label">This ${periodLabel}</div>
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
  }).join("") || '<p class="text-secondary text-center">No chores this week</p>';

  return `<ul class="stat-list">${lbItems}</ul>`;
}

function renderCategoryBars(breakdown) {
  if (!breakdown || breakdown.length === 0) {
    return '<p class="text-secondary text-center">No data yet</p>';
  }

  const barMax = Math.max(1, ...breakdown.map(b => b.count));
  const bars = breakdown.map(b => {
    const pct = (b.count / barMax) * 100;
    return `<div class="stat-bar-row mb-2">
      <span class="stat-bar-label">${b.category}</span>
      <div class="stat-bar-track"><div class="stat-bar-fill" style="width:${pct}%"></div></div>
      <span class="stat-bar-count">${b.count}</span>
    </div>`;
  }).join("");

  return bars;
}

function renderChoreStatsList(choreStats, choreMap) {
  if (!choreStats || choreStats.length === 0) {
    return '<p class="text-secondary text-center">No chore data yet</p>';
  }

  const items = choreStats.map(cs => {
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

  return items;
}

export function renderBabyCareSection(state) {
  const stats = state.stats || {};
  const babyPeriod = stats.babyPeriod || "daily";
  const babyTimeSeries = stats.babyTimeSeries || {};
  const members = state.members || [];

  const memberMap = {};
  members.forEach(m => { memberMap[m.userId] = m; });

  const feedBaby = babyTimeSeries.feedBaby;
  const changeBaby = babyTimeSeries.changeBaby;

  if (!feedBaby && !changeBaby) return "";

  const periodLabel = { daily: "Daily", weekly: "Weekly", monthly: "Monthly" };

  return `<div class="card mb-3">
    <div class="baby-care-header">
      <h3>Baby</h3>
      <div class="period-toggle" role="group" aria-label="Time period">
        ${["daily", "weekly", "monthly"].map(p => {
          const active = p === babyPeriod ? " period-toggle--active" : "";
          const label = periodLabel[p];
          return `<button class="period-toggle-btn${active}" data-action="stats-baby-period" data-period="${p}" aria-pressed="${p === babyPeriod}">${label}</button>`;
        }).join("")}
      </div>
    </div>
    <div class="baby-care-columns">
      ${feedBaby ? renderBabyColumn(feedBaby, memberMap, babyPeriod, "feed") : ""}
      ${changeBaby ? renderBabyColumn(changeBaby, memberMap, babyPeriod, "change") : ""}
    </div>
  </div>`;
}

function renderBabyColumn(ts, memberMap, period, type) {
  const isVolume = type === "feed";
  const label = isVolume ? "Feed" : "Change";
  const membersHTML = renderMemberList(ts.byMember, memberMap);
  const chartHTML = isVolume
    ? renderVolumeChart(ts.periods, period)
    : renderIndicatorChart(ts.periods, period);

  return `<div class="baby-care-column">
    <h4 class="baby-col-title">${ts.choreIcon} ${escapeHTML(ts.choreName)}</h4>
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
  const chartH = 100;
  const chartW = periods.length * 24;
  const barW = 16;
  const gap = 8;

  let bars = "";
  periods.forEach((p, i) => {
    const x = i * (barW + gap) + 10;
    const h = maxML > 0 ? (p.totalML / maxML) * (chartH - 20) : 0;
    const y = chartH - h - 1;
    const label = formatPeriodLabel(p, period);
    bars += `<rect x="${x}" y="${y}" width="${barW}" height="${h}" rx="3" fill="#EC4899" opacity="0.85">
      <title>${label}: ${p.totalML} mL</title>
    </rect>`;
  });

  return `<svg viewBox="0 0 ${chartW + 20} ${chartH}" class="baby-svg-chart" aria-label="Volume chart">
    <line x1="10" y1="${chartH - 1}" x2="${chartW + 10}" y2="${chartH - 1}" stroke="#d1d5db" stroke-width="1"/>
    ${bars}
  </svg>`;
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

  const colors = { "💩 poo": "#8B4513", "💛 pee": "#FACC15" };

  const chartH = 100;
  const chartW = periods.length * 24;
  const barW = 16;
  const gap = 8;

  const groupW = indicatorKeys.length > 0 ? barW / indicatorKeys.length : barW;
  let bars = "";
  periods.forEach((p, i) => {
    indicatorKeys.forEach((key, ki) => {
      const count = p.indicators?.[key] || 0;
      const x = i * (barW + gap) + 10 + ki * groupW;
      const h = maxCount > 0 ? (count / maxCount) * (chartH - 20) : 0;
      const y = chartH - h - 1;
      const color = colors[key] || "#6B7280";
      const label = formatPeriodLabel(p, period);
      bars += `<rect x="${x}" y="${y}" width="${Math.max(1, groupW - 1)}" height="${h}" rx="2" fill="${color}" opacity="0.85">
        <title>${label}: ${key} (${count})</title>
      </rect>`;
    });
  });

  return `<svg viewBox="0 0 ${chartW + 20} ${chartH}" class="baby-svg-chart" aria-label="Indicator chart">
    <line x1="10" y1="${chartH - 1}" x2="${chartW + 10}" y2="${chartH - 1}" stroke="#d1d5db" stroke-width="1"/>
    ${bars}
  </svg>`;
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
      <span class="stat-bar-label">${b.category}</span>
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
