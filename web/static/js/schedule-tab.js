import { escapeHTML }      from "./utils.js";
import { todayISO }        from "./today.js";
import { shiftISO, isActiveForDayJS } from "./calendar.js";
import { recurrenceSummary } from "./schedule.js";

const UPCOMING_DAYS = 14;

export function formatLocalISODate(d) {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function fmtDayHeader(iso) {
  const d = new Date(iso + "T00:00:00");
  const today = todayISO(0);
  if (iso === today) return "Today";
  const tomorrow = shiftISO(today, 1);
  if (iso === tomorrow) return "Tomorrow";
  return d.toLocaleDateString("en-US", { weekday: "long", month: "short", day: "numeric" });
}

function fmtTime(hhmm) {
  if (!hhmm) return "";
  const [h, m] = hhmm.split(":").map(Number);
  const ampm = h >= 12 ? "PM" : "AM";
  const h12 = h % 12 || 12;
  return `${h12}:${String(m).padStart(2, "0")} ${ampm}`;
}

export function renderScheduleTab(state) {
  const chores    = state.chores    || [];
  const schedules = state.schedules || [];
  const todayLogs = state.todayLogs || [];

  const activeSchedules = schedules.filter(s => s.isActive);
  if (activeSchedules.length === 0) {
    return `<div class="schedule-view"><h2>Upcoming</h2>
      <div class="empty-state">
        <div class="empty-state-icon">📅</div>
        <div class="empty-state-title">No scheduled chores</div>
        <p>Tap the Calendar tab and add chores to time slots to create schedules.</p>
      </div></div>`;
  }

  const logMap = {};
  todayLogs.forEach(l => { logMap[l.choreId] = true; });

  const today = todayISO(0);

  const upcoming = [];
  for (let i = 0; i < UPCOMING_DAYS; i++) {
    const iso = shiftISO(today, i);
    const dayOccurrences = [];

    activeSchedules.forEach(sch => {
      if (!isActiveForDayJS(sch, iso)) return;
      const chore = chores.find(c => c.id === sch.choreId);
      if (!chore) return;
      dayOccurrences.push({
        chore,
        sch,
        iso,
        isToday: i === 0,
        isDone: !!logMap[chore.id],
      });
    });

    dayOccurrences.sort((a, b) => {
      const ta = a.sch.specificTime || "23:59";
      const tb = b.sch.specificTime || "23:59";
      return ta.localeCompare(tb);
    });

    if (dayOccurrences.length > 0) {
      upcoming.push({ iso, label: fmtDayHeader(iso), rows: dayOccurrences });
    }
  }

  if (upcoming.length === 0) {
    return `<div class="schedule-view"><h2>Upcoming</h2>
      <div class="empty-state">
        <div class="empty-state-icon">📅</div>
        <div class="empty-state-title">Nothing upcoming</div>
        <p>No active schedules for the next ${UPCOMING_DAYS} days.</p>
      </div></div>`;
  }

  const groups = upcoming.map(group => {
    const rows = group.rows.map(r => {
      const summary = recurrenceSummary(r.sch);
      const timeStr = fmtTime(r.sch.specificTime);
      const doneClass = r.isToday && r.isDone ? "sch-row--done" : "";
      const summarySuffix = r.sch.recurrenceEnd
        ? ` until ${new Date(r.sch.recurrenceEnd).toLocaleDateString("en-US", { month: "short", day: "numeric" })}`
        : "";
      return `
        <div class="sch-row ${doneClass}" style="--chore-color:${r.chore.color}">
          <div class="sch-row-main">
            <span class="sch-icon">${r.chore.icon}</span>
            <div class="sch-body">
              <div class="sch-name-row">
                <span class="sch-name">${escapeHTML(r.chore.name)}</span>
                ${timeStr ? `<span class="sch-time">${timeStr}</span>` : ""}
              </div>
              <span class="sch-meta">${summary}${summarySuffix}</span>
            </div>
          </div>
          <div class="sch-row-actions">
            ${r.isToday
              ? `<button type="button" class="btn btn-sm btn-primary sch-log-btn"
                  data-action="schedule-tap-log"
                  data-chore-id="${r.chore.id}"
                  data-schedule-id="${r.sch.id}"
                  data-date="${r.iso}"
                  aria-label="Log ${escapeHTML(r.chore.name)}">✓</button>`
              : ""}
            <button type="button" class="sch-edit-btn"
              data-action="edit-schedule"
              data-chore-id="${r.chore.id}"
              data-schedule-id="${r.sch.id}"
              aria-label="Edit schedule"
              title="Edit schedule">✎</button>
          </div>
        </div>`;
    }).join("");

    return `<div class="sch-day-header">${group.label}</div>${rows}`;
  }).join("");

  return `<div class="schedule-view">
    <h2>Upcoming</h2>
    <div class="sch-list">${groups}</div>
  </div>`;
}
