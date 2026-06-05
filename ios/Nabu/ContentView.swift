import SwiftUI

struct ContentView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @StateObject private var auth = AuthStore(api: APIClient(baseURL: URL(string: "http://localhost:8080")!))
    @StateObject private var dataLoader = DataLoader()
    @State private var hasCheckedSession = false
    @State private var hasLoadedData = false

    var body: some View {
        Group {
            if !hasCheckedSession {
                ProgressView("Loading...")
            } else if state.user == nil {
                LoginView(auth: auth, apiBaseURL: environment.baseURL)
                    .onAppear { auth.configure(api: environment.apiClient) }
            } else if state.household == nil {
                OnboardingView(auth: auth)
                    .onAppear { auth.configure(api: environment.apiClient) }
                    .onChange(of: state.household) { _, newHousehold in
                        if newHousehold != nil {
                            Task { await loadAppData() }
                        }
                    }
            } else if !hasLoadedData {
                ProgressView("Loading your data...")
                    .task { await loadAppData() }
            } else {
                MainTabView(dataLoader: dataLoader)
            }
        }
        .pageBackground()
        .task {
            if !hasCheckedSession {
                dataLoader.configure(api: environment.apiClient, state: state)
                auth.configure(api: environment.apiClient)
                if TestHooks.seedHomeForUITest {
                    // State was already injected by AppEnvironment.configure().
                    // Skip the network session check so tests never touch the real API.
                    hasCheckedSession = true
                    hasLoadedData = true
                } else {
                    state.user = await auth.loadSession()
                    hasCheckedSession = true
                    if state.user?.householdId != nil {
                        await loadAppData()
                    }
                }
            }
        }
        .onChange(of: state.user) { _, newUser in
            if newUser == nil {
                state.reset()
                hasLoadedData = false
                Task { await auth.logout() }
            } else if newUser?.householdId != nil {
                Task { await loadAppData() }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.willEnterForegroundNotification)) { _ in
            Task { await dataLoader.foregroundRefresh() }
        }
    }

    func loadAppData() async {
        guard state.user != nil else { return }
        await dataLoader.reloadAfterAuth()
        hasLoadedData = state.household != nil
    }
}

struct MainTabView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @ObservedObject var dataLoader: DataLoader

    var body: some View {
        TabView(selection: $state.currentTab) {
            StatsView()
                .tabItem {
                    Label(MainTab.stats.title, systemImage: MainTab.stats.systemImage)
                }
                .tag(MainTab.stats)

            ActivityView(activityStore: ActivityStore(api: environment.apiClient),
                         logStore: LogStore(api: environment.apiClient))
                .tabItem {
                    Label(MainTab.activity.title, systemImage: MainTab.activity.systemImage)
                }
                .tag(MainTab.activity)

            HomeView(logStore: LogStore(api: environment.apiClient))
                .tabItem {
                    Label(MainTab.home.title, systemImage: MainTab.home.systemImage)
                }
                .tag(MainTab.home)

            ScheduleView(scheduleStore: ScheduleStore(api: environment.apiClient))
                .tabItem {
                    Label(MainTab.schedule.title, systemImage: MainTab.schedule.systemImage)
                }
                .tag(MainTab.schedule)

            HouseholdView()
                .tabItem {
                    Label(MainTab.settings.title, systemImage: MainTab.settings.systemImage)
                }
                .badge(state.unreadNotifications)
                .tag(MainTab.settings)
        }
        .tint(DesignColors.primary)
    }
}
