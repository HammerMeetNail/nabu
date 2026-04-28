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
}
