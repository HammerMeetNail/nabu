export function createAppState() {
  return {
    contextGeneration: 0,
    user: null,
    currentRoute: null,
    networkOnline: navigator.onLine,
    views: {},
    todayLogs: [],
    chores: [],
    household: null,
    historicalMembers: [],
    userHouseholds: [],
    activeHouseholdId: null,
    notifications: [],
    notificationCursor: null,
    notificationLoading: false,
    notificationLoadingMore: false,
    notificationMutating: false,
    notificationError: null,
    notificationErrorAction: null,
    unreadNotifications: 0,
    schedules: [],
    activityView: "history",
    calendarView: "day",
    calendarDate: null,    // null = use today
    weekLogs: [],
    activeSheet: null,
    activeSheetData: {},
    deleteAccountOpen: false,
    deleteAccountBusy: false,
    deleteAccountError: null,
    choreOrder: [],            // per-user preferred chore order (array of chore IDs)
    hiddenHomeChoreIDs: [],    // chore IDs hidden from the Home tab grid
    volumeUnit: "ml",          // per-user volume display/input unit ("ml" | "oz")
    hideNotificationBadge: false, // per-user pref: hide the unread count on the bell
    jiggleMode: false,         // home grid reorder mode
    homeView: "log",           // "log" | "manage"
    latestLogs: {},            // map of choreId -> ChoreLog (most recent per chore)
    notificationPrefs: null,
    availableNotificationTypes: [],
    choreReminderPrefs: [],     // array of {userId, choreId, enabled, leadMinutes}
    historyChoreFilter: null,  // null = show all, []string = filtered chore IDs
    historyFilterOpen: false,  // filter dropdown starts closed
    historySearch: "",         // text search across note/title (empty = off)
    historyLogs: [],
    historyHasMore: false,
    historyBefore: null,
    pendingLogs: [],
    activeTimer: null,
    dayNotes: {},
    stats: {
      sectionOrder: [],       // ordered array of section keys (user pref)
      sectionHidden: [],      // array of hidden section keys (user pref)
      customizeOpen: false,   // whether the "Customize Stats" panel is open
    },
  };
}

export function resetAuthedState(state) {
  const publicState = { googleOAuthEnabled:state.googleOAuthEnabled, appleSignInEnabled:state.appleSignInEnabled,
    contextGeneration:(state.contextGeneration || 0)+1 };
  // Delete dynamic caches as well as declared defaults. Object.assign alone
  // leaves Activity pages, timer state and detailed Stats from the old identity.
  for (const key of Object.keys(state)) delete state[key];
  Object.assign(state, createAppState(), publicState);
}
