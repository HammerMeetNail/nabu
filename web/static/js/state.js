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
    activityView: "history",
    calendarView: "day",
    calendarDate: null,    // null = use today
    weekLogs: [],
    activeSheet: null,
    activeSheetData: {},
    choreOrder: [],            // per-user preferred chore order (array of chore IDs)
    hiddenHomeChoreIDs: [],    // chore IDs hidden from the Home tab grid
    jiggleMode: false,         // home grid reorder mode
    homeView: "log",           // "log" | "manage"
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
  state.activityView = "history";
  state.calendarView = "day";
  state.calendarDate = null;
  state.weekLogs = [];
  state.activeSheet = null;
  state.activeSheetData = {};
  state.choreOrder = [];
  state.hiddenHomeChoreIDs = [];
  state.jiggleMode = false;
  state.homeView = "log";
  state.latestLogs = {};
}
