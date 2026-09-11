import { captureScope } from './request-scope.js';
import { localDateStr, shiftDateStr } from './utils.js';
import { loadOverview, loadHeatmap, loadBusyHours, loadChoreStats, loadCategoryBreakdown,
  loadTopChores, loadLeaderboard, loadChoreTimeSeries, loadChoreSummary, loadFeedingGaps,
  choreHasAnalytics, choreAnalyticsGrain, widgetGrain } from './stats.js';

const visible = (state, section) => !(state.stats.sectionHidden || []).includes(section);
const selectedUser = state => state.stats.topChoresUserId || state.user?.id || 0;
const babyChore = (state, type) => state.chores.find(c => c.name === (type === 'feed' ? 'Feed Baby' : 'Change Baby'));

// Every producer of a section uses this owner, including refresh and controls.
// Query changes clear obsolete results; transient failures retain same-query data.
async function read(state, key, section, query, fetcher, publish, clear) {
  const stats = state.stats;
  if (!visible(state, section)) {
    if (stats.loading) stats.loading[key] = false;
    if (stats.errors) delete stats.errors[key];
    return;
  }
  const currentQuery = () => [query(), visible(state, section)];
  const value = structuredClone(query()), fingerprint = JSON.stringify(value);
  const scope = captureScope(state, `stats:${key}`, currentQuery);
  stats.queryKeys ||= {}; stats.loading ||= {}; stats.errors ||= {};
  if (stats.queryKeys[key] !== fingerprint) clear?.();
  stats.queryKeys[key] = fingerprint; stats.loading[key] = true; delete stats.errors[key];
  try {
    const data = await fetcher(value);
    if (scope.current()) publish(data);
  } catch {
    if (scope.current()) stats.errors[key] = 'Could not load this chart. Retry to refresh.';
  } finally {
    if (scope.owns()) stats.loading[key] = false;
  }
}

export function loadStatsResource(state, section, id = null) {
  const s = state.stats;
  switch (section) {
    case 'overview':
      return read(state, section, '', () => null, loadOverview, data => {s.overview = data.overview;});
    case 'activity':
      return read(state, section, section, () => null, loadHeatmap, data => {s.heatmap = data.heatmap;});
    case 'busy-hours':
      return read(state, section, section, () => {
        const f = s.busyHoursFilter || {};
        return {choreId:f.choreId || null,userId:f.userId || null,start:f.start || null,end:f.end || null};
      }, loadBusyHours, data => {
        s.busyHours = data.busyHours; s.busyHoursStart = data.start; s.busyHoursEnd = data.end;
      }, () => {s.busyHours = []; s.busyHoursStart = s.busyHoursEnd = '';});
    case 'chores':
      return read(state, section, section, () => s.choreStatsPeriod || 'month', period => loadChoreStats({period}), data => {
        s.choreStats = data.choreStats; s.choreStatsStart = data.start; s.choreStatsEnd = data.end;
      }, () => {s.choreStats = []; s.choreStatsStart = s.choreStatsEnd = '';});
    case 'categories':
      return read(state, section, section, () => s.categoriesPeriod || 'week', loadCategoryBreakdown,
        data => {s.categoriesBreakdown = data.breakdown;}, () => {s.categoriesBreakdown = [];});
    case 'leaderboard': {
      s.leaderboardByPeriod ||= {}; s.leaderboardRangeByPeriod ||= {};
      const period = s.leaderboardPeriod || 'week';
      return read(state, `${section}:${period}`, section, () => s.leaderboardPeriod || 'week', loadLeaderboard, data => {
        s.leaderboardByPeriod[period] = data.leaderboard;
        s.leaderboardRangeByPeriod[period] = {start:data.start || '',end:data.end || ''};
      });
    }
    case 'top-chores': {
      s.topChoresByUserAndPeriod ||= {};
      const key = `${selectedUser(state)}-${s.topChoresPeriod || 'month'}`;
      return read(state, `${section}:${key}`, section, () => [selectedUser(state),s.topChoresPeriod || 'month'],
        ([user,period]) => loadTopChores(user,period), data => {s.topChoresByUserAndPeriod[key] = data.topChores;});
    }
    case 'chore':
      s.choreTimeSeries ||= {};
      return read(state, `chore:${id}`, `chore:${id}`, () => choreAnalyticsGrain(s.choreAnalyticsPeriod?.[id] || 'day'),
        grain => loadChoreTimeSeries(id,grain), data => {s.choreTimeSeries[id] = data.timeSeries;},
        () => {delete s.choreTimeSeries[id];});
    case 'baby': {
      const chore = babyChore(state,id);
      if (!chore) return Promise.resolve();
      s.babyTimeSeries ||= {};
      const field = id === 'feed' ? 'feedBaby' : 'changeBaby';
      return read(state, `baby:${id}`, 'baby', () => [babyChore(state,id)?.id,s[`${field}Period`] || 'daily'],
        ([cid,period]) => loadChoreTimeSeries(cid,period), data => {s.babyTimeSeries[field] = data.timeSeries;},
        () => {delete s.babyTimeSeries[field];});
    }
    case 'gaps': {
      if (!babyChore(state,'feed')) return Promise.resolve();
      s.feedingGapsEnd ||= localDateStr(new Date());
      s.feedingGapsStart ||= shiftDateStr(s.feedingGapsEnd,-6);
      return read(state, 'gaps', 'baby', () => [s.feedingGapsStart,shiftDateStr(s.feedingGapsEnd,1)],
        ([start,end]) => loadFeedingGaps(start,end), data => {s.feedingGaps = data.feedingGaps;},
        () => {s.feedingGaps = null;});
    }
    case 'widget': {
      const getWidget = () => s.widgets?.find(w => w.id === id) || null;
      s.widgetData ||= {};
      return read(state, `widget:${id}`, `widget:${id}`, getWidget, async widget => {
        if (!widget || widget.type === 'last-done') return [];
        // The editor/server limit widget selections. Deduplicate repeated IDs
        // before fanout; apiFetch also shares identical in-flight GETs.
        return Promise.all([...new Set(widget.choreIds || [])].map(async cid => {
          const chore = state.chores.find(c => c.id === cid);
          if (widget.type === 'timeseries') return {chore,ts:(await loadChoreTimeSeries(cid,widgetGrain(widget))).timeSeries};
          return {chore,summary:(await loadChoreSummary(cid,widget.period || 'week')).summary};
        }));
      }, data => {s.widgetData[id] = data;}, () => {delete s.widgetData[id];});
    }
  }
  return Promise.resolve();
}

export function loadStatsPage(state, onProgress = () => {}) {
  const scope = captureScope(state, 'stats:page');
  const progress = scope.guard(onProgress);
  state.stats.topChoresUserId ||= state.user?.id || 0;
  for (const key of Object.keys(state.stats.loading || {})) {
    const section = key.startsWith('leaderboard:') ? 'leaderboard' : key.startsWith('top-chores:') ? 'top-chores'
      : key.startsWith('baby:') || key === 'gaps' ? 'baby' : key;
    const deleted = key.startsWith('widget:') && !(state.stats.widgets || []).some(w => key === `widget:${w.id}`);
    if (deleted || !visible(state,section)) {state.stats.loading[key] = false; delete state.stats.errors?.[key];}
  }
  const sections = ['overview','activity','busy-hours','chores','categories','top-chores','leaderboard'];
  const load = (section,id = null) => loadStatsResource(state,section,id).then(progress);
  const tasks = sections.map(section => load(section));
  tasks.push(load('baby','feed'),load('baby','change'),load('gaps'));
  tasks.push(loadChoreAnalytics(state,progress),loadStatsWidgets(state,progress));
  return Promise.all(tasks);
}
export function loadChoreAnalytics(state, onProgress = () => {}) {
  return Promise.all(state.chores.filter(choreHasAnalytics).filter(c => visible(state,`chore:${c.id}`)).slice(0,15)
    .map(c => loadStatsResource(state,'chore',c.id).then(onProgress)));
}
export function loadStatsWidgets(state, onProgress = () => {}) {
  for (const id of Object.keys(state.stats.widgetData || {})) {
    if (!(state.stats.widgets || []).some(w => w.id === id)) {
      delete state.stats.widgetData[id];
      if (state.stats.loading) delete state.stats.loading[`widget:${id}`];
      if (state.stats.errors) delete state.stats.errors[`widget:${id}`];
    }
  }
  // The server caps saved widgets at 20. Every visible saved card needs data;
  // the separate per-chore analytics cap must not hide the final five widgets.
  return Promise.all((state.stats.widgets || []).filter(w => visible(state,`widget:${w.id}`))
    .map(w => loadStatsResource(state,'widget',w.id).then(onProgress)));
}
