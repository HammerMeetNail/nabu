export function createAppState() {
  return {
    user: null,
    currentRoute: null,
    networkOnline: navigator.onLine,
    views: {},
    todayLogs: [],
    chores: [],
    household: null,
    notifications: [],
    unreadNotifications: 0,
    schedules: [],
    calendarView: "day",
    calendarDate: null,    // null = use today
    weekLogs: [],
    activeSheet: null,
    activeSheetData: {},
    choreOrder: [],   // per-user preferred chore order (array of chore IDs)
    jiggleMode: false,         // home grid reorder mode
    latestLogs: {},            // map of choreId -> ChoreLog (most recent per chore)
  };
}

export function resetAuthedState(state) {
  state.user = null;
  state.household = null;
  state.chores = [];
  state.todayLogs = [];
  state.notifications = [];
  state.unreadNotifications = 0;
  state.members = [];
  state.invites = [];
  state.schedules = [];
  state.calendarView = "day";
  state.calendarDate = null;
  state.weekLogs = [];
  state.activeSheet = null;
  state.activeSheetData = {};
  state.choreOrder = [];
  state.jiggleMode = false;
  state.latestLogs = {};
}
