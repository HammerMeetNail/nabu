import { getCSRFToken } from "./api.js";

function apiFetch(path) {
  const csrfToken = getCSRFToken();
  const headers = { "X-CSRF-Token": csrfToken };
  return fetch(path, { headers }).then(r => r.json());
}

function escapeHTML(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

export async function loadLeaderboard(period) {
  return apiFetch(`/api/stats/leaderboard?period=${period || "week"}`);
}

export async function loadStreaks() {
  return apiFetch("/api/stats/streaks");
}

export async function loadHeatmap() {
  return apiFetch("/api/stats/heatmap");
}

export async function loadBreakdown() {
  return apiFetch("/api/stats/breakdown");
}

export async function loadRecap() {
  return apiFetch("/api/stats/recap");
}

export function renderStatsView(state) {
  const stats = state.stats || {};
  const leaderboard = stats.leaderboard || [];
  const streaks = stats.streaks || {};
  const breakdown = stats.breakdown || [];
  const recap = stats.recap || {};

  const lbItems = leaderboard.map((entry, i) => {
    const medal = i === 0 ? "🥇" : i === 1 ? "🥈" : i === 2 ? "🥉" : "";
    return `<li class="member-item">
      <span class="avatar-circle-sm" style="background:#19323C">${medal || "#"}</span>
      <span>User ${entry.userId}</span>
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
      <h3>Streaks 🔥</h3>
      <div class="streak-display mt-2">
        <div class="streak-num">${streaks.current || 0}</div>
        <div class="streak-label">day streak</div>
      </div>
      <p class="text-secondary text-center mt-1">Longest: ${streaks.longest || 0} days</p>
    </div>

    <div class="card mb-3">
      <h3>Leaderboard</h3>
      <ul class="member-list">${lbItems}</ul>
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
