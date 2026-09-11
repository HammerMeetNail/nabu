import { createAppState, resetAuthedState } from "./state.js";
import { bootstrapIdentity, checkIdentity, changeIdentity, contextSnapshot, contextIsCurrent, resultIsCurrent, ContextChangedError, sameOrigin, storedIdentity, onExternalIdentityChange, clearBrowserIdentity } from "./browser-context.js";
import { loadActivity, prepareActivity, setActivityChoreFilter } from "./activity-data.js";
import { captureScope } from "./request-scope.js";
import { loadStatsPage, loadStatsResource, loadStatsWidgets } from "./stats-data.js";
import { loadRecentAmounts } from './recent-amounts.js';
import { formatAmount } from './metrics.js';
import { newKey } from "./device-store.js";
import { morphInnerHTML } from "./morph.js";
import { createSheetController } from "./sheets.js";
import { apiMe, apiFetch } from "./api.js";
import { loadNotificationPage, mutateNotification } from "./notification-data.js";
import { renderExports, downloadCSV } from "./exports.js";
import { replayQueue, queuedLogs, discardQueuedLog } from "./offline-queue.js";
import { escapeHTML, localDateStr, shiftDateStr, formatVolume } from "./utils.js";
import {
  loadSession,
  handleLogin,
  handleRegister,
  handleLogout,
  handleMagicLinkRequest,
  handleForgotPassword,
  handleResetPassword,
  handleChangePassword,
  renderLoginView,
  renderRegisterView,
  renderMagicLinkRequestView,
  renderMagicLinkNoticeView,
  renderVerifyEmailView,
  renderForgotPasswordView,
  renderResetPasswordView,
} from "./auth.js";
import { loadHousehold, listHouseholds, activateHousehold, createHousehold, updateHousehold, joinHousehold, createInvite, deleteInvite, leaveHousehold, removeMember, updateMemberRole, transferOwnership, renderHouseholdView, renderJoinView, generateInitials } from "./household.js";
import { loadToday, loadWeek, logChore, undoLog, updateLog, loadChores, renderHistoryView as renderHistoryPage, todayISO } from "./today.js";
import { renderStatsView, renderStatsPage, loadOverview, loadBusyHours, loadChoreStats, loadHeatmap, loadChoreTimeSeries, loadTopChores, loadLeaderboard, loadFeedingGaps, loadCategoryBreakdown, STATS_SECTIONS, choreHasAnalytics, renderWidgetWizard, widgetGrain, loadChoreSummary, choreAnalyticsGrain } from "./stats.js";
import { renderDayView, renderWeekView, isActiveForDayJS } from "./calendar.js";
import { loadSchedules, createSchedule, updateSchedule, deleteSchedule, renderPickChoreSheet, renderConfigureScheduleSheet, renderEditScheduleSheet, renderLogSheet, renderQuickLogSheet, renderRecentAmounts } from "./schedule.js";
import { loadPreferences, saveChoreOrder, saveHiddenHomeChores, saveStatsSectionOrder, saveStatsSectionHidden, sortChoresByOrder, syncTimezone, saveVolumeUnit, saveStatsWidgets, saveHideNotificationBadge } from "./preferences.js";
import { loadTimer, startTimer, stopTimer, clearFinishedTimer, elapsedSeconds, formatElapsed } from "./timer.js";
import { loadLatestLogs, renderHomeHeader, renderHomeView as renderHomeViewGrid, renderHomeManageView, renderConfirmRemoveFromHomeSheet, refreshHomeCardTimes } from "./home.js";
import { renderChoresView as renderChoresViewList, renderChoreSheet } from "./chores.js";
import { renderNotificationPanel, maybeSubscribePush, requestNotificationPermission, clearAppBadge, loadNotificationPreferences, saveNotificationPreferences, loadChoreReminderPrefs, saveChoreReminderPref } from "./notifications.js";
import { renderScheduleTab } from "./schedule-tab.js";
import { renderProfileSheet } from "./profile.js";

/**
 * Reads the current frequency settings from a bottom sheet's freq <select>
 * and weekday pills, returning a partial schedule payload.
 *
 * @param {string} prefix  "sheet" | "edit-sheet"
 * @param {string} date    ISO date string used as startDate for "once"
 */
function readSheetFreq(prefix, date) {
  const sel = document.querySelector(`#${prefix}-freq`);
  if (!sel) return {};
  const freqVal  = sel.value;
  const selOpt   = sel.options[sel.selectedIndex];
  const payload  = { frequencyType: freqVal };

  switch (freqVal) {
    case "once":
      payload.startDate = date || null;
      break;
    case "every_n_days": {
      const intervalInput = document.querySelector(`#${prefix}-interval`);
      payload.intervalDays = Math.max(2, parseInt(intervalInput?.value || "2", 10));
      payload.startDate = date || todayISO(0);
      break;
    }
    case "weekly": {
      // Read which day pills are currently toggled on.
      const sheet   = sel.closest(".bottom-sheet");
      const pills   = sheet ? [...sheet.querySelectorAll(".day-pill--on")] : [];
      payload.daysOfWeek = pills.map(p => parseInt(p.dataset.day, 10));
      if (payload.daysOfWeek.length === 0) {
        // Fall back to the option's data attribute (set at render time).
        const raw = selOpt?.dataset?.daysOfWeek;
        try { payload.daysOfWeek = raw ? JSON.parse(raw) : []; } catch { payload.daysOfWeek = []; }
      }
      break;
    }
    case "monthly_by_date":
      payload.dayOfMonth = parseInt(selOpt?.dataset?.dayOfMonth || "1", 10);
      break;
    case "yearly":
      payload.dayOfMonth   = parseInt(selOpt?.dataset?.dayOfMonth   || "1",  10);
      payload.monthOfYear  = parseInt(selOpt?.dataset?.monthOfYear  || "1",  10);
      break;
    default:
      break;
  }
  const endInput = document.querySelector(`#${prefix}-end-date`);
  if (endInput?.value) {
    payload.recurrenceEnd = endInput.value + "T00:00:00Z";
  } else if (endInput) {
    payload.recurrenceEnd = null;
  }
  return payload;
}

let state;
let sheetController = null, renderedLogDraft = null;
let notifPollTimer = null;
let _lastHHRefresh = 0;
let activeExport = null;
function startNotifPoll() {
    if (notifPollTimer) clearInterval(notifPollTimer);
    notifPollTimer = null;
    if (!state.user) return;
    notifPollTimer = setInterval(() => {
      if (!document.hidden && state.user && !sessionCheck) {
        // Keep a reader's loaded pages stable; the open panel offers Refresh.
        if (document.querySelector("#notif-panel-container")?.hidden !== false) void loadNotifData();
        if (state.household) {
          const hasJoinNotif = (state.notifications || []).some(n => n.type === 'household_joined');
          const now = Date.now();
          if (hasJoinNotif || now - _lastHHRefresh > 300000) {
            _lastHHRefresh = now;
            loadHouseholdData();
          }
        }
      }
    }, 30000);
  }


// Capture ownership when a continuation is attached, before its request settles.
function owned(callback) { return captureScope(state).guard(callback); }
async function withCurrentContext(promise, scope) {
  const result = await promise;
  if (!scope.current()) throw new ContextChangedError();
  return result;
}

let lastSWUpdateCheck = 0;
const SW_UPDATE_CHECK_MS = 60000;

function maybeCheckSWUpdate() {
  if (!window.__swReg) return;
  const now = Date.now();
  if (now - lastSWUpdateCheck < SW_UPDATE_CHECK_MS) return;
  lastSWUpdateCheck = now;
  window.__swReg.update().catch(() => {});
}

export function render(root) {
  maybeCheckSWUpdate();
  sheetController?.beforeRender();

  const route = state.currentRoute || window.location.pathname || "/";
  // Effective route for tab highlighting: unknown/auth-only paths fall back to
  // home ("/") so the "today" tab is always active when the home grid renders.
  const knownTabRoutes = ["/", "/today", "/activity", "/schedule", "/settings", "/stats"];
  const tabRoute = knownTabRoutes.includes(route) ? route : "/";
  let html = "";

  if (state.logoutPending) {
    html = `<div class="auth-card" role="status"><h1>Sign-out is unfinished</h1><p>Your data is hidden on this device. Reconnect and retry to revoke the server session.</p><button class="btn btn-primary btn-block" data-action="retry-logout" ${state.logoutBusy ? "disabled" : ""}>${state.logoutBusy ? "Signing out…" : "Retry sign-out"}</button>${state.logoutError ? `<p class="form-error">${escapeHTML(state.logoutError)}</p>` : ""}</div>`;
  } else if (state.sessionUnconfirmed) {
    html = '<div class="auth-card" role="status"><h1>Check your session</h1><p>Reconnect to confirm your account before continuing.</p><button class="btn btn-primary" data-action="retry-session">Retry</button></div>';
  } else if (state.transitioning) {
    html = `<div class="auth-card" role="status">Loading your household…</div>`;
  } else if (route.startsWith("/verify-email")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    if (token) {
      html = renderVerifyEmailView(state._emailVerificationStatus || "pending");
      if (!state._emailVerified) {
        state._emailVerified = true;
        verifyEmail(token);
      }
    } else {
      html = renderVerifyEmailView("missing");
    }
  } else if (route.startsWith("/magic-login")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    if (token) {
      html = renderMagicLinkNoticeView();
      if (!state._magicLinkConsumed) {
        state._magicLinkConsumed = true;
        consumeMagicLink(token);
      }
    } else {
      html = `<div class="auth-card"><p class="text-center">Invalid magic link.</p></div>`;
    }
  } else if (route.startsWith("/reset-password")) {
    const url = new URL(window.location.href);
    const token = url.searchParams.get("token");
    html = renderResetPasswordView(token);
  } else if (route.startsWith("/join")) {
    const code = new URL(window.location.href).searchParams.get("code") || state._pendingInviteCode;
    if (code) state._pendingInviteCode = code;
    if (!state.user) {
      html = renderJoinView(code);
    } else if (!state.household && code) {
      html = `<div class="auth-card"><p class="text-center">Joining household…</p></div>`;
      if (!state._joinAttempted) {
        state._joinAttempted = true;
        doJoinWithCode(code);
      }
    } else {
      state.currentRoute = "/";
      html = renderHomeViewWrapper();
    }
  } else if (!state.user) {
    switch (route) {
      case "/register":
        html = renderRegisterView(state.googleOAuthEnabled, state.appleSignInEnabled);
        break;
      case "/magic-link":
        html = renderMagicLinkRequestView();
        break;
      case "/forgot-password":
        html = renderForgotPasswordView();
        break;
      default:
        html = renderLoginView(state.googleOAuthEnabled, state.appleSignInEnabled);
    }
  } else {
    switch (route) {
      case "/":
      case "/today":
        html = renderHomeViewWrapper();
        break;
      case "/activity":
        html = renderActivityView();
        break;
      case "/schedule":
        html = renderScheduleView();
        break;
      case "/settings":
        html = renderSettingsView();
        break;
      case "/stats":
        html = renderStatsPageView();
        break;
      default:
        html = renderHomeViewWrapper();
    }
  }

  if (state.user && !state.transitioning && state.pendingLogs?.length) html = renderPendingWork() + html;
  html = `<div class="view-root">${html}</div>`;

  // Keep a log's live form intact through unrelated data refreshes. Inputs,
  // selected chips, focus and a frozen retry belong to this particular draft.
  const draft = ["log", "home-log"].includes(state.activeSheet) ? state.activeSheetData : null;
  const retainedSheet = draft && draft === renderedLogDraft ? root.querySelector('.bottom-sheet') : null;
  const retainedFocus = retainedSheet?.contains(document.activeElement) ? document.activeElement : null;
  if (retainedSheet) retainedSheet.replaceWith(retainedSheet.cloneNode(false));

  // Preserve the day-hour-grid-wrapper scroll position across re-renders.
  // morph.js reuses DOM nodes by position, but template whitespace differences
  // (e.g. when a sheet opens/closes) can cause it to destroy and recreate the
  // wrapper element, resetting scrollTop to 0 and triggering the auto-scroll.
  const prevWrapper = root.querySelector(".day-hour-grid-wrapper");
  const savedScroll = prevWrapper ? prevWrapper.scrollTop : -1;

  // morph.js handles incremental DOM updates well for same-structure renders
  // (e.g. toggling day pills, updating a counter), but it cannot cleanly
  // transition between a sheet-overlay and a plain view — stale nodes leak
  // into the new tree.  Detect this boundary and do a clean replace instead.
  const currentHasSheet = root.querySelector(".sheet-overlay-wrapper") !== null;
  const incomingHasSheet = html.includes("sheet-overlay-wrapper");
  if (currentHasSheet !== incomingHasSheet) {
    root.innerHTML = html;
  } else {
    morphInnerHTML(root, html);
  }
  if (retainedSheet) root.querySelector('.bottom-sheet')?.replaceWith(retainedSheet);
  if (retainedFocus?.isConnected) retainedFocus.focus({preventScroll:true});
  renderedLogDraft = draft;
  updateTabs(tabRoute);
  updateTopBar();
  renderTimerChip();
  syncLogSaveControls(root);
  sheetController?.afterRender(root);
  ensureSheetRecentAmounts(root);

  // Auto-scroll the day-hour-grid-wrapper to show the current time when it is
  // first rendered (scrollTop === 0).  This prevents the grid from always
  // starting at midnight — an hour that is rarely relevant — and ensures that
  // cards in the 9 AM–3 PM range are visible without manual scrolling.
  const wrapper = root.querySelector(".day-hour-grid-wrapper");
  if (wrapper) {
    if (savedScroll > 0) {
      // Restore the position the user was at before this re-render.
      wrapper.scrollTop = savedScroll;
    } else if (savedScroll === -1) {
      // First render (no prior wrapper): scroll to current hour.
      const h = new Date().getHours();
      const ROW_HEIGHT = 48; // must match CSS .day-hour-row height
      // Show 2 rows before the current hour; clamp between 7 AM and 11 AM so
      // that mid-morning and noon chores are always in the visible area without
      // requiring the user to scroll.
      wrapper.scrollTop = Math.min(Math.max(7, h - 2), 11) * ROW_HEIGHT;
    }
  }

  observeHistorySentinel(root);
}

// IntersectionObserver-driven infinite scroll for the history list. The
// sentinel is re-created on each render, so we (re)observe the current one
// after every render. When it scrolls into view we trigger the same
// load-more path as the button (which remains as a fallback).
let _historyObserver = null;
function observeHistorySentinel(root) {
  const sentinel = root.querySelector(".hist-sentinel");
  if (!_historyObserver && typeof IntersectionObserver !== "undefined") {
    _historyObserver = new IntersectionObserver((entries) => {
      // Only auto-load once the user has actually scrolled the list. On a
      // short page the sentinel is already on-screen at scrollTop 0; without
      // this guard we'd drain every page on first paint. Real infinite scroll
      // (scroll down → reach the sentinel) still works; the Load-more button
      // remains the affordance for unscrolled short pages.
      const scroller = document.querySelector(".app-shell");
      if (scroller && scroller.scrollTop <= 0) return;
      for (const entry of entries) {
        if (entry.isIntersecting && state.historyHasMore && !state._historyLoadingMore) {
          loadMoreHistoryPage();
        }
      }
    }, { rootMargin: "200px" });
  }
  if (_historyObserver) {
    _historyObserver.disconnect();
    if (sentinel) _historyObserver.observe(sentinel);
  }
}

async function loadMoreHistoryPage() {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  await withCurrentContext(loadActivity(state,{append:true}), contextScope);
  if (scope.current()) render(document.querySelector("#app"));
}

// Refetch the data backing the currently-active tab, then re-render. Used by
// pull-to-refresh.
async function refreshActiveTab() {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  if ((state.currentRoute || window.location.pathname) === "/settings") await withCurrentContext(loadHouseholdData(), contextScope);
  else await withCurrentContext(reloadViewData(), contextScope);
  if (scope.current()) render(document.querySelector("#app"));
}

function setupPullToRefresh() {
  const shell = document.querySelector(".app-shell");
  if (!shell) return;
  const ptr = document.createElement("div");
  ptr.className = "ptr-indicator";
  ptr.innerHTML = '<div class="ptr-spinner" aria-hidden="true"></div>';
  document.body.appendChild(ptr);

  const THRESHOLD = 70;
  const reduceMotion = typeof window.matchMedia === "function"
    && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let startY = 0, pulling = false, ready = false, refreshing = false;

  const reset = () => {
    pulling = false; ready = false;
    ptr.classList.remove("ptr-indicator--ready", "ptr-indicator--active");
    ptr.style.transition = reduceMotion ? "none" : "";
    ptr.style.transform = "translateX(-50%) translateY(-100%)";
    ptr.style.opacity = "0";
  };

  shell.addEventListener("touchstart", (e) => {
    if (refreshing || state.activeSheet) { pulling = false; return; }
    if (shell.scrollTop <= 0) {
      startY = e.touches[0].clientY;
      pulling = true; ready = false;
    } else {
      pulling = false;
    }
  }, { passive: true });

  shell.addEventListener("touchmove", (e) => {
    if (!pulling) return;
    const dy = e.touches[0].clientY - startY;
    if (dy <= 0 || shell.scrollTop > 0) { reset(); return; }
    const pull = Math.min(dy, 120);
    ready = pull >= THRESHOLD;
    ptr.style.transition = "none";
    ptr.style.transform = `translateX(-50%) translateY(${Math.min(pull - 40, 24)}px)`;
    ptr.style.opacity = String(Math.min(pull / THRESHOLD, 1));
    ptr.classList.toggle("ptr-indicator--ready", ready);
  }, { passive: true });

  shell.addEventListener("touchend", () => {
    if (!pulling) return;
    pulling = false;
    if (!ready) { reset(); return; }
    refreshing = true;
    ptr.style.transition = reduceMotion ? "none" : "";
    ptr.style.transform = "translateX(-50%) translateY(24px)";
    ptr.style.opacity = "1";
    ptr.classList.add("ptr-indicator--active");
    refreshActiveTab().finally(owned(() => { refreshing = false; reset(); }));
  });
}

function renderActivityView() {
  const chores = state.chores || [];
  if (!state.household && state.user) {
    return `<div class="card mt-3"><h2>Welcome!</h2>
      <p>Hi ${escapeHTML(state.user.email || '')}! Set up your household to get started.</p>
      <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a></div>`;
  }
  if (chores.length === 0) {
    return `<div class="today-view"><h2>Activity</h2>
    <div class="empty-state"><div class="empty-state-icon">🏠</div>
    <div class="empty-state-title">No chores set up yet</div>
    <p>Use the Home tab to add chores.</p>
    <button type="button" class="btn btn-primary" data-nav="home">Go to Home</button></div></div>`;
  }
  return renderHistoryView();
}

function renderDayNoteSheet() {
  const { date } = state.activeSheetData || {};
  const existing = (state.dayNotes || {})[date] || "";
  const label = (() => {
    const d = new Date(date + "T00:00:00");
    return isNaN(d.getTime()) ? date : d.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });
  })();
  return `<div class="bottom-sheet day-note-sheet" role="dialog" aria-modal="true" aria-label="Day note">
    <div class="sheet-handle" aria-hidden="true"></div>
    <h2 class="sheet-title">${escapeHTML(label)}</h2>
    <div class="sheet-note-row">
      <label for="day-note-input" class="field-label">Note for this day (shared)</label>
      <textarea id="day-note-input" class="text-input" rows="3" maxlength="500" placeholder="e.g. first solid food!">${escapeHTML(existing)}</textarea>
    </div>
    <button type="button" class="btn btn-primary btn-full" data-action="save-day-note" data-date="${escapeHTML(date)}">Save</button>
    <button type="button" class="btn btn-ghost btn-full sheet-cancel-btn" data-action="close-sheet">Cancel</button>
  </div>`;
}

function renderHistoryView() {
  const mainView = renderHistoryPage(state);
  if (state.activeSheet === "day-note") {
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${renderDayNoteSheet()}
    </div>`;
  }
  if (state.activeSheet === "log") {
    const { choreId, logId, date } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const log = logId ? ((state.historyLogs || []).find(l => l.id === logId) || null) : null;
      const latestLogForChore = state.latestLogs[choreId] ?? null;
      const cachedIndicatorVolumes = latestLogForChore?.indicatorVolumes ?? null;
      const cachedIndicators = latestLogForChore?.indicators ?? null;
      const sheetHTML = renderLogSheet(chore, log, date || "", state.members || [], state.user?.id, null, { showWhen: true, slotHour: state.activeSheetData?.slotHour ?? new Date().getHours(), cachedIndicators, cachedIndicatorVolumes, volumeUnit: state.volumeUnit, recentVolumes: recentVolumesForChore(chore.id) });

  return `<div class="sheet-overlay-wrapper">
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  return mainView;
}

function renderHomeViewWrapper() {
  const header = renderHomeHeader(state);
  const isManage = state.homeView === "manage";
  const mainView = isManage
    ? renderHomeManageView(state)
    : renderHomeViewGrid(state);

  if (state.activeSheet === "home-log") {
    const { choreId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const latestLogForChore = state.latestLogs[choreId] ?? null;
      const cachedIndicatorVolumes = latestLogForChore?.indicatorVolumes ?? null;
      const cachedIndicators = latestLogForChore?.indicators ?? null;
      const sheetHTML = renderLogSheet(chore, null, todayISO(0), state.members || [], state.user?.id, null, { showWhen: true, cachedIndicators, cachedIndicatorVolumes, volumeUnit: state.volumeUnit, recentVolumes: recentVolumesForChore(chore.id) });
      return `<div class="sheet-overlay-wrapper">
        ${header}
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "confirm-remove-home-chore") {
    const { choreId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const sheetHTML = renderConfirmRemoveFromHomeSheet(chore);
      return `<div class="sheet-overlay-wrapper">
        ${header}
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "chore-edit") {
    const { choreId } = state.activeSheetData || {};
    const isNew = choreId === null || choreId === undefined;
    const chore = isNew ? null : (state.chores || []).find(c => c.id === choreId);
    if (isNew || chore) {
      const _memberSelf = (state.members || []).find(m => m.userId === state.user?.id);
      const _isAdmin = _memberSelf?.role === "owner" || _memberSelf?.role === "admin";
      const sheetHTML = renderChoreSheet(isNew ? null : chore, {
        scheduleReminderEnabled: scheduleReminderTypeEnabled(),
        reminderPref: getChoreReminderPref(choreId),
        defaultLeadMinutes: state.notificationPrefs?.defaultReminderLeadMinutes ?? 10,
        isAdmin: _isAdmin,
      });
      return `<div class="sheet-overlay-wrapper">
        ${header}
        ${mainView}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  return `<div class="home-wrapper">${header}${mainView}</div>`;
}

function renderCalendarView() {
  const chores = state.chores || [];
  if (!state.household && state.user) {
    return `<div class="card mt-3"><h2>Welcome!</h2>
      <p>Hi ${escapeHTML(state.user.email || '')}! Set up your household to get started.</p>
      <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a></div>`;
  }
  if (chores.length === 0) {
    return `<div class="today-view"><h2>Today</h2>
    <div class="empty-state"><div class="empty-state-icon">🏠</div>
    <div class="empty-state-title">No chores set up yet</div>
    <p>Use the Home tab to add chores.</p></div></div>`;
  }
  const mainView = state.calendarView === "week"
    ? renderWeekView(state)
    : renderDayView(state);

  // TODO: reinstate FAB when quick-log is needed from Activity tab
  // const fab = `<button type="button" class="fab" data-action="open-quick-log" aria-label="Log a chore">+</button>`;
  const fab = "";

  if (state.activeSheet === "pick-chore") {
    const sheetHTML = renderPickChoreSheet(
      sortChoresByOrder(state.chores, state.choreOrder),
      state.activeSheetData || {},
      state.schedules || []
    );
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      ${fab}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${sheetHTML}
    </div>`;
  }
  if (state.activeSheet === "configure-schedule") {
    const { choreId, date, hour, presetTime, presetFreq } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const sheetHTML = renderConfigureScheduleSheet(chore, date, hour, presetTime, presetFreq);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "edit-schedule") {
    const { choreId, scheduleId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    const sch   = (state.schedules || []).find(s => s.id === scheduleId);
    if (chore && sch) {
      const sheetHTML = renderEditScheduleSheet(chore, sch, state.calendarDate);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "log") {
    const { choreId, logId, date } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const allLogs = state.calendarView === "week"
        ? (state.weekLogs || [])
        : (state.todayLogs || []);
      const log = logId ? (allLogs.find(l => l.id === logId) || null) : null;
      const latestLogForChore = state.latestLogs[choreId] ?? null;
      const cachedIndicatorVolumes = latestLogForChore?.indicatorVolumes ?? null;
      const cachedIndicators = latestLogForChore?.indicators ?? null;
      const sheetHTML = renderLogSheet(chore, log, date || "", state.members || [], state.user?.id, null, { showWhen: true, slotHour: state.activeSheetData?.slotHour ?? new Date().getHours(), cachedIndicators, cachedIndicatorVolumes, volumeUnit: state.volumeUnit, recentVolumes: recentVolumesForChore(chore.id) });
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "quick-log") {
    const date = state.activeSheetData?.date || "";
    const sheetHTML = renderQuickLogSheet(sortChoresByOrder(state.chores, state.choreOrder), date);
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      ${fab}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${sheetHTML}
    </div>`;
  }
  return `<div class="sheet-overlay-wrapper">${mainView}${fab}</div>`;
}

function renderScheduleView() {
  const chores = state.chores || [];
  if (!state.household && state.user) {
    return `<div class="card mt-3"><h2>Welcome!</h2>
      <p>Hi ${escapeHTML(state.user.email || '')}! Set up your household to get started.</p>
      <a class="btn btn-primary mt-2" href="#" data-nav="settings">Set Up Household</a></div>`;
  }
  if (chores.length === 0) {
    return `<div class="schedule-view"><h2>Upcoming</h2>
    <div class="empty-state"><div class="empty-state-icon">🏠</div>
    <div class="empty-state-title">No chores set up yet</div>
    <p>Use the Home tab to add chores.</p></div></div>`;
  }
  const mainView = renderScheduleTab(state);
  const fab = `<button type="button" class="fab" data-action="open-pick-chore-sheet" data-date="${todayISO(0)}" aria-label="Schedule a chore">+</button>`;

  if (state.activeSheet === "pick-chore") {
    const sheetHTML = renderPickChoreSheet(
      sortChoresByOrder(state.chores, state.choreOrder),
      state.activeSheetData || {},
      state.schedules || []
    );
    return `<div class="sheet-overlay-wrapper">
      ${mainView}
      ${fab}
      <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
      ${sheetHTML}
    </div>`;
  }
  if (state.activeSheet === "configure-schedule") {
    const { choreId, date, hour, presetTime, presetFreq } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const sheetHTML = renderConfigureScheduleSheet(chore, date, hour, presetTime, presetFreq);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "edit-schedule") {
    const { choreId, scheduleId } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    const sch   = (state.schedules || []).find(s => s.id === scheduleId);
    if (chore && sch) {
      const sheetHTML = renderEditScheduleSheet(chore, sch, state.calendarDate);
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  if (state.activeSheet === "log") {
    const { choreId, logId, date, scheduleId, slotTime } = state.activeSheetData || {};
    const chore = (state.chores || []).find(c => c.id === choreId);
    if (chore) {
      const allLogs = state.todayLogs || [];
      const log = logId ? (allLogs.find(l => l.id === logId) || null) : null;
      const latestLogForChore = state.latestLogs[choreId] ?? null;
      const cachedIndicatorVolumes = latestLogForChore?.indicatorVolumes ?? null;
      const cachedIndicators = latestLogForChore?.indicators ?? null;
      const sheetHTML = renderLogSheet(chore, log, date || "", state.members || [], state.user?.id, null, { showWhen: true, slotHour: state.activeSheetData?.slotHour ?? new Date().getHours(), scheduleId, slotTime, cachedIndicators, cachedIndicatorVolumes, volumeUnit: state.volumeUnit, recentVolumes: recentVolumesForChore(chore.id) });
      return `<div class="sheet-overlay-wrapper">
        ${mainView}
        ${fab}
        <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
        ${sheetHTML}
      </div>`;
    }
  }
  return `<div class="sheet-overlay-wrapper">${mainView}${fab}</div>`;
}

function renderSettingsView() {
  const hh = state.household;
  const user = state.user;

  const verificationSection = user && !user.emailVerified ? `
    <div class="card mt-3" style="border-left: 4px solid #F4A261;">
      <h3>Email Verification</h3>
      <p class="text-secondary">Your email <strong>${escapeHTML(user.email)}</strong> is not verified. A verification email is queued for delivery; you can request another below.</p>
      <button type="button" class="btn btn-sm btn-secondary mt-2" data-action="resend-verification">Resend verification email</button>
    </div>
  ` : "";

  // In-app account deletion (App Store guideline 5.1.1(v) requires it on the
  // native client; the PWA gets the same flow for parity). The server demands
  // the typed confirmation {"confirm":"DELETE"}; a sole owner of a
  // multi-member household gets a 409 telling them to transfer first.
  const deleteAccountSection = state.deleteAccountOpen ? `
    <div class="mt-3" data-testid="delete-account-confirm">
      <p class="text-secondary">Deleting your account permanently removes your data.
        Households where you are the only member are deleted with all their logs.
        If you are the only owner of a household with other members, transfer
        ownership (or remove the other members) first. This cannot be undone.</p>
      <div class="form-group mt-2">
        <label class="form-label" for="delete-account-input">Type DELETE to confirm</label>
        <input id="delete-account-input" type="text" autocomplete="off" autocapitalize="characters" placeholder="DELETE">
      </div>
      <div id="delete-account-error" class="form-error${state.deleteAccountError ? '' : ' hidden'}" role="alert">${escapeHTML(state.deleteAccountError || '')}</div>
      <button type="button" class="btn btn-danger btn-sm mt-2" data-action="confirm-delete-account"${state.deleteAccountBusy ? ' disabled' : ''}>${state.deleteAccountBusy ? 'Deleting…' : 'Permanently delete my account'}</button>
      <button type="button" class="btn btn-ghost btn-sm mt-2" data-action="cancel-delete-account">Cancel</button>
    </div>
  ` : `
    <button type="button" class="btn btn-danger btn-sm mt-3" data-action="open-delete-account">Delete account…</button>
  `;

  const passwordSection = `
    <div class="card mt-3">
      <h3>${user?.hasPassword === false ? "Set Password" : "Change Password"}</h3>
      <form id="change-password-form" data-action="change-password">
        ${user?.hasPassword === false ? "" : `<div class="form-group">
          <label class="form-label" for="current-password">Current Password</label>
          <input id="current-password" type="password" name="currentPassword" required autocomplete="current-password">
        </div>`}
        <div class="form-group">
          <label class="form-label" for="new-password">New Password</label>
          <input id="new-password" type="password" name="newPassword" required minlength="8" autocomplete="new-password">
        </div>
        <div class="form-group">
          <label class="form-label" for="confirm-password">Confirm New Password</label>
          <input id="confirm-password" type="password" name="confirmPassword" required minlength="8" autocomplete="new-password">
        </div>
        <div id="change-password-error" class="form-error hidden"></div>
        <button type="submit" class="btn btn-primary btn-sm mt-2">Update Password</button>
      </form>
    </div>
  `;

  const notifTypes = state.availableNotificationTypes || [];
  const notifPrefsLoaded = !!state.notificationPrefs;

  let notifPrefsCard = "";
  if (notifPrefsLoaded && notifTypes.length > 0) {
    const prefs = state.notificationPrefs;
    const pushEnabled = prefs?.pushEnabled !== false;
    const checked = pushEnabled ? " checked" : "";

    const permBtn = typeof Notification !== 'undefined' && Notification.permission === 'default'
      ? `<p class="mt-2"><button type="button" class="btn btn-sm btn-primary" data-action="enable-notifications">Enable Notifications</button></p>`
      : '';

    const defaultLead = prefs?.defaultReminderLeadMinutes ?? 10;
    const leadTimes = [5, 10, 15, 30, 60];

    // Per-type push toggles — only visible when push is enabled.
    // Schedule Reminder has a nested lead time selector.
    let pushTypeRows = "";
    if (pushEnabled) {
      const rows = notifTypes.map(t => {
        const allEnabled = (prefs?.enabledPushTypes || []).length === 0;
        const isEnabled = allEnabled || (prefs?.enabledPushTypes || []).includes(t.type);
        const typeChecked = isEnabled ? " checked" : "";

        let nested = "";
        if (t.type === "schedule_reminder" && isEnabled) {
          nested = `<div class="notif-pref-row notif-pref-row--nested">
            <label class="notif-pref-label">
              <span class="notif-pref-title">Reminder lead time</span>
              <span class="notif-pref-desc">Minutes before a scheduled chore's time</span>
            </label>
            <select data-action="change-default-reminder-lead" class="notif-pref-select">
              ${leadTimes.map(m => `<option value="${m}"${m === defaultLead ? ' selected' : ''}>${m} min</option>`).join("")}
            </select>
          </div>`;
        }

        return `<div class="notif-pref-row">
          <label class="notif-pref-label">
            <span class="notif-pref-title">${escapeHTML(t.label)}</span>
            <span class="notif-pref-desc">${escapeHTML(t.description)}</span>
          </label>
          <label class="notif-pref-toggle">
            <input type="checkbox" data-action="toggle-notif-pref" data-notif-type="${escapeHTML(t.type)}"${typeChecked}>
            <span class="toggle-slider"></span>
          </label>
        </div>${nested}`;
      }).join("");
      pushTypeRows = `<div class="notif-pref-list">${rows}</div>`;
    }

    notifPrefsCard = `<div class="card mt-3">
      <h3>Notifications</h3>
      <p class="text-secondary">Applies to your account across all households.</p>
      <div class="notif-pref-row">
        <label class="notif-pref-label">
          <span class="notif-pref-title">Push Notifications</span>
        </label>
        <label class="notif-pref-toggle">
          <input type="checkbox" data-action="toggle-push-enabled"${checked}>
          <span class="toggle-slider"></span>
        </label>
      </div>
      ${permBtn}
      ${pushTypeRows}
    </div>`;
  }

  const volumeUnit = state.volumeUnit === "oz" ? "oz" : "ml";
  const badgeHidden = !!state.hideNotificationBadge;
  const prefsCard = `<div class="card mt-3">
    <h3>Preferences</h3>
    <div class="pref-row">
      <label class="pref-label">
        <span class="pref-title">Feed volume unit</span>
        <span class="pref-desc">How bottle volumes are shown and entered</span>
      </label>
      <div class="segmented" role="group" aria-label="Feed volume unit">
        <button type="button" class="segmented-btn${volumeUnit === "ml" ? " segmented-btn--active" : ""}" data-action="set-volume-unit" data-unit="ml" aria-pressed="${volumeUnit === "ml"}">mL</button>
        <button type="button" class="segmented-btn${volumeUnit === "oz" ? " segmented-btn--active" : ""}" data-action="set-volume-unit" data-unit="oz" aria-pressed="${volumeUnit === "oz"}">oz</button>
      </div>
    </div>
    <div class="pref-row">
      <label class="pref-label">
        <span class="pref-title">Hide notification badge</span>
        <span class="pref-desc">Notifications still collect; the unread count on the bell stays hidden</span>
      </label>
      <label class="notif-pref-toggle">
        <input type="checkbox" data-action="toggle-hide-notification-badge"${badgeHidden ? " checked" : ""} aria-label="Hide notification badge">
        <span class="toggle-slider"></span>
      </label>
    </div>
  </div>`;

  const householdRole = state.members?.find(m => m.userId === user?.id)?.role || user?.role;
  const canExportHousehold = !!hh && (householdRole === "owner" || householdRole === "admin");
  const exportCard = renderExports(state, canExportHousehold);

  const activeId = state.activeHouseholdId || hh?.id;
  const yourHouseholdsCard = state.userHouseholds && state.userHouseholds.length > 1 ? `
    <div class="card mt-3">
      <h3>Your Households</h3>
      <div class="profile-households">
        ${state.userHouseholds.map(h => {
          const isActive = h.id === activeId;
          const ini = h.initials || h.name.charAt(0).toUpperCase();
          return `<button type="button" class="profile-household-item${isActive ? ' profile-household-item--active' : ''}"
            data-action="activate-household" data-household-id="${escapeHTML(h.id)}">
            <span class="hh-initials-badge-sm" aria-hidden="true">${escapeHTML(ini)}</span>
            <span class="profile-household-name">${escapeHTML(h.name)}</span>
            <span class="profile-household-role text-secondary">${escapeHTML(h.role)}</span>
            ${isActive ? '<span class="profile-household-check" aria-label="Active">&#10003;</span>' : ''}
          </button>`;
        }).join('')}
      </div>
    </div>` : "";

  if (!hh) {
    return `<div class="settings-view">${renderHouseholdView(null, null, null, state.user)}${yourHouseholdsCard}${prefsCard}${notifPrefsCard}${exportCard}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p>${verificationSection}${passwordSection}${deleteAccountSection}</div></div>`;
  }
  return `<div class="settings-view"><h2>Settings</h2>${renderHouseholdView(hh, state.members, state.invites, state.user)}${yourHouseholdsCard}${prefsCard}${notifPrefsCard}${exportCard}<div class="card mt-3"><h3>Account</h3><p class="text-secondary">${escapeHTML(state.user ? state.user.email : '')}</p>${verificationSection}${passwordSection}${deleteAccountSection}</div></div>`;
}

function loadAllStatsData() {
  const scope = captureScope(state, 'stats-page-render');
  let frame = null;
  const renderCurrentStats = () => {
    if (scope.current() && state.currentRoute === '/stats') render(document.querySelector('#app'));
  };
  const pending = loadStatsPage(state, () => {
    // A burst of chart responses should cause at most one full DOM morph per
    // frame. Old households, refreshes and tabs must not repaint this page.
    if (frame !== null || !scope.current() || state.currentRoute !== '/stats') return;
    frame = requestAnimationFrame(() => { frame = null; renderCurrentStats(); });
  });
  renderCurrentStats();
  return pending.finally(() => {
    if (frame !== null) cancelAnimationFrame(frame);
    renderCurrentStats();
  });
}
function loadWidgetData() { return loadStatsWidgets(state); }

function refreshStatsResource(section, id = null) {
  const scope = captureScope(state);
  const pending = loadStatsResource(state, section, id);
  render(document.querySelector('#app'));
  return pending.then(scope.guard(() => render(document.querySelector('#app'))));
}

function apiExclusiveEnd(inclusiveEnd) {
  return shiftDateStr(inclusiveEnd, 1);
}

function recentVolumesForChore(choreId) { return state.recentAmounts?.[choreId] || []; }

function ensureSheetRecentAmounts(root) {
  const draft = state.activeSheetData;
  if (!['log','home-log'].includes(state.activeSheet) || draft.recentRequested) return;
  const chore = state.chores.find(c => c.id === draft.choreId);
  if (!chore?.hasVolumeML) return;
  draft.recentRequested = true;
  const scope = captureScope(state);
  loadRecentAmounts(state,chore.id).then(scope.guard(() => {
    if (!['log','home-log'].includes(state.activeSheet) || state.activeSheetData !== draft) return;
    const container = root.querySelector('.sheet-recent-volume-row');
    if (container) container.innerHTML = renderRecentAmounts(chore,recentVolumesForChore(chore.id),state.volumeUnit);
    syncLogSaveControls(root);
  }));
}

function countTodayLogs() {
  if (!state.todayLogs) return 0;
  const today = new Date();
  const todayStr = localDateStr(today);
  return state.todayLogs.filter(l => {
    const d = l.completedAt ? new Date(l.completedAt) : null;
    return d ? localDateStr(d) === todayStr : false;
  }).length;
}

function renderStatsPageView() {
  try {
    if (state.stats) {
      const page = renderStatsPage(state);
      if (state.activeSheet === "widget-wizard") {
        return `<div class="sheet-overlay-wrapper">
          ${page}
          <div class="sheet-backdrop" data-action="close-sheet" aria-hidden="true"></div>
          ${renderWidgetWizard(state, state.activeSheetData?.widgetDraft)}
        </div>`;
      }
      return page;
    }
    return '<div class="stats-page"><h2>Stats</h2><p class="text-center text-secondary">Loading...</p></div>';
  } catch {
    return '<div class="stats-page"><h2>Stats</h2><p class="text-center text-secondary">Stats unavailable</p></div>';
  }
}

async function loadLatestLogsData() {
  const contextScope = captureScope(state, "latest-logs");
  if (!state.household) return;
  try {
    const data = await withCurrentContext(loadLatestLogs(), contextScope);
    state.latestLogs = data?.latestLogs || {};
  } catch {}
}

// loadDayNotesData fetches the household's per-day diary notes (Phase 5.4) and
// indexes them by date for the Activity day headers.
async function loadDayNotesData() {
  const contextScope = captureScope(state, "day-notes");
  if (!state.household) return;
  try {
    const { data } = await withCurrentContext(apiFetch("/api/day-notes"), contextScope);
    const map = {};
    (data?.notes || []).forEach(n => { if (n.note) map[n.date] = n.note; });
    state.dayNotes = map;
  } catch {
    if (!contextScope.current()) return;
    state.dayNotes = state.dayNotes || {};
  }
}

function renderNotifPanel() {
  const container = document.querySelector("#notif-panel-container");
  if (container && !container.hidden) container.innerHTML = renderNotificationPanel(state.notifications, state);
}
async function loadNotifData(options) {
  if (!state.user) return;
  const pending = loadNotificationPage(state, options);
  renderNotifPanel();
  await pending;
  updateTopBar();
  renderNotifPanel();
}
async function updateNotification(action, id) {
  const pending = mutateNotification(state, action, id);
  renderNotifPanel();
  await pending;
  updateTopBar();
  renderNotifPanel();
}

async function loadNotificationPrefs() {
  const contextScope = captureScope(state);
  if (!state.user) return;
  try {
    const data = await withCurrentContext(loadNotificationPreferences(), contextScope);
    state.notificationPrefs = data.preferences;
    state.availableNotificationTypes = data.availableTypes;
  } catch {}
}

async function loadChoreReminderPrefsData() {
  const contextScope = captureScope(state);
  if (!state.user || !state.household) return;
  try {
    state.choreReminderPrefs = await withCurrentContext(loadChoreReminderPrefs(), contextScope);
  } catch {}
}

function scheduleReminderTypeEnabled() {
  const prefs = state.notificationPrefs;
  if (!prefs) return false;
  if (!prefs.pushEnabled && prefs.pushEnabled !== undefined) return false;
  const types = prefs.enabledPushTypes || [];
  if (types.length === 0) return true;
  return types.includes("schedule_reminder");
}

function getChoreReminderPref(choreId) {
  if (!choreId) return null;
  return (state.choreReminderPrefs || []).find(p => p.choreId === choreId) || null;
}

function showToastWithUndo(message, logId) {
  // Haptic tick on a successful log where supported (Android; harmless no-op
  // on iOS). This is the shared success path for logging a chore.
  if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate(10);
  const container = document.querySelector("#toast-container");
  if (!container) return;
  const toast = document.createElement("div");
  toast.className = "toast toast-success";
  toast.style.cssText = "display:flex;align-items:center;gap:8px;";
  const label = document.createElement("span");
  label.textContent = message;
  const undoBtn = document.createElement("button");
  undoBtn.type = "button";
  undoBtn.textContent = "Undo";
  undoBtn.style.cssText = "background:rgba(255,255,255,0.2);border:none;color:white;padding:4px 10px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:600;margin-left:auto;min-height:32px;";
  undoBtn.addEventListener("click", () => {
    toast.remove();
    undoLog(logId).then(owned(async () => {
  const contextScope = captureScope(state);
      await withCurrentContext(loadLatestLogsData(), contextScope);
      render(document.querySelector("#app"));
    })).catch(owned(() => showToast("Failed to undo", "error")));
  });
  toast.appendChild(label);
  toast.appendChild(undoBtn);
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

// Find a full log record by id across the loaded view collections, so a
// removed log can be faithfully re-created if the user taps Undo.
function findLogById(id) {
  for (const pool of [state.historyLogs, state.todayLogs, state.weekLogs]) {
    const found = (pool || []).find(l => l.id === id);
    if (found) return found;
  }
  return null;
}

// Toast shown after removing a log, whose Undo re-creates the log from its
// captured data (a new id, but identical fields/timing). Mirrors the
// create-then-undo affordance so removals are equally reversible.
function showToastWithRestore(message, log) {
  const container = document.querySelector("#toast-container");
  if (!container || !log) { showToast(message, "success"); return; }
  const toast = document.createElement("div");
  toast.className = "toast toast-success";
  toast.style.cssText = "display:flex;align-items:center;gap:8px;";
  const label = document.createElement("span");
  label.textContent = message;
  const btn = document.createElement("button");
  btn.type = "button";
  btn.textContent = "Undo";
  btn.style.cssText = "background:rgba(255,255,255,0.2);border:none;color:white;padding:4px 10px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:600;margin-left:auto;min-height:32px;";
  btn.addEventListener("click", () => {
    toast.remove();
    const d = new Date(log.completedAt);
    const pad = n => String(n).padStart(2, "0");
    const dateStr = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    logChore(
      log.choreId, log.note || "", dateStr, log.indicators || [],
      (log.slotHour ?? null), log.completedAt,
      (log.volumeML ?? null), log.userId ?? state.user?.id,
      log.indicatorVolumes || {}, 0, null,
      (log.rating ?? null), log.title ?? null,
    ).then(owned(async () => {
  const contextScope = captureScope(state);
      await withCurrentContext(Promise.all([reloadViewData(), loadLatestLogsData()]), contextScope);
      render(document.querySelector("#app"));
    })).catch(owned(() => showToast("Failed to restore log", "error")));
  });
  toast.appendChild(label);
  toast.appendChild(btn);
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 6000);
}

// setStarRatingValue applies a rating (0–50, in half-star increments of 5)
// to a `.star-rating` slider widget, updating its fill and ARIA state. Shared
// by the pointer (click) and keyboard (arrow-key) paths.
function setStarRatingValue(container, starTenths) {
  starTenths = Math.max(0, Math.min(50, Math.round(starTenths / 5) * 5));
  const fg = container.querySelector(".star-rating-fg");
  if (fg) fg.style.width = (starTenths * 2) + "%";
  container.dataset.rating = starTenths;
  const stars = starTenths / 10;
  container.setAttribute("aria-valuenow", starTenths);
  container.setAttribute("aria-valuetext", stars + " stars");
  const clearBtn = container.parentElement?.querySelector(".star-clear-btn");
  if (clearBtn) clearBtn.style.display = starTenths > 0 ? "" : "none";
}

function updateTabs(route) {
  const tabs = document.querySelector("#bottom-tabs");
  if (!tabs || !state.user) return;
  tabs.querySelectorAll(".tab-item").forEach((tab) => {
    const active = route === "/" + tab.dataset.nav || (route === "/" && tab.dataset.nav === "today");
    tab.classList.toggle("active", active);
  });
}

function updateTopBar() {
  const topBar = document.querySelector("#top-bar");
  const tabs = document.querySelector("#bottom-tabs");
  if (!topBar || !tabs) return;

  if (state.user) {
    topBar.hidden = false;
    tabs.hidden = false;
    const hhIndicator = document.querySelector("#hh-indicator");
    if (hhIndicator) {
      hhIndicator.hidden = false;
      const initials = state.household?.initials || "";
      hhIndicator.textContent = initials || state.user.email.charAt(0).toUpperCase();
      hhIndicator.title = state.household?.name || state.user.email;
    }
    const bell = document.querySelector("#notifications-bell");
    const badge = document.querySelector("#notification-badge");
    if (bell) {
      bell.hidden = false;
      bell.title = "Notifications";
    }
    if (badge) {
      // The hideNotificationBadge preference only suppresses the count
      // display; notifications still accumulate (see settings toggle).
      if (!state.hideNotificationBadge && state.unreadNotifications > 0) {
        badge.hidden = false;
        badge.textContent = String(state.unreadNotifications);
      } else {
        badge.hidden = true;
        badge.textContent = "";
      }
    }
  } else {
    topBar.hidden = true;
    tabs.hidden = true;
  }
}

// ─── Duration timer chip (Phase 5.2) ─────────────────────────────────────────

// renderTimerChip creates/updates/removes a fixed-position chip showing the
// running timer. It lives on document.body (outside the morph root) so DOM
// morphing never disturbs it. A 1s interval keeps the elapsed time fresh.
let _timerInterval = null;
function renderTimerChip() {
  let chip = document.querySelector("#timer-chip");
  const t = state.activeTimer;
  if (!t) {
    if (chip) chip.remove();
    if (_timerInterval) { clearInterval(_timerInterval); _timerInterval = null; }
    return;
  }
  if (!chip) {
    chip = document.createElement("button");
    chip.id = "timer-chip";
    chip.type = "button";
    chip.className = "timer-chip";
    chip.setAttribute("data-action", "stop-timer");
    document.body.appendChild(chip);
  }
  chip.disabled = !!t.saving;
  const secs = elapsedSeconds(t);
  chip.innerHTML = `<span class="timer-chip-icon">${escapeHTML(t.choreIcon || "⏱")}</span>
    <span class="timer-chip-name">${escapeHTML(t.choreName || "Timer")}</span>
    <span class="timer-chip-time">${formatElapsed(secs)}</span>
    <span class="timer-chip-stop">${t.saving ? "Saving…" : t.stoppedAt ? "Retry log" : "Stop &amp; log"}</span>`;
  if (!_timerInterval) {
    _timerInterval = setInterval(() => {
      const el = document.querySelector("#timer-chip .timer-chip-time");
      if (el && state.activeTimer) el.textContent = formatElapsed(elapsedSeconds(state.activeTimer));
      else if (!state.activeTimer && _timerInterval) { clearInterval(_timerInterval); _timerInterval = null; }
    }, 1000);
  }
  sheetController?.syncBackground();
}

function closeNotifPanel() {
  const container = document.querySelector("#notif-panel-container");
  if (container && !container.hidden) {
    container.hidden = true;
    container.innerHTML = "";
  }
}

function closeProfilePanel() {
  const container = document.querySelector("#profile-panel-container");
  if (container && !container.hidden) {
    container.hidden = true;
    container.innerHTML = "";
  }
}

function closeAllPanels() {
  closeNotifPanel();
  closeProfilePanel();
}

function showToast(message, type) {
  const container = document.querySelector("#toast-container");
  if (!container) return;
  const toast = document.createElement("div");
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3000);
}

function setError(containerId, message) {
  const el = document.querySelector(containerId);
  if (!el) return;
  el.textContent = message;
  el.classList.remove("hidden");
}

function hideError(containerId) {
  const el = document.querySelector(containerId);
  if (!el) return;
  el.classList.add("hidden");
}

function adoptUser(user, route = "/") {
  const invite = state._pendingInviteCode;
  activeExport?.controller.abort();
  activeExport = null;
  resetAuthedState(state);
  state.user = user;
  state.currentRoute = route;
  if (invite) state._pendingInviteCode = invite;
  document.querySelector("#toast-container")?.replaceChildren();
  delete window.__pushDiag;
  delete window.__pushError;
  startNotifPoll();
}

let identityUIRevision = 0;
let deleteAccountIntent = 0;
let deleteAccountAttempt = null;
let sessionCheck = null;
async function confirmBrowserSession({ invalidate = false, expected = contextSnapshot() } = {}) {
  if (!contextIsCurrent(expected) || state.logoutPending || state.transitioning) return false;
  const route = state.currentRoute || window.location.pathname;
  const hide = entry => {
    entry.invalidated = true;
    entry.invalidate?.();
    entry.ticket = ++identityUIRevision;
    entry.route = route;
    closeAllPanels(); adoptUser(null,entry.route); state.sessionUnconfirmed = true;
    render(document.querySelector("#app"));
  };
  if (sessionCheck?.revision === expected?.revision) {
    if (invalidate && !sessionCheck.invalidated) hide(sessionCheck);
    return sessionCheck.promise;
  }
  const entry = { revision:expected?.revision, ticket:identityUIRevision, route, invalidated:false };
  if (invalidate) hide(entry);
  entry.promise = (async () => {
    try {
      const result = await checkIdentity(expected,{control:entry});
      if (entry.ticket !== identityUIRevision) return false;
      if (!resultIsCurrent(result)) {
        if (result.identity?.revision === contextSnapshot()?.revision) {
          closeAllPanels(); adoptUser(null,entry.route); state.sessionUnconfirmed = true;
          render(document.querySelector("#app"));
        }
        return false;
      }
      if (result.identity.revision !== expected?.revision || entry.invalidated) {
        await finishIdentity(result,entry.route,() => { state.logoutPending = result.logoutPending; });
      } else state.user = result.user;
      return !!result.user && resultIsCurrent(result);
    } catch (err) {
      if (entry.ticket === identityUIRevision && err.identity?.revision === contextSnapshot()?.revision) {
        closeAllPanels(); adoptUser(null,entry.route); state.sessionUnconfirmed = true;
        render(document.querySelector("#app"));
      }
      return false;
    } finally { if (sessionCheck === entry) sessionCheck = null; }
  })();
  sessionCheck = entry;
  return entry.promise;
}
async function finishIdentity(result, route = "/", afterAdopt = () => {}) {
  if (!resultIsCurrent(result)) return false;
  adoptUser(result.user, route);
  afterAdopt();
  const scope = captureScope(state);
  await reloadAfterAuth();
  if (!scope.current()) return false;
  render(document.querySelector("#app"));
  return true;
}
async function recoverIdentity(error, route = "/", afterAdopt = () => {}) {
  if (!resultIsCurrent(error)) return;
  adoptUser(error.confirmed ? error.user : null, route);
  state.sessionUnconfirmed = !error.confirmed;
  afterAdopt();
  const scope = captureScope(state);
  if (error.confirmed) await reloadAfterAuth();
  if (scope.current()) render(document.querySelector("#app"));
}
async function doLogout() {
  if (state.logoutBusy) return;
  const ticket = ++identityUIRevision, app = document.querySelector("#app");
  closeAllPanels(); state.logoutBusy = true;
  try {
    const result = await handleLogout(({durable}) => {
      if (ticket !== identityUIRevision) return;
      adoptUser(null); state.logoutPending = true; state.logoutBusy = true; state.logoutDurable = durable; render(app);
    });
    if (resultIsCurrent(result) && ticket === identityUIRevision) adoptUser(null);
  } catch (err) {
    const identity = contextSnapshot();
    if (ticket !== identityUIRevision) return;
    if (identity?.status === "logout-pending") {
      adoptUser(null); state.logoutPending = true; state.logoutError = err.message;
    } else showToast(err.message || "Could not finish signing out. Please retry.", "error");
  }
  if (ticket === identityUIRevision) { state.logoutBusy = false; render(app); }
}
async function doLogin(form) {
  if (form.dataset.busy) return;
  hideError("#login-error"); requestNotificationPermission();
  form.dataset.busy = "true";
  try {
    const result = await handleLogin(form.querySelector("#login-email").value, form.querySelector("#login-password").value);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.user) {
      if (!await finishIdentity(result)) return;
      if (state._pendingInviteCode && !state.household) await doJoinWithCode(state._pendingInviteCode);
    } else if (form.isConnected) setError("#login-error",result.data?.error || "Invalid email or password");
  } catch (err) {
    await recoverIdentity(err);
    if (form.isConnected) setError("#login-error",err.message || "Could not sign in. Please retry.");
  } finally { delete form.dataset.busy; }
}

async function reloadAfterAuth() {
  const contextScope = captureScope(state);
  if (!state.user) return;
  const scope = captureScope(state);
  startNotifPoll();
  maybeSubscribePush().catch(() => {});
  state.activeTimer = loadTimer(scope.origin);
  await withCurrentContext(Promise.all([loadHouseholdData(),loadPreferences(state),hydratePendingLogs()]), contextScope);
  if (!scope.current()) return;
  await withCurrentContext(syncTimezone(state), contextScope);
  if (!scope.current() || !state.household) return;
  await withCurrentContext(loadChoreData(), contextScope);
  if (!scope.current()) return;
  await withCurrentContext(Promise.all([loadLatestLogsData(),loadNotifData(),reloadViewData()]), contextScope);
}


async function doRegister(form) {
  if (form.dataset.busy) return;
  hideError("#register-error"); hideError("#register-status");
  const email = form.querySelector("#reg-email").value, password = form.querySelector("#reg-password").value;
  if (password !== form.querySelector("#reg-confirm").value) { setError("#register-error","Passwords do not match"); return; }
  requestNotificationPermission(); form.dataset.busy = "true";
  try {
    const result = await handleRegister(email,password);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.data?.user && result.user) {
      if (!await finishIdentity(result)) return;
      if (state._pendingInviteCode && !state.household) await doJoinWithCode(state._pendingInviteCode);
    } else if (form.isConnected) {
      if (result.ok) setError("#register-status","If this email is new, check your inbox. You can also sign in or request a magic link.");
      else setError("#register-error",result.data?.error || "Registration failed");
    }
  } catch (err) {
    await recoverIdentity(err);
    if (form.isConnected) setError("#register-error","We couldn't confirm registration. Try again, or sign in if your account was created.");
  } finally { delete form.dataset.busy; }
}
async function doMagicLinkRequest(form) {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  try {
    await withCurrentContext(handleMagicLinkRequest(form.querySelector("#magic-email").value), contextScope);
    if (scope.current() && form.isConnected) setError("#magic-link-status","Check your email for the magic link!");
  } catch {
    if (!contextScope.current()) return; if (scope.current()) showToast("Could not request a link. Please retry.","error"); }
}
async function doForgotPassword(form) {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  try {
    await withCurrentContext(handleForgotPassword(form.querySelector("#forgot-email").value), contextScope);
    if (scope.current()) showToast("If an account exists, a reset link has been sent.","info");
  } catch {
    if (!contextScope.current()) return; if (scope.current()) showToast("Could not request a link. Please retry.","error"); }
}
async function doResetPassword(form) {
  if (form.dataset.busy) return;
  hideError("#reset-error");
  const password = form.querySelector("#reset-password").value;
  if (password !== form.querySelector("#reset-confirm").value) { setError("#reset-error","Passwords do not match"); return; }
  form.dataset.busy = "true";
  try {
    const result = await handleResetPassword(form.querySelector("input[name='token']").value,password);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.user) await finishIdentity(result);
    else if (form.isConnected) setError("#reset-error",result.data?.error || "Password reset failed");
  } catch (err) { await recoverIdentity(err); if (form.isConnected) setError("#reset-error",err.message || "Password reset failed"); }
  finally { delete form.dataset.busy; }
}
async function doChangePassword(form) {
  if (form.dataset.busy) return;
  hideError("#change-password-error");
  const password = form.querySelector("#new-password").value;
  if (password !== form.querySelector("#confirm-password").value) { setError("#change-password-error","New passwords do not match"); return; }
  if (password.length < 8) { setError("#change-password-error","Password must be at least 8 characters"); return; }
  form.dataset.busy = "true";
  try {
    const result = await handleChangePassword(form.querySelector("#current-password")?.value || "",password);
    if (!resultIsCurrent(result)) return;
    if (result.ok && result.user) {
      if (await finishIdentity(result,"/settings")) showToast("Password updated","success");
    } else {
      maybeSubscribePush().catch(() => {});
      if (form.isConnected) setError("#change-password-error",result.data?.error || "Password change failed");
    }
  } catch (err) { await recoverIdentity(err,"/settings"); if (form.isConnected) setError("#change-password-error",err.message || "Password change failed"); }
  finally { delete form.dataset.busy; }
}
async function doResendVerification() {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  try {
    const {response} = await withCurrentContext(apiFetch("/api/auth/email/verification/resend",{method:"POST"}), contextScope);
    if (!response.ok) throw new Error("Verification request failed");
    if (scope.current()) showToast("Verification email queued","info");
  } catch {
    if (!contextScope.current()) return; if (scope.current()) showToast("Failed to request verification email. Please try again.","error"); }
}
async function verifyEmail(token) {
  const ticket = ++identityUIRevision;
  try {
    const result = await changeIdentity(async () => {
      const response = await fetch(`/api/auth/email/verify?token=${encodeURIComponent(token)}`,{signal:AbortSignal.timeout(15000)});
      if (!response.ok) throw new Error("Verification failed");
      return {ok:true};
    });
    if (ticket !== identityUIRevision || !resultIsCurrent(result)) return;
    if (!await finishIdentity(result,"/verify-email", () => {
      state._emailVerified = true; state._emailVerificationStatus = "success";
    })) return;
  } catch (err) {
    if (ticket !== identityUIRevision) return;
    await recoverIdentity(err,"/verify-email", () => {
      state._emailVerified = true; state._emailVerificationStatus = "error";
    });
  }
  if (ticket === identityUIRevision) render(document.querySelector("#app"));
}
async function doJoinWithCode(code) {
  state._pendingInviteCode = null;
  await runHouseholdTransition(() => joinHousehold(code),{route:"/"});
}
async function consumeMagicLink(token) {
  const ticket = ++identityUIRevision;
  try {
    const result = await changeIdentity(async () => {
      const response = await fetch(`/api/auth/magic-link/consume?token=${encodeURIComponent(token)}`,{signal:AbortSignal.timeout(15000)});
      if (!response.ok) throw new Error("Magic link is unavailable. Request a new link.");
      return {ok:true};
    });
    if (ticket === identityUIRevision) await finishIdentity(result);
  } catch (err) {
    if (ticket !== identityUIRevision) return;
    await recoverIdentity(err);
    if (ticket === identityUIRevision) { state.currentRoute="/"; showToast(err.message,"error"); render(document.querySelector("#app")); }
  }
}
async function doDeleteAccount() {
  if (deleteAccountAttempt) return;
  const input = document.querySelector("#delete-account-input");
  const app = document.querySelector("#app");
  if (input?.value.trim() !== "DELETE") {
    state.deleteAccountError = "Type DELETE (in capitals) to confirm.";
    render(app);
    return;
  }
  const origin = contextSnapshot(), ticket = ++identityUIRevision;
  const attempt = {intent:deleteAccountIntent};
  deleteAccountAttempt = attempt;
  state.deleteAccountBusy = true;
  state.deleteAccountError = null;
  render(app);
  try {
    const result = await changeIdentity(async origin => {
      await apiFetch("/api/me",{method:"DELETE",body:JSON.stringify({confirm:"DELETE"}),origin,allowContextChange:true});
      return {ok:true};
    });
    if (ticket !== identityUIRevision || !resultIsCurrent(result)) return;
    if (await finishIdentity(result)) showToast("Your account has been deleted.","success");
  } catch (err) {
    if (ticket !== identityUIRevision) return;
    await recoverIdentity(err,state.currentRoute || "/",() => {
      if (attempt.intent !== deleteAccountIntent || !err.confirmed || !sameOrigin(origin,{userId:err.user?.id,householdId:err.user?.householdId || 0})) return;
      state.deleteAccountOpen = true;
      state.deleteAccountBusy = true;
      state.deleteAccountError = err.status ? err.message : "Account deletion could not be confirmed. Please retry.";
    });
  } finally {
    if (deleteAccountAttempt === attempt) deleteAccountAttempt = null;
    if (ticket === identityUIRevision) {
      state.deleteAccountBusy = false;
      if (state.deleteAccountOpen) render(app);
    }
  }
}

export async function init() {
  state = createAppState();
  sheetController = createSheetController(() => {
    state.activeSheet = null; state.activeSheetData = {};
    render(document.querySelector('#app'));
  });

  state.googleOAuthEnabled = document.body?.dataset?.googleOauthEnabled === "true";
  state.appleSignInEnabled = document.body?.dataset?.appleSigninEnabled === "true";

  // Restore an in-progress duration timer (Phase 5.2) so it survives reloads.


  // Register the service worker and set up the controllerchange listener early,
  // before any async work, so the "App updated" toast fires reliably on every
  // deploy regardless of session-load timing.
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/service-worker.js').then(reg => {
      window.__swReg = reg;
      lastSWUpdateCheck = Date.now();
    }).catch(() => {});

    let hadController = !!navigator.serviceWorker.controller;
    let swRefreshing = false;
    navigator.serviceWorker.addEventListener('controllerchange', () => {
      if (swRefreshing) return;
      if (!hadController) { hadController = true; return; }
      const container = document.querySelector("#toast-container");
      if (!container) return;
      const toast = document.createElement("div");
      toast.className = "toast toast-info sw-update-toast";
      toast.style.cssText = "display:flex;align-items:center;gap:8px;";
      const label = document.createElement("span");
      label.textContent = "App updated";
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = "Refresh";
      btn.style.cssText = "background:rgba(255,255,255,0.2);border:none;color:white;padding:4px 10px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:600;margin-left:auto;min-height:32px;";
      btn.addEventListener("click", () => {
        swRefreshing = true;
        window.location.reload();
      });
      toast.appendChild(label);
      toast.appendChild(btn);
      container.appendChild(toast);
      setTimeout(() => { if (!swRefreshing) toast.remove(); }, 30000);
    });

    // Poll for updates every 5 minutes as a fallback, in case the user
    // leaves the app open on a single view without navigating.
    setInterval(() => {
      if (window.__swReg) window.__swReg.update().catch(() => {});
    }, 300000);

    // A "Log now" notification action on an already-open app posts a message
    // (rather than opening a new window). Open the pre-filled log sheet.
    navigator.serviceWorker.addEventListener("message", (event) => {
      const msg = event.data;
      if (msg && msg.type === "quicklog" && msg.choreId && state.user && !sessionCheck && sameOrigin(msg, contextSnapshot())) {
        const chore = (state.chores || []).find(c => c.id === msg.choreId);
        if (chore) {
          state.currentRoute = "/";
          state.homeView = "log";
          state.activeSheet = "home-log";
          state.activeSheetData = { choreId: chore.id };
          const appEl = document.querySelector("#app");
          if (appEl) render(appEl);
        }
      }
    });
  }

  try {
    const identity = await bootstrapIdentity();
    state.user = identity.user;
    state.logoutPending = identity.logoutPending;
    state.activeTimer = loadTimer(contextSnapshot());
  } catch { state.user = null; state.sessionUnconfirmed = true; }

  if (state.user) {
    maybeSubscribePush().catch(() => {});
  }

  const app = document.querySelector("#app");
  if (!app) return;

  window.addEventListener("nabu-session-invalid", event => {
    if (!state.sessionUnconfirmed && contextIsCurrent(event.detail?.origin)) {
      void confirmBrowserSession({invalidate:true,expected:event.detail.origin});
    }
  });

  onExternalIdentityChange(async record => {
    const ticket = ++identityUIRevision, route = state.currentRoute || window.location.pathname;
    adoptUser(null,route);
    state.logoutPending = record?.status === "logout-pending";
    state.transitioning = record?.status === "changing";
    state.sessionUnconfirmed = record?.status === "unconfirmed";
    render(app);
    if (record?.status !== "active") return;
    try {
      const result = await bootstrapIdentity();
      if (ticket === identityUIRevision && resultIsCurrent(result)) await finishIdentity(result,route);
    } catch { /* A newer identity event owns recovery. */ }
  });

  let longPressTimer    = null;
  let longPressJustFired = false;
  let pressStartX = 0;
  let pressStartY = 0;
  const jiggleDrag = { active: false, choreId: null, targetChoreId: null };

  document.addEventListener("click", (e) => {
    // Always prevent default for nav links so a long-press residual click
    // never causes a full-page navigation via the href attribute.
    const navEl = e.target.closest("[data-nav]");
    if (navEl) e.preventDefault();

    // Only swallow the residual click on the card / cell that was long-pressed.
    // Sheet buttons (e.g. the "Log" save button) are inside .bottom-sheet and
    // must always be processed, even if the user taps within the 50 ms grace
    // period after lifting their finger from the long-press.
    if (longPressJustFired) {
      longPressJustFired = false;
      if (!e.target.closest(".bottom-sheet")) return;
    }

    const actionEl = e.target.closest("[data-action]");

    // Dismiss any open heatmap tap-tooltip when tapping away from a cell.
    if (!e.target.closest("[data-action=\"heatmap-tap\"]")) {
      document.querySelectorAll(".heatmap-tooltip--visible").forEach(el => el.classList.remove("heatmap-tooltip--visible"));
    }

    // data-nav SPA navigation: check first so it works without data-action
    if (navEl) {
      if (deleteAccountAttempt) {
        deleteAccountIntent++;
        state.deleteAccountOpen = false;
        state.deleteAccountError = null;
      }
      closeAllPanels();
      state.currentRoute = `/${navEl.dataset.nav}`;
      if (state.currentRoute === "/settings") {
        state._loadedHousehold = true;
        render(app);
        Promise.all([loadNotificationPrefs(), loadChoreReminderPrefsData()]).then(owned(() => render(app)));
        return;
      }
      if (state.currentRoute === "/activity") {
        if (state.activityView === "history") {
          loadActivityPage().catch(() => {});
        } else {
          render(app);
          (state.calendarView === "week" ? loadWeekData() : loadTodayData())
            .then(owned(() => render(app)));
        }
        return;
      }
      if (state.currentRoute === "/today") {
        state.homeView = "log";
        loadLatestLogsData().then(owned(() => render(app)));
        return;
      }
      if (state.currentRoute === "/stats") {
        state.stats = state.stats || {};
        state.stats.todayCount = countTodayLogs();
        loadAllStatsData();
        return;
      }
      if (state.currentRoute === "/schedule") {
        render(app);
        Promise.all([loadChoreData(), loadSchedules()]).then(owned(async ([, schedules]) => {
  const contextScope = captureScope(state);
          state.schedules = schedules;
          await withCurrentContext(loadTodayData(), contextScope);
          render(app);
        }));
        return;
      }
      render(app);
      return;
    }

    const action = actionEl?.dataset?.action;
    if (!action) return;

    if (action === "export-csv") {
      if (state.exportBusy) return;
      const scope = captureScope(state, "export");
      const ticket = {scope, controller:new AbortController()};
      activeExport = ticket;
      state.exportBusy = true;
      state.exportError = null;
      state.exportStatus = null;
      const range = {...(state.exportRange || {})};
      render(app);
      downloadCSV(actionEl.dataset.kind, range, scope.origin, ticket.controller.signal)
        .then(() => { if (scope.current()) state.exportStatus = "Export ready."; })
        .catch(err => {
          if (scope.current() && !ticket.controller.signal.aborted) state.exportError = err.name === "TimeoutError"
            ? "Export took too long. Choose a smaller date range and retry." : err.message;
        })
        .finally(() => {
          if (activeExport === ticket) activeExport = null;
          if (scope.current()) { state.exportBusy = false; render(app); }
        });
      return;
    }
    if (action === "cancel-export") {
      activeExport?.controller.abort();
      activeExport = null;
      captureScope(state, "export");
      state.exportBusy = false;
      state.exportError = null;
      state.exportStatus = "Export canceled.";
      render(app);
      return;
    }

    // ── Weekday pill: toggle on/off ─────────────────────────────────────────
    if (action === "toggle-day") {
      actionEl.classList.toggle("day-pill--on");
      actionEl.setAttribute("aria-pressed",
        String(actionEl.classList.contains("day-pill--on")));
      return;
    }

    // ── Indicator chip: toggle on/off (log sheet) ───────────────────────────
    if (action === "toggle-indicator") {
      actionEl.classList.toggle("log-chip--on");
      actionEl.setAttribute("aria-pressed",
        String(actionEl.classList.contains("log-chip--on")));
      const row = actionEl.closest(".indicator-row");
      const volumeSelect = row?.querySelector(".indicator-volume-select");
      if (volumeSelect) {
        volumeSelect.style.display = actionEl.classList.contains("log-chip--on") ? "" : "none";
      }
      return;
    }

    // ── Star rating: set the rating based on tap position ────────────────────
    if (action === "set-rating") {
      const container = actionEl.closest(".star-rating");
      if (!container) return;
      const rect = container.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const pct = x / rect.width;
      const starTenths = Math.round(pct * 50 / 5) * 5;
      setStarRatingValue(container, starTenths);
      return;
    }

    // ── Clear star rating ────────────────────────────────────────────────────
    if (action === "clear-rating") {
      const row = actionEl.closest(".star-rating-row");
      const container = row?.querySelector(".star-rating");
      if (!container) return;
      container.querySelector(".star-rating-fg").style.width = "0%";
      container.dataset.rating = "0";
      container.setAttribute("aria-valuenow", "0");
      container.setAttribute("aria-valuetext", "0 stars");
      actionEl.style.display = "none";
      return;
    }

    const actionScope = captureScope(state);
    switch (action) {
      case "google-signin":
      case "apple-signin": {
        const oauthURL = action === "apple-signin"
          ? "/api/auth/apple/web/login"
          : "/api/auth/google/login";
        if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
          Notification.requestPermission().then(owned(() => {
            window.location.href = oauthURL;
          })).catch(owned(() => {
            window.location.href = oauthURL;
          }));
        } else {
          window.location.href = oauthURL;
        }
        break;
      }
      case "show-login":
      case "show-register":
      case "show-magic-link":
      case "show-forgot-password": {
        e.preventDefault();
        const routes = {
          "show-login": "/",
          "show-register": "/register",
          "show-magic-link": "/magic-link",
          "show-forgot-password": "/forgot-password",
        };
        state.currentRoute = routes[action];
        render(app);
        break;
      }
      case "discard-log-draft": {
        e.preventDefault();
        const draft = state.activeSheetData;
        if (draft.saving || !draft.submission?.body) break;
        if (!confirm("Discard the saved copy and edit a new entry? This will not undo a log the server already received.")) break;
        const discard = draft.savedDurably
          ? discardQueuedLog(draft.submission.idempotencyKey, actionScope.origin) : Promise.resolve();
        discard.then(actionScope.guard(() => {
          if (state.activeSheetData !== draft) return;
          draft.submission = {}; draft.saveError = null; draft.savedDurably = false;
          syncLogSaveControls(app);
        })).catch(actionScope.guard(err => showToast(err.message, "error")));
        break;
      }
      case "retry-pending-log":
        e.preventDefault();
        void flushOfflineQueue(actionEl.dataset.key);
        break;
      case "discard-pending-log":
        e.preventDefault();
        if (confirm("Discard this saved copy? This will not undo a log the server already received.")) {
          discardQueuedLog(actionEl.dataset.key, actionScope.origin).then(actionScope.guard(async () => {
  const contextScope = captureScope(state);
            await withCurrentContext(hydratePendingLogs(), contextScope);
            if (actionScope.current()) render(app);
          })).catch(actionScope.guard(err => showToast(err.message, "error")));
        }
        break;
      case "retry-session":
        e.preventDefault();
        bootstrapIdentity().then(result => finishIdentity(result)).catch(() => showToast("Could not check your session. Please retry.","error"));
        break;
      case "logout":
      case "retry-logout":
        e.preventDefault();
        void doLogout();
        break;
      case "resend-verification":
        e.preventDefault();
        doResendVerification();
        break;
      case "open-delete-account":
        e.preventDefault();
        deleteAccountIntent++;
        state.deleteAccountOpen = true;
        state.deleteAccountBusy = !!deleteAccountAttempt;
        state.deleteAccountError = null;
        render(app);
        break;
      case "cancel-delete-account":
        e.preventDefault();
        deleteAccountIntent++;
        state.deleteAccountOpen = false;
        state.deleteAccountError = null;
        render(app);
        break;
      case "confirm-delete-account":
        e.preventDefault();
        void doDeleteAccount();
        break;

      case "open-notifications": {
        e.preventDefault();
        clearAppBadge();
        const container = document.querySelector("#notif-panel-container");
        if (container) { container.hidden = false; renderNotifPanel(); }
        void loadNotifData();
        break;
      }
      case "close-notifications":
        e.preventDefault(); closeNotifPanel(); break;
      case "refresh-notifications":
        e.preventDefault(); void loadNotifData(); break;
      case "more-notifications":
        e.preventDefault(); void loadNotifData({append:true}); break;
      case "mark-all-read":
        e.preventDefault(); clearAppBadge(); void updateNotification("all"); break;
      case "dismiss-notification":
        e.preventDefault(); void updateNotification("delete", Number(actionEl.dataset.notifId)); break;
      case "mark-notif-read":
        e.preventDefault(); void updateNotification("read", Number(actionEl.dataset.notifId)); break;
      case "open-profile": {
        e.preventDefault();
        closeNotifPanel();
        const container = document.querySelector("#profile-panel-container");
        if (container) {
          container.hidden = false;
          container.innerHTML = renderProfileSheet(state.user, state.household, state.userHouseholds, state.activeHouseholdId);
        }
        break;
      }
      case "close-profile": {
        e.preventDefault();
        closeProfilePanel();
        break;
      }
      case "profile-nav-settings": {
        e.preventDefault();
        closeProfilePanel();
        state._loadedHousehold = true;
        state.currentRoute = "/settings";
        render(app);
        Promise.all([loadNotificationPrefs(), loadChoreReminderPrefsData()]).then(owned(() => render(app)));
        break;
      }
      case "create-invite":
        e.preventDefault();
        createInvite().then(owned((data) => {
          if (data.invite) {
            const url = `${window.location.origin}/join?code=${data.invite.code}`;
            state.invites = [...(state.invites || []), data.invite];
            navigator.clipboard.writeText(url).then(
              owned(() => showToast("Invite link copied to clipboard!", "info")),
              owned(() => showToast("Invite link: " + url, "info"))
            );
            render(app);
          }
        }));
        break;
      case "copy-invite-link": {
        e.preventDefault();
        const code = actionEl.dataset.code;
        const url = `${window.location.origin}/join?code=${code}`;
        navigator.clipboard.writeText(url).then(
          owned(() => showToast("Invite link copied!", "info")),
          owned(() => showToast(`Link: ${url}`, "info"))
        );
        break;
      }
      case "delete-invite":
        e.preventDefault();
        deleteInvite(parseInt(actionEl.dataset.inviteId)).then(owned(async () => {
  const contextScope = captureScope(state);
          await withCurrentContext(loadHouseholdData(), contextScope);
          render(app);
        }));
        break;
      case "leave-household":
        e.preventDefault();
        if (!confirm("Are you sure you want to leave this household? All your data will remain with the household.")) break;
        void runHouseholdTransition(() => leaveHousehold());
        break;
      case "toggle-edit-household": {
        e.preventDefault();
        const editForm = app.querySelector("#edit-household-form");
        if (editForm) editForm.classList.toggle("hidden");
        break;
      }
      case "activate-household": {
        e.preventDefault();
        const hhId = parseInt(actionEl.dataset.householdId, 10);
        if (!hhId || hhId === state.activeHouseholdId) { closeProfilePanel(); break; }
        void runHouseholdTransition(() => activateHousehold(hhId));
        break;
      }
      case "remove-member": {
        e.preventDefault();
        const userId = parseInt(actionEl.dataset.userId, 10);
        const member = (state.members || []).find(m => m.userId === userId);
        const name = member ? (member.displayName || member.email) : "this member";
        // eslint-disable-next-line no-alert
        if (!confirm(`Remove ${name} from this household?`)) break;
        removeMember(userId).then(owned(async (data) => {
  const contextScope = captureScope(state);
          if (data.status === "removed") {
            await withCurrentContext(loadHouseholdData(), contextScope);
            render(app);
            showToast(`${name} removed`, "info");
          } else {
            showToast(data.error || "Failed to remove member", "error");
          }
        })).catch(owned(() => showToast("Failed to remove member", "error")));
        break;
      }
      case "update-member-role": {
        e.preventDefault();
        const userId = parseInt(actionEl.dataset.userId, 10);
        const newRole = actionEl.value;
        updateMemberRole(userId, newRole).then(owned(async (data) => {
  const contextScope = captureScope(state);
          if (data.status === "updated") {
            await withCurrentContext(loadHouseholdData(), contextScope);
            render(app);
          } else {
            showToast(data.error || "Failed to update role", "error");
          }
        })).catch(owned(() => showToast("Failed to update role", "error")));
        break;
      }
      case "transfer-ownership": {
        e.preventDefault();
        const userId = parseInt(actionEl.dataset.userId, 10);
        const member = (state.members || []).find(m => m.userId === userId);
        const name = member ? (member.displayName || member.email) : "this member";
        if (!confirm(`Transfer ownership to ${name}?`)) break;
        transferOwnership(userId).then(owned(async (data) => {
  const contextScope = captureScope(state);
          if (data.status === "transferred") {
            await withCurrentContext(loadHouseholdData(), contextScope);
            render(app);
            showToast(`Ownership transferred to ${name}`, "info");
          } else {
            showToast(data.error || "Failed to transfer ownership", "error");
          }
        })).catch(owned(() => showToast("Failed to transfer ownership", "error")));
        break;
      }
      case "log-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const slotEl = actionEl.closest('[data-hour]');
        const slotHour = slotEl ? parseInt(slotEl.dataset.hour, 10) : null;
        logChore(choreId, "", actionEl.dataset.date || "", [], slotHour, null, null, state.user?.id).then(owned(async () => {
  const contextScope = captureScope(state);
          await (withCurrentContext(state.calendarView === "week" ? loadWeekData() : loadTodayData(), contextScope));
          render(app);
        }));
        break;
      }
      case "undo-chore": {
        e.preventDefault();
        const uLogId = parseInt(actionEl.dataset.logId);
        // Capture the log before deleting so removal is undoable (2.3).
        const removedLog = findLogById(uLogId);
        undoLog(uLogId).then(owned(async () => {
  const contextScope = captureScope(state);
          state.activeSheet     = null;
          state.activeSheetData = {};
          state.historyLogs = (state.historyLogs || []).filter(l => l.id !== uLogId);
          await withCurrentContext(reloadViewData(), contextScope);
          render(app);
          showToastWithRestore("Log removed", removedLog);
        })).catch(owned((err) => {
          console.error('undo-chore failed:', err);
          state.activeSheet     = null;
          state.activeSheetData = {};
          reloadViewData().then(owned(() => render(app)));
          showToast(err.message || "Failed to remove log", "error");
        }));
        break;
      }
      case "view-log": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const logId   = actionEl.dataset.logId ? parseInt(actionEl.dataset.logId, 10) : null;
        const date    = actionEl.dataset.date || "";
        const chore   = (state.chores || []).find(c => c.id === choreId);
        if (chore) {
          state.activeSheet     = "log";
          state.activeSheetData = { choreId, logId, date };
          render(app);
        }
        break;
      }

      case "schedule-open-log": {
        e.preventDefault();
        const choreId    = parseInt(actionEl.dataset.choreId, 10);
        const date       = actionEl.dataset.date || "";
        const scheduleId = actionEl.dataset.scheduleId ? parseInt(actionEl.dataset.scheduleId, 10) : null;
        const slotHourVal = actionEl.dataset.slotHour;
        const slotHour = slotHourVal && slotHourVal !== ""
          ? parseInt(slotHourVal, 10)
          : null;
        const slotTime = actionEl.dataset.slotTime || "";
        state.activeSheet     = "log";
        state.activeSheetData = { choreId, logId: null, date, slotHour, scheduleId, slotTime };
        render(app);
        break;
      }
      case "schedule-tap-log": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const date    = actionEl.dataset.date || todayISO(0);
        const scheduleId = parseInt(actionEl.dataset.scheduleId, 10);
        const sch = (state.schedules || []).find(s => s.id === scheduleId);
        const slotHour = sch?.specificTime
          ? parseInt(sch.specificTime.split(":")[0], 10)
          : null;
        logChore(choreId, "", date, [], slotHour, null, null, state.user?.id).then(owned(async () => {
  const contextScope = captureScope(state);
          await withCurrentContext(loadTodayData(), contextScope);
          render(app);
        })).catch(owned(() => showToast("Failed to log chore", "error")));
        break;
      }
      case "edit-schedule": {
        e.preventDefault();
        const choreId    = parseInt(actionEl.dataset.choreId, 10);
        const scheduleId = parseInt(actionEl.dataset.scheduleId, 10);
        state.activeSheet     = "edit-schedule";
        state.activeSheetData = { choreId, scheduleId };
        render(app);
        break;
      }

      case "save-log": {
        e.preventDefault();
        const draft = state.activeSheetData;
        if (draft.saving) break;
        const invalid = [...actionEl.closest('.bottom-sheet').querySelectorAll('input')].find(input => !input.checkValidity());
        if (invalid) { invalid.reportValidity(); break; }
        draft.submission ||= {};
        const ownsSheet = () => actionScope.current() && state.activeSheetData === draft;
        const logId   = actionEl.dataset.logId;
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const note    = (document.querySelector('#log-note')?.value || "").trim();
        const titleVal = (document.querySelector('#log-title')?.value || "").trim();
        const indicators = [...document.querySelectorAll('.log-chip--on[data-action="toggle-indicator"]')]
          .map(el => el.dataset.label)
          .filter(Boolean);
        // Subject tag (Phase 5.5): single-select chip, if any.
        const subject = document.querySelector('.subject-chip.subject-chip--on')?.dataset.subject || null;
        const indicatorVolumes = {};
        document.querySelectorAll('.indicator-volume-select').forEach(select => {
          const indicator = select.dataset.indicator;
          const val = select.value;
          if (indicator && val !== "" && indicators.includes(indicator)) {
            indicatorVolumes[indicator] = parseInt(val, 10);
          }
        });
        const volumeVal = document.querySelector('#log-volume')?.value;
        const volumeML = volumeVal && volumeVal !== "" ? parseInt(volumeVal, 10) : null;
        const durationInput = document.querySelector('#log-duration');
        const durationSeconds = durationInput?.value ? Number(durationInput.value) : null;
        const memberVal = document.querySelector('#log-member')?.value;
        const userId = memberVal && memberVal !== "" ? parseInt(memberVal, 10) : null;

        const ratingEl = document.querySelector('.star-rating');
        const ratingVal = ratingEl?.dataset?.rating;
        const rating = ratingVal && ratingVal !== "0" ? parseInt(ratingVal, 10) : null;

        // Require volume AND indicator for chores that have both features.
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (chore && chore.hasVolumeML && (chore.indicatorLabels || []).length > 0) {
          if (Object.keys(indicatorVolumes).length === 0 || indicators.length === 0) {
            showToast("Select a volume and food type", "error");
            break;
          }
        }

        let date        = actionEl.dataset.date || "";
        let completedAt = actionEl.dataset.completedAt || null;
        let slotHour    = null;
        if (actionEl.dataset.slotHour && actionEl.dataset.slotHour !== "") {
          slotHour = parseInt(actionEl.dataset.slotHour, 10);
        }

        const whenInput = document.querySelector('#log-when');
        if (whenInput?.value) {
          if (!logId) {
            // New log: always use the when input value so the submitted
            // time matches what the user sees in the picker.
            completedAt = new Date(whenInput.value).toISOString();
            slotHour = new Date(whenInput.value).getHours();
            date = whenInput.value.split('T')[0];
          } else {
            // Compare the full displayed local value, including minutes.
            // An unchanged picker must not round away stored seconds.
            if (whenInput.value !== whenInput.dataset.originalValue) {
              completedAt = new Date(whenInput.value).toISOString();
              slotHour = new Date(whenInput.value).getHours();
              date = whenInput.value.split("T")[0];
            } else { completedAt = null; slotHour = null; date = ""; }
          }
        }

        draft.saving = true;
        actionEl.disabled = true;
        actionEl.dataset.readyLabel ||= actionEl.textContent;
        actionEl.textContent = "Saving…";
        const patch = { note };
        const original = logId ? findLogById(parseInt(logId,10)) : null;
        if ((chore?.indicatorLabels || []).length) patch.indicators = indicators;
        if (chore?.hasVolumeML && (chore?.indicatorLabels || []).length) patch.indicatorVolumes = indicatorVolumes;
        if (document.querySelector("#log-volume")) patch.volumeML = volumeML;
        if (durationInput) patch.durationSeconds = durationSeconds;
        if (ratingEl) patch.rating = rating;
        if (document.querySelector("#log-title")) patch.title = titleVal || null;
        if (document.querySelector(".subject-chip")) patch.subject = subject;
        if (userId !== null && userId !== original?.userId) patch.userId = userId;
        if (completedAt) { patch.completedAt = completedAt; patch.hour = slotHour; patch.date = date; }
        const doLog = logId
          ? updateLog(parseInt(logId, 10), patch)
          : (() => {
            const followUpDays = parseInt(document.querySelector('#followup-days')?.value || '0', 10) || 0;
            const followUpHours = parseInt(document.querySelector('#followup-hours')?.value || '0', 10) || 0;
            const followUpMins = parseInt(document.querySelector('#followup-mins')?.value || '0', 10) || 0;
            const followUpMinutes = followUpDays * 1440 + followUpHours * 60 + followUpMins;
            let followUpTime = null;
            if (followUpMinutes > 0 && whenInput?.value) {
              const when = new Date(whenInput.value);
              const fu = new Date(when.getTime() + followUpMinutes * 60000);
              const pad = n => String(n).padStart(2, "0");
              followUpTime = `${fu.getFullYear()}-${pad(fu.getMonth() + 1)}-${pad(fu.getDate())}T${pad(fu.getHours())}:${pad(fu.getMinutes())}`;
            }
            return logChore(choreId, note, date, indicators, slotHour, completedAt, volumeML, userId, indicatorVolumes, followUpMinutes, followUpTime, rating, titleVal || null, durationSeconds, subject, { submission:draft.submission });
          })();
        syncLogSaveControls(app);
        doLog.then(owned(async (data) => {
  const contextScope = captureScope(state);
          if (!actionScope.current()) return;
          const newLogId = data?.log?.id;
          if (ownsSheet()) { state.activeSheet = null; state.activeSheetData = {}; }
          await withCurrentContext(hydratePendingLogs(), contextScope);
          if (!actionScope.current()) return;
          if (data.queued) showToast("Saved on this device. Waiting to sync.", "info");
          if (state.currentRoute === "/" || state.currentRoute === "/today") {
            await withCurrentContext(loadLatestLogsData(), contextScope);
          }
          if (!actionScope.current()) return;
          await withCurrentContext(reloadViewData(), contextScope);
          if (!actionScope.current()) return;
          render(app);
          if (newLogId) {
            const chore = (state.chores || []).find(c => c.id === choreId);
            showToastWithUndo(`${chore ? chore.icon + " " + chore.name : "Chore"}`, newLogId);
          }
        })).catch(owned(async err => {
  const contextScope = captureScope(state);
          if (!ownsSheet()) return;
          draft.saveError = err.message || "Failed to save log. Your draft is still here.";
          draft.savedDurably = err.durable === true;
          await withCurrentContext(hydratePendingLogs(), contextScope);
          if (ownsSheet()) showToast(draft.saveError, "error");
        })).finally(owned(() => {
          draft.saving = false;
          if (ownsSheet()) syncLogSaveControls(app);
        }));
        break;
      }

      case "open-quick-log": {
        e.preventDefault();
        state.activeSheet     = "quick-log";
        state.activeSheetData = { date: state.calendarDate || todayISO(0) };
        render(app);
        break;
      }

      case "quick-log-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const date    = actionEl.dataset.date || "";
        const note    = (document.querySelector('#quick-log-note')?.value || "").trim();
        logChore(choreId, note, date, []).then(owned(async () => {
  const contextScope = captureScope(state);
          state.activeSheet     = null;
          state.activeSheetData = {};
          await (withCurrentContext(state.calendarView === "week" ? loadWeekData() : loadTodayData(), contextScope));
          render(app);
        })).catch(owned(() => showToast("Failed to log chore", "error")));
        break;
      }

      case "navigate-day":
        e.preventDefault();
        state.calendarDate = actionEl.dataset.date;
        state.todayDate = actionEl.dataset.date;
        loadTodayData().then(owned(() => render(app)));
        break;

      case "navigate-week":
        e.preventDefault();
        state.calendarDate = actionEl.dataset.date;
        loadWeekData().then(owned(() => render(app)));
        break;

      case "open-pick-chore-sheet":
        e.preventDefault();
        state.activeSheet = "pick-chore";
        state.activeSheetData = {
          date: actionEl.dataset.date,
          hour: actionEl.dataset.hour ? parseInt(actionEl.dataset.hour, 10) : null,
        };
        render(app);
        break;

      case "schedule-chore-here": {
        e.preventDefault();
        const choreId  = parseInt(actionEl.dataset.choreId, 10);
        const slotDate = actionEl.dataset.date || state.activeSheetData?.date || null;
        const hour     = state.activeSheetData?.hour ?? null;

        // From the Schedule tab (no hour preset), open a configure sheet so the
        // user can set time and recurrence before submitting.
        if (hour == null) {
          const chore = (state.chores || []).find(c => c.id === choreId);
          if (!chore) break;
          const timeInput = document.querySelector("#sheet-time");
          const presetTime = timeInput?.value || null;
          const presetFreq = readSheetFreq("sheet", slotDate);
          state.activeSheet = "configure-schedule";
          state.activeSheetData = { choreId, date: slotDate, hour: null, presetTime, presetFreq };
          render(app);
          break;
        }

        // From the calendar day view (hour preset), schedule immediately.
        const timeInput    = document.querySelector("#sheet-time");
        const specificTime = timeInput?.value || null;
        const freqPayload  = readSheetFreq("sheet", slotDate);
        createSchedule({
          choreId,
          timePeriod:    "anytime",
          specificTime,
          isActive:      true,
          ...freqPayload,
        }).then(owned(async () => {
  const contextScope = captureScope(state);
          state.activeSheet = null;
          state.activeSheetData = {};
          state.schedules = await withCurrentContext(loadSchedules(), contextScope);
          await (withCurrentContext(state.calendarView === "week" ? loadWeekData() : loadTodayData(), contextScope));
          render(app);
        })).catch(owned(() => showToast("Failed to schedule chore", "error")));
        break;
      }

      case "close-sheet":
        e.preventDefault();
        state.activeSheet = null;
        state.activeSheetData = {};
        render(app);
        break;

      case "save-configure-schedule": {
        e.preventDefault();
        const choreId      = parseInt(actionEl.dataset.choreId, 10);
        const slotDate     = actionEl.dataset.date || null;
        const timeInput    = document.querySelector("#config-sheet-time");
        const specificTime = timeInput?.value || null;
        const freqPayload  = readSheetFreq("config-sheet", slotDate);
        createSchedule({
          choreId,
          timePeriod:    "anytime",
          specificTime,
          isActive:      true,
          ...freqPayload,
        }).then(owned(async () => {
  const contextScope = captureScope(state);
          state.activeSheet = null;
          state.activeSheetData = {};
          state.schedules = await withCurrentContext(loadSchedules(), contextScope);
          await withCurrentContext(reloadViewData(), contextScope);
          render(app);
        })).catch(owned(() => showToast("Failed to schedule chore", "error")));
        break;
      }

      case "save-schedule-edit": {
        e.preventDefault();
        const scheduleId   = parseInt(actionEl.dataset.scheduleId, 10);
        const timeInput    = document.querySelector("#edit-sheet-time");
        const specificTime = timeInput?.value || null;
        const freqPayload  = readSheetFreq("edit-sheet", state.calendarDate);
        updateSchedule(scheduleId, { specificTime, ...freqPayload })
          .then(owned(async () => {
  const contextScope = captureScope(state);
            state.activeSheet     = null;
            state.activeSheetData = {};
            state.schedules = await withCurrentContext(loadSchedules(), contextScope);
            await withCurrentContext(reloadViewData(), contextScope);
            render(app);
          })).catch(owned(() => showToast("Failed to update schedule", "error")));
        break;
      }

      case "delete-schedule": {
        e.preventDefault();
        const scheduleId = parseInt(actionEl.dataset.scheduleId, 10);
        deleteSchedule(scheduleId)
          .then(owned(async () => {
  const contextScope = captureScope(state);
            state.activeSheet     = null;
            state.activeSheetData = {};
            state.schedules = await withCurrentContext(loadSchedules(), contextScope);
            await withCurrentContext(reloadViewData(), contextScope);
            render(app);
          })).catch(owned(() => showToast("Failed to remove schedule", "error")));
        break;
      }

      case "home-tap-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.homeChoreId, 10);
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (!chore) break;
        state.activeSheet     = "home-log";
        state.activeSheetData = { choreId };
        render(app);
        break;
      }

      case "home-remove-chore": {
        e.preventDefault();
        e.stopPropagation();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        state.activeSheet = "confirm-remove-home-chore";
        state.activeSheetData = { choreId };
        render(app);
        break;
      }

      case "confirm-remove-home-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const newHidden = [...new Set([...(state.hiddenHomeChoreIDs || []), choreId])];
        state.activeSheet = null;
        state.activeSheetData = {};
        // Optimistically update state and re-render; persist in the background.
        saveHiddenHomeChores(state, newHidden).then(owned(() => render(app)));
        render(app);
        break;
      }

      case "exit-jiggle-mode":
        e.preventDefault();
        state.jiggleMode = false;
        render(app);
        break;

      // ── Chores tab management ─────────────────────────────────────────────

      case "switch-home-view": {
        e.preventDefault();
        state.currentRoute = "/";
        state.homeView = actionEl.dataset.view || "log";
        state.jiggleMode = false;
        loadLatestLogsData().then(owned(() => render(app)));
        break;
      }

      case "chore-add": {
        e.preventDefault();
        state.activeSheet = "chore-edit";
        state.activeSheetData = { choreId: null };
        render(app);
        break;
      }

      case "chore-edit": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        state.activeSheet = "chore-edit";
        state.activeSheetData = { choreId };
        render(app);
        Promise.all([loadNotificationPrefs(), loadChoreReminderPrefsData()]).then(owned(() => render(app)));
        break;
      }

      case "chore-toggle-home": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const hidden = new Set(state.hiddenHomeChoreIDs || []);
        if (hidden.has(choreId)) {
          hidden.delete(choreId);
        } else {
          hidden.add(choreId);
        }
        const newHidden = [...hidden];
        saveHiddenHomeChores(state, newHidden).then(owned(() => render(app)));
        render(app);
        break;
      }

      case "save-chore": {
        e.preventDefault();
        const isNew = actionEl.dataset.isNew === "true";
        const choreId = actionEl.dataset.choreId ? parseInt(actionEl.dataset.choreId, 10) : null;
        const nameEl = document.querySelector("#chore-edit-name");
        const name = (nameEl?.value || "").trim();
        if (!name) {
          nameEl?.focus();
          break;
        }
        const iconEl = document.querySelector("#chore-icon-input");
        const icon = (iconEl?.value || "").trim() || "📋";
        // Read the selected color swatch.
        const selectedSwatch = document.querySelector(".color-swatch--selected");
        const color = selectedSwatch?.dataset?.color || "#2E86AB";
        // Collect indicator labels (skip empty ones).
        const indicatorLabels = [...document.querySelectorAll(".indicator-label-input")]
          .map(el => el.value.trim())
          .filter(v => v.length > 0);
        // Collect default indicators by matching checked boxes to their index's label.
        const indicatorDefaults = [];
        document.querySelectorAll(".indicator-chip-row").forEach(row => {
          const cb = row.querySelector("[data-action='toggle-indicator-default']");
          const input = row.querySelector(".indicator-label-input");
          if (cb && cb.checked && input) {
            const val = input.value.trim();
            if (val) indicatorDefaults.push(val);
          }
        });

        // Metric config (Phase 3): type + optional amount unit.
        const metricType = document.querySelector("#chore-metric-type")?.value || "none";
        const metricUnit = metricType === "amount"
          ? ((document.querySelector("#chore-metric-unit")?.value || "").trim() || "mL")
          : "";
        // Subjects (Phase 5.5): collect non-empty subject tags.
        const subjects = [...document.querySelectorAll(".subject-label-input")]
          .map(el => el.value.trim())
          .filter(v => v.length > 0);

        // Visibility (private household tasks): only owners/admins can set.
        const memberSelf = (state.members || []).find(m => m.userId === state.user?.id);
        const isAdmin = memberSelf?.role === "owner" || memberSelf?.role === "admin";
        const visEl = document.querySelector('input[name="chore-visibility"]:checked');
        const visibility = visEl?.value;
        if (isNew) {
          const followUpEnabled = document.querySelector("[data-action='toggle-followup-enabled']")?.checked ?? true;
          const body = { name, icon, color, category: "custom", indicatorLabels, indicatorDefaults, followUpEnabled, metricType, metricUnit, subjects };
          if (isAdmin && visibility) body.visibility = visibility;
          apiFetch("/api/chores", {
            method: "POST",
            body: JSON.stringify(body),
          }).then(owned(async ({ data }) => {
  const contextScope = captureScope(state);
            const newChore = data?.chore;
            if (!newChore) { showToast("Failed to create chore", "error"); return; }
            state.activeSheet = null;
            state.activeSheetData = {};
            await withCurrentContext(loadChoreData(), contextScope);
            // Append new chore to order so it appears at the bottom.
            if (newChore.id) {
              const newOrder = [...(state.choreOrder || []), newChore.id];
              await withCurrentContext(saveChoreOrder(state, newOrder), contextScope);
            }
            render(app);
            showToast(`${icon} ${name} added`, "success");
          })).catch(owned(() => showToast("Failed to create chore", "error")));
        } else {
          const chore = (state.chores || []).find(c => c.id === choreId);
          const oldVis = chore?.visibility || "household";
          const newVis = isAdmin && visibility ? visibility : oldVis;
          if (oldVis !== newVis) {
            const toPrivate = newVis === "admins";
            const msg = toPrivate
              ? `Make "${chore?.name || name}" Admins only?\n\nIt will disappear, along with its activity and statistics, for regular members. Information they saw while it was shared cannot be recalled.`
              : `Make "${chore?.name || name}" visible to everyone?\n\nAll members will gain access to the entire retained history.`;
            if (!confirm(msg)) break;
          }
          const followUpEnabledEl = document.querySelector("[data-action='toggle-followup-enabled']");
          const followUpEnabled = followUpEnabledEl?.checked;
          const body = { name, icon, color, indicatorLabels, indicatorDefaults, followUpEnabled, metricType, metricUnit, subjects };
          if (isAdmin && visibility) body.visibility = visibility;
          apiFetch(`/api/chores/${choreId}`, {
            method: "PATCH",
            body: JSON.stringify(body),
          }).then(owned(async ({ data, response }) => {
  const contextScope = captureScope(state);
            if (!response.ok) {
              const msg = data?.error || "Failed to update chore";
              showToast(msg, "error");
              // If 404, the task may have become private and viewer lost access; reload.
              if (response.status === 404) {
                state.activeSheet = null;
                state.activeSheetData = {};
                await withCurrentContext(loadChoreData(), contextScope);
                render(app);
              }
              return;
            }
            const updated = data?.chore;
            if (updated) {
              const idx = (state.chores || []).findIndex(c => c.id === choreId);
              if (idx >= 0) state.chores[idx] = updated;
              else state.chores.push(updated);
              // If visibility changed to private, ensure member assignments cleared are reflected in schedules
              try { state.schedules = await withCurrentContext(loadSchedules(), contextScope); } catch {}
            } else {
              await withCurrentContext(loadChoreData(), contextScope);
            }
            state.activeSheet = null;
            state.activeSheetData = {};
            render(app);
            if (data?.clearedAssignments) {
              showToast(`Chore updated — ${data.clearedAssignments} assignment(s) cleared`, "info");
            } else {
              showToast("Chore updated", "success");
            }
          })).catch(owned(() => showToast("Failed to update chore", "error")));
        }
        break;
      }

      case "delete-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (!chore) break;
        // eslint-disable-next-line no-alert
        if (!confirm(`Delete "${chore.name}"? This cannot be undone.`)) break;
        apiFetch(`/api/chores/${choreId}`, { method: "DELETE" })
          .then(owned(async ({ response }) => {
  const contextScope = captureScope(state);
            if (!response.ok) { showToast("Cannot delete this chore", "error"); return; }
            state.activeSheet = null;
            state.activeSheetData = {};
            // Remove from chore order.
            state.choreOrder = (state.choreOrder || []).filter(id => id !== choreId);
            // Remove from hidden list.
            state.hiddenHomeChoreIDs = (state.hiddenHomeChoreIDs || []).filter(id => id !== choreId);
            await withCurrentContext(loadChoreData(), contextScope);
            render(app);
            showToast("Chore deleted", "info");
          }))
          .catch(owned(() => showToast("Failed to delete chore", "error")));
        break;
      }

      case "restore-chore-default": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const chore = (state.chores || []).find(c => c.id === choreId);
        if (!chore) break;
        // eslint-disable-next-line no-alert
        if (!confirm(`Restore "${chore.name}" to its original default values?`)) break;
        apiFetch(`/api/chores/${choreId}/restore-default`, { method: "POST" })
          .then(owned(async ({ response }) => {
  const contextScope = captureScope(state);
            if (!response.ok) { showToast("Could not restore default", "error"); return; }
            state.activeSheet = null;
            state.activeSheetData = {};
            await withCurrentContext(loadChoreData(), contextScope);
            render(app);
            showToast("Restored to default", "success");
          }))
          .catch(owned(() => showToast("Failed to restore default", "error")));
        break;
      }

      // ── Subject picker (Phase 5.5) ───────────────────────────────────────

      case "pick-subject": {
        e.preventDefault();
        const wasOn = actionEl.getAttribute("aria-pressed") === "true";
        // Single-select: clear all, then set this one unless it was already on.
        document.querySelectorAll(".subject-chip").forEach(chip => {
          chip.classList.remove("subject-chip--on");
          chip.setAttribute("aria-pressed", "false");
        });
        if (!wasOn) {
          actionEl.classList.add("subject-chip--on");
          actionEl.setAttribute("aria-pressed", "true");
        }
        break;
      }

      // ── Recent-value volume chips (Phase 5.3) ────────────────────────────

      case "set-recent-volume": {
        e.preventDefault();
        const ml = parseInt(actionEl.dataset.ml, 10);
        if (isNaN(ml)) break;
        // Fill the plain volume input if present.
        const plain = document.querySelector("#log-volume");
        const chore = state.chores.find(c => c.id === state.activeSheetData?.choreId);
        const setAmount = input => {
          if (input.tagName === 'SELECT' && ![...input.options].some(o => o.value === String(ml))) {
            const option = document.createElement('option'); option.value = String(ml);
            option.textContent = formatAmount(ml,chore || {},state.volumeUnit); input.appendChild(option);
          }
          input.value = String(ml);
        };
        if (plain) setAmount(plain);
        // Fill only per-indicator volume selects whose type is already on.
        // Recent amounts must not change the user's type selection.
        document.querySelectorAll(".indicator-row").forEach(row => {
          const select = row.querySelector(".indicator-volume-select");
          const chip = row.querySelector("[data-action='toggle-indicator']");
          if (!select || !chip || chip.getAttribute("aria-pressed") !== "true") return;
          setAmount(select);
          select.style.display = "";
        });
        actionEl.classList.add("volume-recent-chip--active");
        break;
      }

      // ── Duration timer (Phase 5.2) ───────────────────────────────────────

      case "start-timer": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        if (isNaN(choreId)) break;
        const draft = state.activeSheetData;
        const timer = { choreId, choreName:actionEl.dataset.choreName || "", choreIcon:actionEl.dataset.choreIcon || "⏱", startedAt:Date.now() };
        startTimer(timer, actionScope.origin).then(actionScope.guard(saved => {
          state.activeTimer = saved;
          if (state.activeSheetData === draft) { state.activeSheet = null; state.activeSheetData = {}; }
          render(app);
          showToast("Timer started", "info");
        })).catch(actionScope.guard(err => showToast(err.message || "Could not save the timer. Please retry.", "error")));
        break;
      }

      case "stop-timer": {
        e.preventDefault();
        const shown = state.activeTimer;
        if (!shown || shown.saving) break;
        shown.saving = true;
        renderTimerChip();
        stopTimer(shown.id, actionScope.origin).then(owned(async t => {
  const contextScope = captureScope(state);
          if (!actionScope.current()) return;
          if (!t) { if (state.activeTimer?.id === shown.id) state.activeTimer = null; return; }
          t.saving = true;
          state.activeTimer = t;
          renderTimerChip();
          const data = await withCurrentContext(logChore(t.choreId, "", "", [], null, null, null, null, {}, 0, null, null, null, null, null, {submission:t.submission}), contextScope);
          await withCurrentContext(clearFinishedTimer(t.id, actionScope.origin), contextScope);
          if (!actionScope.current()) return;
          if (state.activeTimer?.id === t.id) state.activeTimer = null;
          await withCurrentContext(Promise.all([loadTodayData(), loadLatestLogsData(), hydratePendingLogs()]), contextScope);
          if (!actionScope.current()) return;
          render(app);
          showToast(data.queued ? `Saved ${formatElapsed(elapsedSeconds(t))} on this device. Waiting to sync.` : `Logged ${formatElapsed(elapsedSeconds(t))}`, data.queued ? "info" : "success");
        })).catch(actionScope.guard(err => showToast(err.message || "Timer is saved. Retry when ready.", "error")))
          .finally(owned(() => {
            if (!actionScope.current()) return;
            if (state.activeTimer?.id === shown.id) state.activeTimer.saving = false;
            renderTimerChip();
          }));
        break;
      }

      // ── Per-day diary notes (Phase 5.4) ──────────────────────────────────

      case "edit-day-note": {
        e.preventDefault();
        const date = actionEl.dataset.date;
        if (!date) break;
        state.activeSheet = "day-note";
        state.activeSheetData = { date };
        render(app);
        break;
      }

      case "save-day-note": {
        e.preventDefault();
        const date = actionEl.dataset.date;
        const note = (document.querySelector("#day-note-input")?.value || "").trim();
        apiFetch(`/api/day-notes/${date}`, {
          method: "PUT",
          body: JSON.stringify({ note }),
        }).then(owned(({ response }) => {
          if (!response.ok) { showToast("Failed to save note", "error"); return; }
          state.dayNotes = state.dayNotes || {};
          if (note) state.dayNotes[date] = note;
          else delete state.dayNotes[date];
          state.activeSheet = null;
          state.activeSheetData = {};
          render(app);
        })).catch(owned(() => showToast("Failed to save note", "error")));
        break;
      }

      // ── Stats widgets (Phase 4) ──────────────────────────────────────────

      case "widget-add": {
        e.preventDefault();
        state.activeSheet = "widget-wizard";
        state.activeSheetData = { widgetDraft: { type: "total", metric: "count", period: "week" } };
        render(app);
        break;
      }

      case "widget-save": {
        e.preventDefault();
        const title = (document.querySelector("#widget-title")?.value || "").trim();
        const type = document.querySelector("#widget-presentation")?.value || "total";
        const metric = document.querySelector("#widget-metric")?.value || "count";
        // Period is chosen on the widget card (day/week/month toggle), not here.
        const period = "week";
        const choreIds = [...document.querySelectorAll("[data-action='widget-draft-chore']:checked")]
          .map(el => parseInt(el.dataset.choreId, 10))
          .filter(n => !isNaN(n));
        if (type !== "total" && choreIds.length === 0) {
          showToast("Pick at least one chore", "error");
          break;
        }
        const widget = { type, metric, period, choreIds, title: title || "Widget" };
        const widgets = [...(state.stats?.widgets || []), widget];
        saveStatsWidgets(state, widgets).then(owned(async (saved) => {
  const contextScope = captureScope(state);
          if (!saved) { showToast("Failed to add widget", "error"); return; }
          state.activeSheet = null;
          state.activeSheetData = {};
          await withCurrentContext(loadWidgetData(), contextScope);
          render(app);
          showToast("Widget added", "success");
        }));
        break;
      }

      case "widget-remove": {
        e.preventDefault();
        const id = actionEl.dataset.widgetId;
        const widgets = (state.stats?.widgets || []).filter(w => w.id !== id);
        saveStatsWidgets(state, widgets).then(owned((saved) => {
          if (!saved) { showToast("Failed to remove widget", "error"); return; }
          render(app);
        }));
        break;
      }

      case "widget-period": {
        e.preventDefault();
        const id = actionEl.dataset.widgetId;
        const period = actionEl.dataset.period;
        if (!id || !period) break;
        const current = (state.stats?.widgets || []).find(w => w.id === id);
        if (!current || current.period === period) break;
        const widgets = (state.stats.widgets || []).map(w => w.id === id ? { ...w, period } : w);
        // Persist the new period and refetch the affected widget's data.
        saveStatsWidgets(state, widgets).then(owned(async (saved) => {
  const contextScope = captureScope(state);
          if (!saved) { showToast("Failed to update widget", "error"); return; }
          await withCurrentContext(loadWidgetData(), contextScope);
          render(app);
        }));
        break;
      }

      // ── Chore sheet: inline interactions ─────────────────────────────────

      case "pick-chore-color": {
        // Update selected swatch without a full re-render.
        document.querySelectorAll(".color-swatch").forEach(el => {
          const isSelected = el.dataset.color === actionEl.dataset.color;
          el.classList.toggle("color-swatch--selected", isSelected);
          el.setAttribute("aria-pressed", String(isSelected));
        });
        // Update the icon preview background.
        const preview = document.querySelector("#chore-icon-preview");
        if (preview) preview.style.background = actionEl.dataset.color;
        break;
      }

      case "pick-chore-emoji": {
        const emoji = actionEl.dataset.emoji;
        const iconInput = document.querySelector("#chore-icon-input");
        const preview = document.querySelector("#chore-icon-preview");
        if (iconInput) iconInput.value = emoji;
        if (preview) preview.textContent = emoji;
        break;
      }

      case "toggle-chore-reminder": {
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        if (!choreId) break;
        const enabled = actionEl.checked;
        const pref = getChoreReminderPref(choreId) || { choreId, enabled: false, leadMinutes: state.notificationPrefs?.defaultReminderLeadMinutes ?? 10 };
        pref.enabled = enabled;
        saveChoreReminderPref(choreId, { enabled, leadMinutes: pref.leadMinutes })
          .then(owned(updated => {
            const idx = (state.choreReminderPrefs || []).findIndex(p => p.choreId === choreId);
            if (idx >= 0) {
              state.choreReminderPrefs[idx] = updated;
            } else {
              state.choreReminderPrefs = [...(state.choreReminderPrefs || []), updated];
            }
            render(app);
          }))
          .catch(owned(() => {
            actionEl.checked = !enabled;
          }));
        break;
      }

      case "add-indicator-label": {
        e.preventDefault();
        const list = document.querySelector("#indicator-labels-list");
        if (!list) break;
        const idx = list.children.length;
        const row = document.createElement("div");
        row.className = "indicator-chip-row";
        row.dataset.index = idx;
        row.innerHTML = `<input type="text" class="indicator-label-input input" data-index="${idx}"
          value="" placeholder="e.g. 💩 poo" maxlength="30" />
          <label class="indicator-default-toggle" title="Preselect when logging">
            <input type="checkbox" data-action="toggle-indicator-default" data-index="${idx}" />
            <span class="indicator-default-label">default</span>
          </label>
          <button type="button" class="indicator-remove-btn"
            data-action="remove-indicator-label" data-index="${idx}"
            aria-label="Remove label">×</button>`;
        list.appendChild(row);
        row.querySelector("input")?.focus();
        break;
      }

      case "remove-indicator-label": {
        e.preventDefault();
        const row = actionEl.closest(".indicator-chip-row");
        if (row) row.remove();
        break;
      }

      case "add-subject-label": {
        e.preventDefault();
        const list = document.querySelector("#subject-labels-list");
        if (!list) break;
        const idx = list.children.length;
        const row = document.createElement("div");
        row.className = "indicator-chip-row";
        row.dataset.subjectIndex = idx;
        row.innerHTML = `<input type="text" class="subject-label-input input" data-subject-index="${idx}"
          value="" placeholder="e.g. 👶 Alice" maxlength="30" />
          <button type="button" class="indicator-remove-btn"
            data-action="remove-subject-label" data-subject-index="${idx}"
            aria-label="Remove subject">×</button>`;
        list.appendChild(row);
        row.querySelector("input")?.focus();
        break;
      }

      case "remove-subject-label": {
        e.preventDefault();
        const row = actionEl.closest(".indicator-chip-row");
        if (row) row.remove();
        break;
      }

      case "load-more-history":
        e.preventDefault();
        void loadMoreHistoryPage();
        break;
      case "retry-activity":
        e.preventDefault();
        void loadActivityPage();
        break;

      case "history-filter-all": {
        e.preventDefault();
        // Clear the filter: show all activity, nothing highlighted.
        if (setActivityChoreFilter(state,null)) void loadActivityPage();
        render(app);
        break;
      }

      case "toggle-history-filter": {
        e.preventDefault();
        state.historyFilterOpen = !state.historyFilterOpen;
        render(app);
        break;
      }

      case "history-filter-chore": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        // Additive selection: build up the set of chores to show. An empty
        // set means no filter (show everything).
        const selected = Array.isArray(state.historyChoreFilter) ? [...state.historyChoreFilter] : [];
        const idx = selected.indexOf(choreId);
        if (idx === -1) {
          selected.push(choreId);
        } else {
          selected.splice(idx, 1);
        }
        if (setActivityChoreFilter(state,selected.length === 0 ? null : selected)) void loadActivityPage();
        render(app);
        break;
      }

      case "toggle-push-enabled": {
        e.preventDefault();
        const isChecked = actionEl.checked;
        const enabledTypes = state.notificationPrefs?.enabledPushTypes || [];
        const pushEnabled = isChecked;
        saveNotificationPreferences({ pushEnabled, enabledPushTypes: isChecked ? enabledTypes : [] })
          .then(owned(data => {
            state.notificationPrefs = data.preferences;
            render(app);
          }))
          .catch(owned(() => {
            actionEl.checked = !isChecked;
          }));
        break;
      }

      case "enable-notifications": {
        e.preventDefault();
        if (typeof Notification === 'undefined') break;
        Notification.requestPermission().then(owned(result => {
          if (result === 'granted') {
            maybeSubscribePush().catch(() => {});
          }
        })).catch(() => {});
        break;
      }

      case "toggle-notif-pref": {
        e.preventDefault();
        const notifType = actionEl.dataset.notifType;
        const isChecked = actionEl.checked;
        let enabledTypes = [...(state.notificationPrefs?.enabledPushTypes || [])];
        const prevPushEnabled = state.notificationPrefs?.pushEnabled;
        const allTypes = (state.availableNotificationTypes || []).map(t => t.type);

        // If no explicit preferences have been set yet (pushEnabled defaults
        // to true and enabledTypes is empty), start from all types.
        if (enabledTypes.length === 0 && prevPushEnabled !== false) {
          enabledTypes = [...allTypes];
        }

        if (isChecked) {
          if (!enabledTypes.includes(notifType)) enabledTypes.push(notifType);
        } else {
          enabledTypes = enabledTypes.filter(t => t !== notifType);
        }

        const pushEnabled = enabledTypes.length > 0;
        saveNotificationPreferences({ enabledPushTypes: enabledTypes, pushEnabled })
          .then(owned(data => {
            state.notificationPrefs = data.preferences;
            render(app);
          }))
          .catch(owned(() => {
            actionEl.checked = !isChecked;
          }));
          break;
      }

      case "chore-analytics-period": {
        e.preventDefault();
        const choreId = parseInt(actionEl.dataset.choreId, 10);
        const period = actionEl.dataset.period;
        if (!choreId || !period) break;
        state.stats = state.stats || {};
        state.stats.choreAnalyticsPeriod = state.stats.choreAnalyticsPeriod || {};
        if (state.stats.choreAnalyticsPeriod[choreId] === period) break;
        state.stats.choreAnalyticsPeriod[choreId] = period;
        state.stats.choreTimeSeries = state.stats.choreTimeSeries || {};
        void refreshStatsResource('chore', choreId);
        break;
      }

      case "stats-baby-period": {
        e.preventDefault();
        const period = actionEl.dataset.period;
        const type = actionEl.dataset.type;
        if (!period || !type) break;
        state.stats = state.stats || {};
        if (type === "feed") {
          state.stats.feedBabyPeriod = period;
        } else if (type === "change") {
          state.stats.changeBabyPeriod = period;
        } else {
          break;
        }
        void refreshStatsResource('baby', type);
        break;
      }

      case "toggle-feeding-gaps-info": {
        e.preventDefault();
        state.stats = state.stats || {};
        state.stats.feedingGapsExplainerVisible = !state.stats.feedingGapsExplainerVisible;
        render(app);
        break;
      }

      case "chart-tap": {
        e.preventDefault();
        const g = e.target.closest("[data-action=\"chart-tap\"]");
        if (!g) break;
        const barIdx = g.dataset.bar;
        const svg = g.closest("svg");
        const val = svg?.querySelector(`.chart-bar-val[data-bar="${barIdx}"]`);
        if (!val) break;
        const visible = val.classList.contains("chart-bar-val--visible");
        document.querySelectorAll(".chart-bar-val--visible").forEach(el => el.classList.remove("chart-bar-val--visible"));
        if (!visible) val.classList.add("chart-bar-val--visible");
        break;
      }

      case "scatter-tap": {
        e.preventDefault();
        const g = e.target.closest("[data-action=\"scatter-tap\"]");
        if (!g) break;
        const gapIdx = g.dataset.gap;
        const svg = g.closest("svg");
        const tip = svg?.querySelector(`.scatter-tooltip[data-gap="${gapIdx}"]`);
        if (!tip) break;
        const visible = tip.classList.contains("scatter-tooltip--visible");
        document.querySelectorAll(".scatter-tooltip--visible").forEach(el => el.classList.remove("scatter-tooltip--visible"));
        if (!visible) tip.classList.add("scatter-tooltip--visible");
        break;
      }

      case "set-volume-unit": {
        e.preventDefault();
        const unit = actionEl.dataset.unit === "oz" ? "oz" : "ml";
        if (unit === state.volumeUnit) break;
        // saveVolumeUnit updates state optimistically (and rolls back on
        // failure); render now for snappy feedback and again on completion.
        saveVolumeUnit(state, unit).then(owned(() => render(app)));
        render(app);
        break;
      }

      case "toggle-hide-notification-badge": {
        // No preventDefault: the checkbox keeps its native toggled state so
        // the UI reflects the tap instantly. saveHideNotificationBadge
        // applies the value optimistically and rolls back if the PATCH
        // fails; the completion render redraws the toggle from persisted
        // state either way.
        const hide = actionEl.checked;
        if (hide === state.hideNotificationBadge) break;
        const saved = saveHideNotificationBadge(state, hide);
        updateTopBar();
        saved.then(owned(() => {
          updateTopBar();
          render(app);
        }));
        break;
      }

      case "heatmap-tap": {
        // Touch devices can't hover the cell's title attribute, so reveal a
        // positioned tooltip on tap. Tapping the same cell again, or anywhere
        // else, dismisses it (see the top-of-handler dismissal below).
        e.preventDefault();
        const cell = e.target.closest("[data-action=\"heatmap-tap\"]");
        if (!cell) break;
        const wrap = cell.closest(".heatmap-wrap");
        const tip = wrap?.querySelector(".heatmap-tooltip");
        if (!tip) break;
        const alreadyFor = tip.dataset.date === cell.dataset.date;
        const wasVisible = tip.classList.contains("heatmap-tooltip--visible");
        if (alreadyFor && wasVisible) {
          tip.classList.remove("heatmap-tooltip--visible");
          break;
        }
        const label = `${new Date(cell.dataset.date + "T00:00:00").toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" })} · ${cell.dataset.count} chore${cell.dataset.count === "1" ? "" : "s"}`;
        tip.textContent = label;
        tip.dataset.date = cell.dataset.date;
        const wrapRect = wrap.getBoundingClientRect();
        const cellRect = cell.getBoundingClientRect();
        tip.style.left = `${cellRect.left - wrapRect.left + cellRect.width / 2}px`;
        tip.style.top = `${cellRect.top - wrapRect.top - 4}px`;
        tip.classList.add("heatmap-tooltip--visible");
        break;
      }

      case "top-chores-user": {
        e.preventDefault();
        const uid = parseInt(actionEl.dataset.userId, 10);
        if (!uid) break;
        state.stats.topChoresUserId = uid;
        void refreshStatsResource('top-chores');
        break;
      }

      case "stats-period": {
        e.preventDefault();
        const section = actionEl.dataset.section, period = actionEl.dataset.period;
        const fields = {leaderboard:'leaderboardPeriod', 'top-chores':'topChoresPeriod', categories:'categoriesPeriod', chores:'choreStatsPeriod'};
        const field = fields[section];
        if (!field || !period || state.stats[field] === period) break;
        state.stats[field] = period;
        void refreshStatsResource(section);
        break;
      }

      case 'retry-stats':
        e.preventDefault();
        loadAllStatsData().then(owned(() => render(app)));
        render(app);
        break;

      case "stats-feeding-gaps-quick": {
        e.preventDefault();
        const days = parseInt(actionEl.dataset.days, 10);
        state.stats = state.stats || {};
        const fmt = d => `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,"0")}-${String(d.getDate()).padStart(2,"0")}`;
        const endDate = new Date();
        const startDate = new Date(endDate);
        startDate.setDate(startDate.getDate() - (days - 1));
        state.stats.feedingGapsEnd = fmt(endDate);
        state.stats.feedingGapsStart = fmt(startDate);
        void refreshStatsResource('gaps');
        break;
      }

      case "toggle-customize-stats": {
        e.preventDefault();
        state.stats = state.stats || {};
        state.stats.customizeOpen = !state.stats.customizeOpen;
        render(app);
        break;
      }
    }
  });

  // ── Stats customize drag-and-drop reorder ────────────────────────────────────
  let dragSection = null;
  document.addEventListener("dragstart", (e) => {
    const row = e.target.closest(".customize-row");
    if (!row) return;
    dragSection = row.dataset.section;
    row.classList.add("customize-row--dragging");
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", dragSection);
  });
  document.addEventListener("dragend", (e) => {
    const row = e.target.closest(".customize-row");
    if (row) row.classList.remove("customize-row--dragging");
    document.querySelectorAll(".customize-row.drag-over").forEach(el => el.classList.remove("drag-over"));
    dragSection = null;
  });
  document.addEventListener("dragover", (e) => {
    const row = e.target.closest(".customize-row");
    if (!row) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    row.classList.add("drag-over");
  });
  document.addEventListener("dragleave", (e) => {
    const row = e.target.closest(".customize-row");
    if (!row) return;
    row.classList.remove("drag-over");
  });
  document.addEventListener("drop", (e) => {
    const row = e.target.closest(".customize-row");
    if (!row || !dragSection) return;
    e.preventDefault();
    row.classList.remove("drag-over");
    const targetSection = row.dataset.section;
    if (targetSection === dragSection) return;
    state.stats = state.stats || {};
    state.stats.sectionOrder = state.stats.sectionOrder || [];
    const cur = [...state.stats.sectionOrder];
    const srcIdx = cur.indexOf(dragSection);
    const dstIdx = cur.indexOf(targetSection);
    if (srcIdx === -1) {
      if (dstIdx === -1) {
        cur.push(dragSection, targetSection);
      } else {
        cur.splice(dstIdx, 0, dragSection);
      }
    } else {
      cur.splice(srcIdx, 1);
      if (dstIdx === -1) {
        cur.push(dragSection);
      } else {
        const newDst = cur.indexOf(targetSection);
        cur.splice(newDst, 0, dragSection);
      }
    }
    const all = [...new Set([...cur, ...STATS_SECTIONS])];
    state.stats.sectionOrder = all;
    saveStatsSectionOrder(state, all).then(owned(() => render(app)));
  });

  // Prevent taps/clicks on select elements inside member rows from
  // propagating to the <details> / <summary> and toggling them closed.
  // capture=true so we intercept before the browser's native toggle.
  document.addEventListener("touchstart", (e) => {
    if (e.target.closest(".member-row-details select")) {
      e.stopPropagation();
    }
  }, { capture: true });
  document.addEventListener("click", (e) => {
    if (e.target.closest(".member-row-details select")) {
      e.stopPropagation();
    }
  }, { capture: true });

  // ── Star rating keyboard support (a11y) ────────────────────────────────────
  // The rating widget is role="slider"; support arrow keys / Home / End so it
  // is operable without a pointer. Increments are half-stars (5 tenths).
  document.addEventListener("keydown", (e) => {
    const container = e.target.closest?.(".star-rating");
    if (!container) return;
    const cur = parseInt(container.dataset.rating || "0", 10) || 0;
    let next = cur;
    switch (e.key) {
      case "ArrowRight":
      case "ArrowUp":   next = cur + 5; break;
      case "ArrowLeft":
      case "ArrowDown": next = cur - 5; break;
      case "Home":      next = 0; break;
      case "End":       next = 50; break;
      default: return;
    }
    e.preventDefault();
    setStarRatingValue(container, next);
  });

  // ── History text search (debounced) ────────────────────────────────────────
  let historySearchTimer = null;
  document.addEventListener("input", (e) => {
    const el = e.target.closest?.('[data-action="history-search"]');
    if (!el) return;
    state.historySearch = el.value || "";
    clearTimeout(historySearchTimer);
    prepareActivity(state);
    const scope = captureScope(state, null, () => [state.historySearch,state.currentRoute]);
    historySearchTimer = setTimeout(() => {
      if (scope.current()) void loadActivityPage();
    }, 300);
  });

  // ── Frequency selector: show/hide weekday pill row ─────────────────────────
  // Uses "change" (not "click") because <select> fires "change" on selection.
  document.addEventListener("change", (e) => {
    const actionEl = e.target.closest("[data-action]");
    if (actionEl?.dataset?.action === "export-range") {
      state.exportRange = {...(state.exportRange || {}), [actionEl.dataset.field]:actionEl.value};
      state.exportError = null;
      state.exportStatus = null;
      return;
    }

    if (actionEl?.dataset?.action === "change-frequency") {
      const sheet   = actionEl.closest(".bottom-sheet");
      const freqVal = actionEl.value;
      const wkRow   = sheet?.querySelector(".sheet-weekday-row");
      const intvRow = sheet?.querySelector(".sheet-interval-row");
      const endRow  = sheet?.querySelector(".sheet-end-date-row");
      if (wkRow)   wkRow.hidden   = (freqVal !== "weekly");
      if (intvRow) intvRow.hidden = (freqVal !== "every_n_days");
      if (endRow)  endRow.hidden  = (freqVal === "once");
    }
      if (actionEl?.dataset?.action === "busy-hours-filter") {
        const filter = actionEl.dataset.filter;
        state.stats = state.stats || {};
        state.stats.busyHoursFilter = state.stats.busyHoursFilter || {};
        const raw = actionEl.value;
        if (filter === "start" || filter === "end") {
          state.stats.busyHoursFilter[filter] = raw || null;
        } else {
          state.stats.busyHoursFilter[filter] = raw ? parseInt(raw, 10) : null;
        }
        void refreshStatsResource('busy-hours');
      }
      if (actionEl?.dataset?.action === "stats-feeding-gaps-date") {
        const field = actionEl.dataset.field;
        state.stats = state.stats || {};
        if (field === "start") state.stats.feedingGapsStart = actionEl.value || "";
        if (field === "end") state.stats.feedingGapsEnd = actionEl.value || "";
        const s = state.stats.feedingGapsStart;
        const e = state.stats.feedingGapsEnd;
        if (s && e) {
          void refreshStatsResource('gaps');
        }
      }
    if (actionEl?.dataset?.action === "pick-metric-type") {
      const sheet = actionEl.closest(".bottom-sheet");
      const unitRow = sheet?.querySelector(".chore-metric-unit-row");
      if (unitRow) unitRow.classList.toggle("hidden", actionEl.value !== "amount");
    }
    if (actionEl?.dataset?.action === "toggle-stats-section") {
      const section = actionEl.dataset.section;
      if (!section) return;
      state.stats = state.stats || {};
      state.stats.sectionHidden = state.stats.sectionHidden || [];
      const hidden = state.stats.sectionHidden;
      if (actionEl.checked) {
        state.stats.sectionHidden = hidden.filter(s => s !== section);
      } else {
        if (!hidden.includes(section)) {
          state.stats.sectionHidden = [...hidden, section];
        }
      }
      saveStatsSectionHidden(state, state.stats.sectionHidden).then(owned(async () => {
        const scope = captureScope(state);
        await loadAllStatsData();
        if (scope.current()) render(app);
      }));
      render(app);
    }
    if (actionEl?.dataset?.action === "update-member-role") {
      const userId = parseInt(actionEl.dataset.userId, 10);
      const newRole = actionEl.value;
      if (newRole === "owner") {
        const member = (state.members || []).find(m => m.userId === userId);
        const name = member ? (member.displayName || member.email) : "this member";
        // eslint-disable-next-line no-alert
        if (!confirm(`Transfer ownership to ${name}? You will become an admin.`)) return;
        transferOwnership(userId).then(owned(async (data) => {
  const contextScope = captureScope(state);
          if (data.status === "transferred") {
            await withCurrentContext(loadHouseholdData(), contextScope);
            render(app);
            showToast(`Ownership transferred to ${name}`, "info");
          } else {
            showToast(data.error || "Failed to transfer ownership", "error");
          }
        })).catch(owned(() => showToast("Failed to transfer ownership", "error")));
      } else {
        updateMemberRole(userId, newRole).then(owned(async (data) => {
  const contextScope = captureScope(state);
          if (data.status === "updated") {
            await withCurrentContext(loadHouseholdData(), contextScope);
            render(app);
          } else {
            showToast(data.error || "Failed to update role", "error");
          }
        })).catch(owned(() => showToast("Failed to update role", "error")));
      }
    }
    if (actionEl?.dataset?.action === "change-chore-reminder-lead") {
      const choreId = parseInt(actionEl.dataset.choreId, 10);
      if (!choreId) return;
      const leadMinutes = parseInt(actionEl.value, 10);
      if (isNaN(leadMinutes)) return;
      const pref = getChoreReminderPref(choreId) || { choreId, enabled: true, leadMinutes };
      pref.leadMinutes = leadMinutes;
      saveChoreReminderPref(choreId, { enabled: true, leadMinutes })
        .then(owned(updated => {
          const idx = (state.choreReminderPrefs || []).findIndex(p => p.choreId === choreId);
          if (idx >= 0) {
            state.choreReminderPrefs[idx] = updated;
          } else {
            state.choreReminderPrefs = [...(state.choreReminderPrefs || []), updated];
          }
          render(app);
        }))
        .catch(() => {});
    }
    if (actionEl?.dataset?.action === "change-default-reminder-lead") {
      const leadMinutes = parseInt(actionEl.value, 10);
      if (isNaN(leadMinutes)) return;
      saveNotificationPreferences({ defaultReminderLeadMinutes: leadMinutes })
        .then(owned(data => {
          state.notificationPrefs = data.preferences;
          render(app);
        }))
        .catch(() => {});
    }
  });

  // Keep the "Every N days" option label in sync as the user edits the interval.
  // Also keep the emoji preview in sync with the chore icon input.
  document.addEventListener("input", (e) => {
    const input = e.target;
    // Interval sync
    if (input.classList.contains("interval-input")) {
      const sheet = input.closest(".bottom-sheet");
      const sel   = sheet?.querySelector("[data-action='change-frequency']");
      if (!sel) return;
      const opt = sel.options[sel.selectedIndex];
      if (opt?.value !== "every_n_days") return;
      const n = Math.max(2, parseInt(input.value || "2", 10));
      opt.textContent = `Every ${n} days`;
      return;
    }
    // Emoji input sync: update the large icon preview.
    if (input.id === "chore-icon-input") {
      const preview = document.querySelector("#chore-icon-preview");
      if (preview) preview.textContent = input.value || "📋";
    }
    // Auto-initials: when a name field has data-autoinitials, populate the target
    // initials field if it's empty or still matches the auto-generated value.
    if (input.dataset.autoinitials) {
      const targetId = input.dataset.autoinitials;
      const initialsInput = document.getElementById(targetId);
      if (initialsInput) {
        const prevAuto = initialsInput.dataset.autoValue || "";
        if (!initialsInput.value || initialsInput.value === prevAuto) {
          const auto = generateInitials(input.value);
          initialsInput.value = auto;
          initialsInput.dataset.autoValue = auto;
        }
      }
    }
  });

  document.addEventListener("submit", (e) => {
    const form = e.target;
    const action = form.dataset.action;
    e.preventDefault();

    switch (action) {
      case "login":
        doLogin(form);
        break;
      case "register":
        doRegister(form);
        break;
      case "magic-link-request":
        doMagicLinkRequest(form);
        break;
      case "password-forgot":
        doForgotPassword(form);
        break;
      case "password-reset":
        doResetPassword(form);
        break;
      case "change-password":
        doChangePassword(form);
        break;
      case "create-household":
        doCreateHousehold(form);
        break;
      case "update-household":
        doUpdateHousehold(form);
        break;
      case "join-household":
        doJoinHousehold(form);
        break;
      case "new-chore-from-sheet":
        doCreateChoreFromSheet(form);
        break;
    }
  });

  if (state.user) await reloadAfterAuth();

  // ── PWA manifest shortcuts (?quicklog=…) ───────────────────────────────────
  // Long-pressing the home-screen icon exposes "Log feed", "Log chore", and
  // "Activity" shortcuts that deep-link via a start_url query param. Handle it
  // once here after bootstrap so the user lands one tap from logging.
  try {
    const params = new URLSearchParams(window.location.search);
    const quicklog = params.get("quicklog");
    const pushMatches = !params.has("pushUser") || sameOrigin(contextSnapshot(), {userId:Number(params.get("pushUser")), householdId:Number(params.get("pushHousehold"))});
    if (quicklog && pushMatches && state.user && state.household) {
      if (quicklog === "activity") {
        state.currentRoute = "/activity";
        state.activityView = "history";
        await loadActivityPage();
      } else {
        state.currentRoute = "/";
        state.homeView = "log";
        const chores = state.chores || [];
        let chore = null;
        if (quicklog === "feed-baby") {
          chore = chores.find(c => c.predefinedKey === "Feed Baby")
            || chores.find(c => c.name === "Feed Baby");
        } else if (quicklog.startsWith("chore:")) {
          // Deep link from a "Log now" notification action.
          const id = parseInt(quicklog.slice("chore:".length), 10);
          chore = chores.find(c => c.id === id) || null;
        }
        if (chore) {
          state.activeSheet = "home-log";
          state.activeSheetData = { choreId: chore.id };
        }
        // quicklog=chore (or a missing Feed Baby chore) simply lands on the
        // fast home grid, which is one tap from logging.
      }
      // Strip the query so a refresh doesn't reopen the sheet.
      window.history.replaceState({}, "", window.location.pathname);
    }
  } catch {}

  // ── Drag and drop ──────────────────────────────────────────────────────────
  document.addEventListener("dragstart", e => {
    clearTimeout(longPressTimer);
    // Sheet chore reorder drag — must check before calendar card drag because
    // sheet items have [data-reorder-chore-id] but not [data-drag-chore-id].
    const reorderItem = e.target.closest("[data-reorder-chore-id]");
    if (reorderItem) {
      const choreId = parseInt(reorderItem.dataset.reorderChoreId, 10);
      e.dataTransfer.setData("text/plain", JSON.stringify({ reorderChoreId: choreId }));
      e.dataTransfer.effectAllowed = "move";
      reorderItem.classList.add("sheet-chore-item--dragging");
      return;
    }
    // Chores-tab list reorder drag.
    const choresTabItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabItem) {
      const choreId = parseInt(choresTabItem.dataset.choresTabReorderId, 10);
      e.dataTransfer.setData("text/plain", JSON.stringify({ choresTabReorderId: choreId }));
      e.dataTransfer.effectAllowed = "move";
      choresTabItem.classList.add("chore-row--dragging");
      return;
    }
    // Home grid jiggle-mode reorder drag.
    const homeReorderItem = e.target.closest("[data-home-reorder-chore-id]");
    if (homeReorderItem) {
      const choreId = parseInt(homeReorderItem.dataset.homeReorderChoreId, 10);
      e.dataTransfer.setData("text/plain", JSON.stringify({ homeReorderChoreId: choreId }));
      e.dataTransfer.effectAllowed = "move";
      homeReorderItem.classList.add("home-chore-card--dragging");
      return;
    }
    // Calendar card drag.
    const card = e.target.closest("[data-drag-chore-id]");
    if (!card) return;
    e.dataTransfer.setData("text/plain", JSON.stringify({
      choreId:    parseInt(card.dataset.dragChoreId,    10),
      scheduleId: parseInt(card.dataset.dragScheduleId, 10) || null,
    }));
    card.classList.add("dragging");
  });

  document.addEventListener("dragend", e => {
    e.target.closest("[data-drag-chore-id]")?.classList.remove("dragging");
    e.target.closest("[data-reorder-chore-id]")?.classList.remove("sheet-chore-item--dragging");
    document.querySelectorAll(".sheet-chore-item--drag-over-top, .sheet-chore-item--drag-over-bottom")
      .forEach(el => el.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom"));
    e.target.closest("[data-home-reorder-chore-id]")?.classList.remove("home-chore-card--dragging");
    document.querySelectorAll(".home-chore-card--drag-over")
      .forEach(el => el.classList.remove("home-chore-card--drag-over"));
    e.target.closest("[data-chores-tab-reorder-id]")?.classList.remove("chore-row--dragging");
    document.querySelectorAll(".chore-row--drag-over-top, .chore-row--drag-over-bottom")
      .forEach(el => el.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom"));
  });

  document.addEventListener("dragover", e => {
    const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
    if (cell) { e.preventDefault(); cell.classList.add("drop-target"); }
    // Sheet chore reorder: show insert position indicator.
    const item = e.target.closest("[data-reorder-chore-id]");
    if (item) {
      e.preventDefault();
      const rect = item.getBoundingClientRect();
      const half = rect.top + rect.height / 2;
      // Clear indicators on all siblings, then set the right one on this item.
      item.closest(".sheet-chore-list")
        ?.querySelectorAll("[data-reorder-chore-id]")
        .forEach(el => el.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom"));
      item.classList.add(e.clientY < half
        ? "sheet-chore-item--drag-over-top"
        : "sheet-chore-item--drag-over-bottom");
    }
    // Chores-tab list reorder: show insert position indicator.
    const choresTabItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabItem) {
      e.preventDefault();
      const rect = choresTabItem.getBoundingClientRect();
      const half = rect.top + rect.height / 2;
      choresTabItem.closest("#chore-list")
        ?.querySelectorAll("[data-chores-tab-reorder-id]")
        .forEach(el => el.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom"));
      choresTabItem.classList.add(e.clientY < half
        ? "chore-row--drag-over-top"
        : "chore-row--drag-over-bottom");
    }
    // Home grid jiggle reorder: highlight target card.
    const homeItem = e.target.closest("[data-home-reorder-chore-id]");
    if (homeItem) {
      e.preventDefault();
      document.querySelectorAll(".home-chore-card--drag-over")
        .forEach(el => el.classList.remove("home-chore-card--drag-over"));
      homeItem.classList.add("home-chore-card--drag-over");
    }
  });

  document.addEventListener("dragleave", e => {
    const cell = e.target.closest(".drop-target");
    if (cell) {
      // Only remove when the cursor truly leaves the cell, not when it moves
      // into a child element (dragleave bubbles from children to the cell).
      if (!cell.contains(e.relatedTarget)) {
        cell.classList.remove("drop-target");
      }
    }
    // Sheet chore reorder: remove indicator when leaving the item.
    const item = e.target.closest("[data-reorder-chore-id]");
    if (item && !item.contains(e.relatedTarget)) {
      item.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom");
    }
    // Chores-tab reorder: remove indicator when leaving the row.
    const choresTabItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabItem && !choresTabItem.contains(e.relatedTarget)) {
      choresTabItem.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom");
    }
  });

  document.addEventListener("drop", async e => {
  const contextScope = captureScope(state);
    // ── Chores-tab list reorder ──────────────────────────────────────────────
    const choresTabTargetItem = e.target.closest("[data-chores-tab-reorder-id]");
    if (choresTabTargetItem) {
      e.preventDefault();
      document.querySelectorAll(".chore-row--drag-over-top, .chore-row--drag-over-bottom")
        .forEach(el => el.classList.remove("chore-row--drag-over-top", "chore-row--drag-over-bottom"));
      let payload;
      try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); } catch {
    if (!contextScope.current()) return; return; }
      if (!payload.choresTabReorderId) return;
      const draggedId = payload.choresTabReorderId;
      const targetId  = parseInt(choresTabTargetItem.dataset.choresTabReorderId, 10);
      if (draggedId === targetId) return;
      const rect = choresTabTargetItem.getBoundingClientRect();
      const insertBefore = e.clientY < rect.top + rect.height / 2;
      const sorted = sortChoresByOrder(state.chores, state.choreOrder);
      const ids = sorted.map(c => c.id);
      const fromIdx = ids.indexOf(draggedId);
      const toIdx   = ids.indexOf(targetId);
      if (fromIdx === -1 || toIdx === -1) return;
      ids.splice(fromIdx, 1);
      const insertIdx = ids.indexOf(targetId);
      ids.splice(insertBefore ? insertIdx : insertIdx + 1, 0, draggedId);
      await withCurrentContext(saveChoreOrder(state, ids), contextScope);
      render(app);
      return;
    }

    // ── Home grid chore reorder ──────────────────────────────────────────────
    const homeTargetItem = e.target.closest("[data-home-reorder-chore-id]");
    if (homeTargetItem) {
      e.preventDefault();
      document.querySelectorAll(".home-chore-card--drag-over")
        .forEach(el => el.classList.remove("home-chore-card--drag-over"));
      let payload;
      try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); } catch {
    if (!contextScope.current()) return; return; }
      if (!payload.homeReorderChoreId) return;
      const draggedId = payload.homeReorderChoreId;
      const targetId  = parseInt(homeTargetItem.dataset.homeReorderChoreId, 10);
      if (draggedId === targetId) return;
      const sorted = sortChoresByOrder(state.chores, state.choreOrder);
      const ids = sorted.map(c => c.id);
      const fromIdx = ids.indexOf(draggedId);
      if (fromIdx === -1 || ids.indexOf(targetId) === -1) return;
      ids.splice(fromIdx, 1);
      const insertIdx = ids.indexOf(targetId);
      ids.splice(insertIdx, 0, draggedId);
      await withCurrentContext(saveChoreOrder(state, ids), contextScope);
      render(app);
      return;
    }

    // ── Sheet chore reorder ──────────────────────────────────────────────────
    const targetItem = e.target.closest("[data-reorder-chore-id]");
    if (targetItem) {
      e.preventDefault();
      document.querySelectorAll(".sheet-chore-item--drag-over-top, .sheet-chore-item--drag-over-bottom")
        .forEach(el => el.classList.remove("sheet-chore-item--drag-over-top", "sheet-chore-item--drag-over-bottom"));
      let payload;
      try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); } catch {
    if (!contextScope.current()) return; return; }
      if (!payload.reorderChoreId) return;
      const draggedId = payload.reorderChoreId;
      const targetId  = parseInt(targetItem.dataset.reorderChoreId, 10);
      if (draggedId === targetId) return;
      const rect = targetItem.getBoundingClientRect();
      const insertBefore = e.clientY < rect.top + rect.height / 2;
      // Build new order from the currently-displayed (sorted) chore list.
      const sorted = sortChoresByOrder(state.chores, state.choreOrder);
      const ids = sorted.map(c => c.id);
      const fromIdx = ids.indexOf(draggedId);
      const toIdx   = ids.indexOf(targetId);
      if (fromIdx === -1 || toIdx === -1) return;
      ids.splice(fromIdx, 1);
      const insertIdx = ids.indexOf(targetId);
      ids.splice(insertBefore ? insertIdx : insertIdx + 1, 0, draggedId);
      await withCurrentContext(saveChoreOrder(state, ids), contextScope);
      render(app);
      return;
    }

    // ── Calendar schedule drop ───────────────────────────────────────────────
    const cell = e.target.closest("[data-drop-period], [data-drop-hour]");
    if (!cell) return;
    e.preventDefault();
    cell.classList.remove("drop-target");
    let payload;
    try { payload = JSON.parse(e.dataTransfer.getData("text/plain")); }
    catch {
    if (!contextScope.current()) return; return; }
    const { choreId, scheduleId } = payload;
    const newPeriod = cell.dataset.dropPeriod || "anytime";
    const newHour   = cell.dataset.dropHour != null
      ? `${String(cell.dataset.dropHour).padStart(2, "0")}:00`
      : null;
    try {
      if (scheduleId) {
        // Move an existing schedule to the new time slot (PATCH preserves all
        // other fields including isActive, frequencyType, etc.).
        await withCurrentContext(updateSchedule(scheduleId, {
          timePeriod:   newPeriod,
          specificTime: newHour,
        }), contextScope);
      } else {
        // Unscheduled chore dragged into a slot — create a new "once" schedule
        // for the drop target's date (shown on that specific day only).
        const dropDate = cell.dataset.dropDate || state.calendarDate || null;
        await withCurrentContext(createSchedule({
          choreId,
          timePeriod:    newPeriod,
          specificTime:  newHour,
          frequencyType: "once",
          startDate:     dropDate,
          isActive:      true,
        }), contextScope);
      }
      state.schedules = await withCurrentContext(loadSchedules(), contextScope);
      render(app);
    } catch {
    if (!contextScope.current()) return; showToast("Failed to schedule chore", "error"); }
  });

  // ── Long-press to log a chore (with indicators/note sheet) ──────────────
  function openLogSheet(card) {
    const choreId = parseInt(card.dataset.dragChoreId, 10);
    const logId   = card.dataset.logId ? parseInt(card.dataset.logId, 10) : null;
    const date    = card.dataset.date || state.calendarDate || "";
    state.activeSheet     = "log";
    state.activeSheetData = { choreId, logId, date };
    render(app);
  }

  // Long-press on a chore item inside a bottom sheet (pick-chore / quick-log)
  // opens the log detail sheet so the user can add notes and indicator chips
  // before saving — without scheduling the chore.
  function openLogSheetFromItem(item) {
    const choreId  = parseInt(item.dataset.choreId, 10);
    const date     = item.dataset.date || state.calendarDate || "";
    const slotHour = state.activeSheetData?.hour ?? null;
    state.activeSheet     = "log";
    state.activeSheetData = { choreId, logId: null, date, slotHour };
    render(app);
  }

  function cancelPress() {
    clearTimeout(longPressTimer);
    longPressTimer = null;
    document.querySelectorAll(".chore-card--pressing")
      .forEach(el => el.classList.remove("chore-card--pressing"));
    document.querySelectorAll(".sheet-chore-item--pressing")
      .forEach(el => el.classList.remove("sheet-chore-item--pressing"));
    document.querySelectorAll(".home-chore-card--pressing")
      .forEach(el => el.classList.remove("home-chore-card--pressing"));
  }

  document.addEventListener("mousedown", e => {
    const card = e.target.closest("[data-drag-chore-id]");
    const item = !card && e.target.closest(".sheet-chore-item");
    // Only trigger long-press on home cards that are NOT in jiggle/reorder mode.
    const homeCard = !card && !item && e.target.closest(".home-chore-card:not(.home-chore-card--jiggle)");
    if (!card && !item && !homeCard) return;
    pressStartX = e.clientX;
    pressStartY = e.clientY;
    if (card) card.classList.add("chore-card--pressing");
    if (item) item.classList.add("sheet-chore-item--pressing");
    if (homeCard) homeCard.classList.add("home-chore-card--pressing");
    longPressTimer = setTimeout(() => {
      longPressJustFired = true;
      if (card) { card.classList.remove("chore-card--pressing"); openLogSheet(card); }
      if (item) { item.classList.remove("sheet-chore-item--pressing"); openLogSheetFromItem(item); }
      if (homeCard) {
        homeCard.classList.remove("home-chore-card--pressing");
        state.jiggleMode = true;
        render(app);
      }
    }, 500);
  });
  // Cancel on actual cursor movement (>8px) — but NOT on DOM-triggered mouseleave
  // events that fire when morphInnerHTML removes elements from under the cursor.
  document.addEventListener("mousemove", e => {
    if (!longPressTimer) return;
    const dx = e.clientX - pressStartX;
    const dy = e.clientY - pressStartY;
    if (Math.hypot(dx, dy) > 8) cancelPress();
  });
  document.addEventListener("mouseup", e => {
    cancelPress();
    if (longPressJustFired) {
      // The DOM changed during the long-press (backdrop appeared), so the
      // browser may not synthesize a click event at all (mousedown target ≠
      // mouseup target).  If no click fires, longPressJustFired would stay
      // true forever and block the next intentional click (e.g. the save
      // button in the edit sheet).  Reset it after a short delay so any
      // genuinely synthesized residual click is still swallowed, but the
      // next real user click is never blocked.
      setTimeout(() => { longPressJustFired = false; }, 50);
    }
  });

  // { passive: false } is required to call e.preventDefault() — without it,
  // Chrome (and Android WebView) treats document-level touch listeners as
  // passive by default (since Chrome 56) and silently ignores preventDefault.
  // iOS Safari also adopted this default.
  //
  // For bottom-sheet cards/items we still preventDefault() immediately so that
  // their scroll-contained sheet doesn't fight the outer page scroll.
  //
  // For home-grid cards and sheet-chore items in normal mode we
  // deliberately skip preventDefault() here so the browser can start a
  // vertical scroll freely.  We call preventDefault() in touchend instead
  // to suppress the synthesised click (we fire our own for short taps).
  // In jiggle mode we must consume the whole sequence so the card drag
  // doesn't become a page scroll.
  document.addEventListener("touchstart", e => {
    const card = e.target.closest("[data-drag-chore-id]");
    const item = !card && e.target.closest(".sheet-chore-item");
    const homeCard = !card && !item && e.target.closest(".home-chore-card");
    if (!card && !item && !homeCard) return;
    if (card || (homeCard && state.jiggleMode)) e.preventDefault();
    const t = e.touches[0];
    pressStartX = t.clientX;
    pressStartY = t.clientY;
    // In jiggle mode a touch on a home card starts a drag — handled in touchmove/touchend.
    if (homeCard && state.jiggleMode) {
      // If the user tapped the X (remove) button, skip drag setup so touchend
      // can synthesize the click normally.
      if (e.target.closest("[data-action='home-remove-chore']")) {
        return;
      }
      jiggleDrag.active = true;
      jiggleDrag.choreId = parseInt(homeCard.dataset.homeChoreId, 10);
      jiggleDrag.targetChoreId = null;
      homeCard.closest("[data-home-reorder-chore-id]")?.classList.add("home-chore-card--dragging");
      return;
    }
    if (card) card.classList.add("chore-card--pressing");
    if (item) item.classList.add("sheet-chore-item--pressing");
    if (homeCard) homeCard.classList.add("home-chore-card--pressing");
    longPressTimer = setTimeout(() => {
      longPressJustFired = true;
      if (card) { card.classList.remove("chore-card--pressing"); openLogSheet(card); }
      if (item) { item.classList.remove("sheet-chore-item--pressing"); openLogSheetFromItem(item); }
      if (homeCard) {
        homeCard.classList.remove("home-chore-card--pressing");
        state.jiggleMode = true;
        render(app);
      }
    }, 500);
  }, { passive: false });
  document.addEventListener("touchend", e => {
    // ── Jiggle drag end ──────────────────────────────────────────────────────
    if (jiggleDrag.active) {
      document.querySelectorAll(".home-chore-card--dragging")
        .forEach(el => el.classList.remove("home-chore-card--dragging"));
      document.querySelectorAll(".home-chore-card--drag-over")
        .forEach(el => el.classList.remove("home-chore-card--drag-over"));
      const draggedId = jiggleDrag.choreId;
      const targetId  = jiggleDrag.targetChoreId;
      jiggleDrag.active = false;
      jiggleDrag.choreId = null;
      jiggleDrag.targetChoreId = null;
      if (targetId && targetId !== draggedId) {
        const sorted = sortChoresByOrder(state.chores, state.choreOrder);
        const ids = sorted.map(c => c.id);
        const fromIdx = ids.indexOf(draggedId);
        if (fromIdx !== -1 && ids.indexOf(targetId) !== -1) {
          ids.splice(fromIdx, 1);
          const insertIdx = ids.indexOf(targetId);
          ids.splice(insertIdx, 0, draggedId);
          saveChoreOrder(state, ids).then(owned(() => render(app)));
        }
      }
      return;
    }

    const fired = longPressJustFired;
    cancelPress();

    // Detect the touched element early so we can suppress the browser's
    // synthesised click for home cards and sheet-chore items in all cases
    // (tap, long-press, scroll).  For calendar cards the touchstart already
    // called preventDefault(), so the browser won't synthesise a click and
    // we don't need to here.
    const card = e.target.closest("[data-drag-chore-id]");
    const item = !card && e.target.closest(".sheet-chore-item");
    const homeCard = !card && !item && e.target.closest(".home-chore-card");
    // touchstart skipped preventDefault() for normal-mode home cards and
    // sheet-chore items so the browser could scroll; prevent the synthesised
    // mouse events here instead.
    if (homeCard || item) e.preventDefault();

    if (fired) {
      // Long press was handled; allow a short grace period so any stray
      // synthesised event (on browsers that still fire one) is swallowed.
      setTimeout(() => { longPressJustFired = false; }, 50);
      return;
    }
    // For a short tap (finger barely moved) fire a click manually so the
    // existing data-action handler processes it.
    if (!card && !item && !homeCard) return;
    const t = e.changedTouches[0];
    if (!t) return;
    const dx = t.clientX - pressStartX;
    const dy = t.clientY - pressStartY;
    if (Math.hypot(dx, dy) <= 8) {
      (e.target.closest("[data-action]") || e.target).click();
    }
  });
  document.addEventListener("touchmove", e => {
    if (!longPressTimer) return;
    const t = e.touches[0];
    const dx = t.clientX - pressStartX;
    const dy = t.clientY - pressStartY;
    if (Math.hypot(dx, dy) > 8) cancelPress();
  }, { passive: true });

  // Jiggle-mode touch drag: separate listener so we can call preventDefault()
  // without making the regular touchmove passive listener non-passive.
  document.addEventListener("touchmove", e => {
    if (!jiggleDrag.active) return;
    e.preventDefault();
    const t = e.touches[0];
    const el = document.elementFromPoint(t.clientX, t.clientY);
    const targetCard = el?.closest("[data-home-reorder-chore-id]");
    document.querySelectorAll(".home-chore-card--drag-over")
      .forEach(c => c.classList.remove("home-chore-card--drag-over"));
    if (targetCard) {
      const tid = parseInt(targetCard.dataset.homeReorderChoreId, 10);
      jiggleDrag.targetChoreId = tid;
      if (tid !== jiggleDrag.choreId) targetCard.classList.add("home-chore-card--drag-over");
    } else {
      jiggleDrag.targetChoreId = null;
    }
  }, { passive: false });

  // ── Notification polling ─────────────────────────────────────────────────
  // Poll every 30 s for new notifications.  Pause while the tab is hidden so
  // we don't hammer the API in background tabs.  Also refreshes household data
  // when a household_joined notification is detected so new members appear
  // without needing a page reload.
  async function resumeSession() {
    if (document.hidden) return;
    if (state.logoutPending) { void doLogout(); return; }
    if (!await confirmBrowserSession()) return;
    const scope = captureScope(state);
    // Confirm both the cookie's owner and current chore visibility before
    // replay or push registration. Old UI data is never authority to resume.
    if (!await loadChoreData() || !scope.current()) return;
    const refreshNotifications = document.querySelector("#notif-panel-container")?.hidden !== false;
    await Promise.all([refreshNotifications ? loadNotifData() : undefined,loadHouseholdData(),reloadViewData(),flushOfflineQueue(null,{confirmed:true})]);
    if (scope.current()) { maybeSubscribePush().catch(() => {}); render(app); }
  }
  document.addEventListener("visibilitychange", () => { if (!document.hidden) void resumeSession(); });
  if (state.user) startNotifPoll();

  // Tick the home grid's "X ago" labels once a minute while the home tab is
  // visible, so relative times stay fresh without a navigation/re-render.
  setInterval(() => {
    if (document.hidden || !state.user) return;
    const route = state.currentRoute || window.location.pathname || "/";
    if (route === "/" || route === "/today") refreshHomeCardTimes(state);
  }, 60000);

  // ── Pull-to-refresh ────────────────────────────────────────────────────────
  // In iOS standalone mode there is no browser refresh chrome. Add a light
  // overscroll gesture on the scroll container: pulling down from the top
  // past a threshold refetches the active tab's data. Honors
  // prefers-reduced-motion (skips the transition, still refreshes).
  setupPullToRefresh();

  window.addEventListener("storage", event => {
    const origin = contextSnapshot();
    if (event.key !== `nabu_timer:${origin?.userId}:${origin?.householdId}`) return;
    const saved = loadTimer(origin);
    if (saved?.id === state.activeTimer?.id && state.activeTimer?.saving) saved.saving = true;
    state.activeTimer = saved;
    renderTimerChip();
  });
  window.addEventListener("nabu-journal-change", () => {
    hydratePendingLogs().then(owned(() => { if (!state.activeSheet) render(app); })).catch(() => {});
  });
  window.addEventListener("online", () => {
    void resumeSession();
  });
  await hydratePendingLogs();
  if (state.user && navigator.onLine !== false) void flushOfflineQueue();

  render(app);
}

function syncLogSaveControls(root) {
  const button = root.querySelector('[data-action="save-log"]');
  if (!button) return;
  const draft = state.activeSheetData || {}, sheet = button.closest(".bottom-sheet");
  button.dataset.readyLabel ||= button.textContent;
  button.disabled = !!draft.saving;
  button.textContent = draft.saving ? "Saving…" : draft.saveError && draft.submission?.body
    ? (draft.savedDurably ? "Retry saved entry" : "Retry entry") : button.dataset.readyLabel;
  const frozen = !!draft.submission?.body && !!(draft.saving || draft.saveError);
  sheet?.querySelectorAll('input, textarea, select, button:not([data-action="save-log"]):not([data-action="close-sheet"]):not([data-action="discard-log-draft"])').forEach(el => {
    if (frozen && !el.disabled) { el.dataset.frozenDisabled = "true"; el.disabled = true; }
    else if (!frozen && el.dataset.frozenDisabled) { el.disabled = false; delete el.dataset.frozenDisabled; }
  });
  let error = sheet?.querySelector(".saved-draft-error");
  if (draft.saveError && sheet) {
    if (!error) { error = document.createElement("div"); error.className = "saved-draft-error form-error"; error.setAttribute("role", "status"); button.before(error); }
    error.innerHTML = `<p>${escapeHTML(draft.saveError)}</p>${draft.submission?.body ? `<p>Retry sends this entry unchanged.${draft.savedDurably ? "" : " Keep this page open; device storage is unavailable."}</p><button type="button" class="btn btn-ghost btn-sm" data-action="discard-log-draft">${draft.savedDurably ? "Discard saved copy and edit" : "Discard retry and edit"}</button>` : ''}`;
  } else error?.remove();
}

function renderPendingWork() {
  const rows = state.pendingLogs || [];
  return `<details class="pending-work"><summary>${rows.length} log${rows.length === 1 ? "" : "s"} saved on this device · awaiting confirmation</summary>${rows.map(log => {
    const chore = (state.chores || []).find(c => c.id === log.choreId);
    return `<div class="pending-work-row" data-pending-key="${escapeHTML(log.idempotencyKey)}"><strong>${escapeHTML(chore?.name || "Saved log")}</strong>
      ${log.note ? `<p>${escapeHTML(log.note)}</p>` : ""}${log.durationSeconds != null ? `<p>${formatElapsed(log.durationSeconds)}</p>` : ""}
      <p>${escapeHTML(log._error || "Waiting to sync.")}</p>
      <button type="button" class="btn btn-sm" data-action="retry-pending-log" data-key="${escapeHTML(log.idempotencyKey)}">Retry</button>
      <button type="button" class="btn btn-ghost btn-sm" data-action="discard-pending-log" data-key="${escapeHTML(log.idempotencyKey)}">Discard saved copy</button></div>`;
  }).join("")}</details>`;
}
async function hydratePendingLogs() {
  const contextScope = captureScope(state);
  if (!state.user?.householdId) return;
  const scope = captureScope(state, "journal");
  try {
    const entries = await withCurrentContext(queuedLogs(scope.origin), contextScope);
    if (!scope.current()) return;
    state.pendingLogs = entries.map(entry => ({ ...entry.body, id:`pending-${entry.idempotencyKey}`, userId:entry.body.userId || entry.actorId,
      _pending:true, _error:entry.error, _status:entry.status }));
  } catch { /* preserve last good rows if storage is temporarily unavailable */ }
}
async function flushOfflineQueue(retryKey = null, {confirmed=false} = {}) {
  if (!state.user?.householdId || state.logoutPending || state.transitioning || state.sessionUnconfirmed) return;
  if (!confirmed && !await confirmBrowserSession()) return;
  const contextScope = captureScope(state);
  if (!state.user?.householdId || state.logoutPending || state.transitioning) return;
  const scope = captureScope(state);
  try {
    const { syncedKeys } = await withCurrentContext(replayQueue(apiFetch, {origin:scope.origin,retryKey}), contextScope);
    if (!scope.current()) return;
    await withCurrentContext(hydratePendingLogs(), contextScope);
    if (!scope.current()) return;
    if (syncedKeys.length) {
      await withCurrentContext(Promise.all([loadLatestLogsData(), reloadViewData()]), contextScope);
      if (!scope.current()) return;
      showToast(`Synced ${syncedKeys.length} log${syncedKeys.length === 1 ? "" : "s"}`, "success");
    }
    render(document.querySelector("#app"));
  } catch { /* retained in the journal */ }
}

async function loadHouseholdData() {
  const contextScope = captureScope(state, "household");
  try {
    const [data, listData] = await withCurrentContext(Promise.all([
      loadHousehold(),
      listHouseholds(),
    ]), contextScope);
    if (data.household) {
      state.household = data.household;
      state.members = data.members;
      state.historicalMembers = data.historicalMembers || [];
      state.invites = data.invites;
    }
    if (Array.isArray(listData?.households)) {
      state.userHouseholds = listData.households;
      if (data.household) {
        state.activeHouseholdId = data.household.id;
      }
    }
  } catch {}
}

async function loadChoreData() {
  const contextScope = captureScope(state, "chores");
  try {
    const data = await withCurrentContext(loadChores(), contextScope);
    if (data.chores) {
      state.chores = data.chores;
      return true;
    }
  } catch {}
  return false;
}

async function loadTodayData() {
  const contextScope = captureScope(state, "today", () => state.calendarDate || state.todayDate || todayISO(0));
  try {
    const date = state.calendarDate || state.todayDate || todayISO(0);
    const [todayResult, scheduleList] = await withCurrentContext(Promise.all([
      loadToday(date),
      loadSchedules(),
    ]), contextScope);
    state.todayLogs = todayResult.logs || [];
    state.dailySummary = todayResult.summary;
    state.schedules = scheduleList;
  } catch {}
}

async function loadWeekData() {
  const contextScope = captureScope(state, "week", () => state.calendarDate || todayISO(0));
  try {
    const date = state.calendarDate || todayISO(0);
    const d = new Date(date + "T00:00:00");
    const day = d.getDay();
    d.setDate(d.getDate() + (day === 0 ? -6 : 1 - day));
    const weekStart = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    const [weekResult, scheduleList] = await withCurrentContext(Promise.all([
      loadWeek(weekStart),
      loadSchedules(),
    ]), contextScope);
    state.weekLogs = weekResult.logs || [];
    state.schedules = scheduleList;
  } catch {}
}

async function loadActivityPage(options = {}) {
  const contextScope = captureScope(state);
  const scope = captureScope(state);
  const pending = loadActivity(state,options);
  if (!state.activeSheet) render(document.querySelector("#app"));
  await withCurrentContext(Promise.all([pending,loadDayNotesData()]), contextScope);
  if (scope.current()) render(document.querySelector("#app"));
}
async function reloadViewData() {
  const contextScope = captureScope(state);
  if (!state.user || !state.household) return;
  const scope = captureScope(state);
  const route = state.currentRoute || window.location.pathname || "/";
  if (route === "/activity") {
    await withCurrentContext(Promise.all([loadActivity(state,{preservePages:true}),loadDayNotesData()]), contextScope);
  } else if (route === "/stats") {
    await withCurrentContext(loadAllStatsData(), contextScope);
  } else if (route === "/schedule") {
    await withCurrentContext(loadTodayData(), contextScope);
  } else {
    await withCurrentContext(Promise.all([loadTodayData(),loadLatestLogsData()]), contextScope);
  }
  if (scope.current()) await withCurrentContext(hydratePendingLogs(), contextScope);
}

async function runHouseholdTransition(run, {seed=false,route=state.currentRoute || window.location.pathname || "/"} = {}) {
  if (state.transitioning) return;
  const ticket = ++identityUIRevision, app = document.querySelector("#app");
  closeAllPanels(); adoptUser(state.user,route); state.transitioning=true; render(app);
  try {
    const result = await run();
    if (ticket !== identityUIRevision || !resultIsCurrent(result)) return;
    adoptUser(result.user,route);
    const scope = captureScope(state);
    if (seed && result.ok) { await seedDefaultChores(); if (!scope.current()) return; }
    await reloadAfterAuth();
    if (!scope.current()) return;
    if (!result.ok) showToast(result.error || "Could not change household. Please retry.","error");
  } catch (err) {
    if (ticket !== identityUIRevision) return;
    await recoverIdentity(err,route);
    if (ticket === identityUIRevision) showToast(err.message || "Could not confirm the household change. Please retry.","error");
  }
  if (ticket === identityUIRevision) { state.transitioning=false; render(app); }
}

async function doCreateHousehold(form) {
  const name = form.querySelector("#hh-name").value;
  const initials = (form.querySelector("#hh-initials")?.value || "").trim();
  return runHouseholdTransition(() => createHousehold(name, initials || generateInitials(name)), {seed:true,route:"/"});
}

async function seedDefaultChores() {
  const contextScope = captureScope(state);
  try {
    await withCurrentContext(apiFetch("/api/chores/seed-defaults", { method: "POST" }), contextScope);
  } catch {}
}

async function doJoinHousehold(form) {
  const code = form.querySelector("#invite-code").value;
  return runHouseholdTransition(() => joinHousehold(code), {route:"/"});
}

async function doUpdateHousehold(form) {
  const contextScope = captureScope(state);
  const name = form.querySelector("#edit-hh-name")?.value?.trim() || "";
  const initials = (form.querySelector("#edit-hh-initials")?.value || "").trim();
  if (!name) return;
  const data = await withCurrentContext(updateHousehold(name, initials || generateInitials(name)), contextScope);
  if (!data || data.error) {
    showToast(data?.error || "Failed to update household", "error");
    return;
  }
  await withCurrentContext(loadHouseholdData(), contextScope);
  updateTopBar();
  const app = document.querySelector("#app");
  if (app) render(app);
  showToast("Household updated", "info");
}

async function doCreateChoreFromSheet(form) {
  const contextScope = captureScope(state);
  const name      = form.querySelector('[name="choreName"]').value.trim();
  if (!name) return;

  try {
    const { data: choreData } = await withCurrentContext(apiFetch("/api/chores", {
      method: "POST",
      body: JSON.stringify({ name }),
    }), contextScope);
    const newChore = choreData?.chore;
    if (!newChore) { showToast("Failed to create chore", "error"); return; }

    const timeInput    = document.querySelector("#sheet-time");
    const specificTime = timeInput?.value || null;
    const slotDate     = form.querySelector('[name="date"]')?.value || state.activeSheetData?.date || null;
    const freqPayload  = readSheetFreq("sheet", slotDate);
    await withCurrentContext(createSchedule({
      choreId:       newChore.id,
      timePeriod:    "anytime",
      specificTime,
      isActive:      true,
      ...freqPayload,
    }), contextScope);

    await withCurrentContext(loadChoreData(), contextScope);
    // Append new chore to the user's custom order so it appears at the bottom
    // of the sheet list rather than being sorted to an arbitrary position.
    if (newChore.id) {
      const newOrder = [...(state.choreOrder || []), newChore.id];
      await withCurrentContext(saveChoreOrder(state, newOrder), contextScope);
    }
    state.schedules = await withCurrentContext(loadSchedules(), contextScope);
    state.activeSheet     = null;
    state.activeSheetData = {};
    const app = document.querySelector("#app");
    if (app) render(app);
  } catch {
    if (!contextScope.current()) return;
    showToast("Failed to create chore", "error");
  }
}

function bootstrap() {
  init().catch(() => {});
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", bootstrap);
} else {
  bootstrap();
}
