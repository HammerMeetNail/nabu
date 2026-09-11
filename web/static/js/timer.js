// Duration timer (Phase 5.2). A single active timer is persisted in
// localStorage so it survives reloads. Pure helpers here; the DOM chip and
// action wiring live in app.js.

import { contextSnapshot, assertContext } from "./browser-context.js";

import { newKey, withBrowserLock } from "./device-store.js";
import { localDateStr } from "./utils.js";

function key(origin) { return origin?.userId && origin?.householdId ? `nabu_timer:${origin.userId}:${origin.householdId}` : null; }

export function loadTimer(origin = contextSnapshot()) {
  try {
    const storageKey = key(origin);
    if (!storageKey) return null;
    const t = JSON.parse(localStorage.getItem(storageKey) || "null");
    if (t && t.actorId === origin.userId && t.householdId === origin.householdId &&
        typeof t.choreId === "number" && typeof t.startedAt === "number") return t;
  } catch { /* leave an unreadable record untouched for recovery */ }
  return null;
}

export function saveTimer(t, origin = contextSnapshot()) {
  const storageKey = key(origin);
  if (!storageKey) throw new Error("Sign in to this household before starting a timer");
  if (t) localStorage.setItem(storageKey, JSON.stringify({ ...t, actorId:origin.userId, householdId:origin.householdId }));
  else localStorage.removeItem(storageKey);
  return true;
}

// elapsedSeconds returns whole seconds elapsed since the timer started.
export function elapsedSeconds(t, now = Date.now()) {
  if (!t || typeof t.startedAt !== "number") return 0;
  return Math.max(0, Math.floor(((t.stoppedAt || now) - t.startedAt) / 1000));
}

// formatElapsed renders seconds as m:ss (or h:mm:ss past an hour).
export function formatElapsed(sec) {
  const s = Math.max(0, Math.floor(sec || 0));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  const pad = (n) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(ss)}` : `${m}:${pad(ss)}`;
}

function timerLock(origin) {
  const name = key(origin);
  if (!name) throw new Error("Sign in to this household to use its timer.");
  return name;
}
export async function startTimer(timer, origin = contextSnapshot()) {
  return withBrowserLock(timerLock(origin), () => {
    assertContext(origin);
    if (loadTimer(origin)) throw new Error("Finish the active timer first.");
    const next = { ...timer, id:newKey(), submission:{idempotencyKey:newKey()} };
    saveTimer(next, origin);
    return next;
  });
}
export async function stopTimer(timerID, origin = contextSnapshot()) {
  return withBrowserLock(timerLock(origin), () => {
    assertContext(origin);
    const timer = loadTimer(origin);
    if (!timer) return null;
    if (timer.id !== timerID) throw new Error("The active timer changed in another tab.");
    timer.stoppedAt ||= Date.now();
    timer.submission ||= { idempotencyKey:newKey() };
    if (!timer.submission.body) {
      const when = new Date(timer.stoppedAt);
      timer.submission.body = { choreId:timer.choreId, note:"", indicators:[], date:localDateStr(when), hour:when.getHours(),
        completedAt:when.toISOString(), userId:origin.userId, durationSeconds:elapsedSeconds(timer), idempotencyKey:timer.submission.idempotencyKey };
    }
    saveTimer(timer, origin);
    return timer;
  });
}
export async function clearFinishedTimer(timerID, origin) {
  return withBrowserLock(timerLock(origin), () => {
    if (loadTimer(origin)?.id === timerID) saveTimer(null, origin);
  });
}
