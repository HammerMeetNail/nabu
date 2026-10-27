import SwiftUI

struct ContentView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @StateObject private var auth = AuthStore(api: APIClient(baseURL: URL(string: "http://localhost:8080")!))
    @StateObject private var dataLoader = DataLoader()
    @StateObject private var accountDeletion = AccountDeletionModel()
    @State private var hasCheckedSession = false
    @State private var hasLoadedData = false
    /// Identity revision of the `loadAppData()` run currently in flight, so the
    /// several triggers that fire for one sign-in cannot overlap.
    @State private var loadRevision: UUID?
    @State private var handledLinks: Set<URL> = []

    var body: some View {
        Group {
            if !hasCheckedSession {
                SkeletonScreen()
            } else if state.sessionPhase == .logoutPending {
                VStack(spacing: 16) {
                    Text("Sign-out is unfinished").font(.headline)
                    Text("Your data is hidden. Retry to finish signing out on the server.")
                    if !state.logoutIsDurable {
                        Text("This device could not save the sign-out status. Keep the app open and retry.")
                    }
                    Button("Retry sign-out") { Task { _ = await auth.logout() } }
                        .accessibilityIdentifier("retry-logout")
                }.padding()
            } else if state.sessionPhase == .checking || state.sessionPhase == .changing {
                VStack(spacing: 16) {
                    Text("Confirming your session")
                    if state.sessionPhase == .changing { ProgressView() }
                    else {
                        Text("Reconnect and retry to continue.")
                        Button("Retry") { Task { await checkSession() } }
                    }
                }.padding()
            } else if state.user == nil {
                LoginView(auth: auth, apiBaseURL: environment.baseURL)
                    .onAppear { auth.configure(api: environment.apiClient) }
            } else if state.household == nil, let message = state.accessSuspendedMessage {
                AccessSuspendedView(message: message) {
                    Task { _ = await auth.logout() }
                }
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
                SkeletonScreen()
                    .task { await loadAppData() }
            } else {
                MainTabView(dataLoader: dataLoader)
            }
        }
        .environmentObject(accountDeletion)
        .pageBackground()
        .sheet(isPresented: $accountDeletion.isPresented, onDismiss: {
            let returnToSettings = accountDeletion.matches(environment.apiClient)
            accountDeletion.cancel()
            if returnToSettings { state.currentTab = .settings }
        }) {
            DeleteAccountSheet().environmentObject(accountDeletion)
        }
        .onChange(of: state.revision) { _, _ in
            accountDeletion.reconcile(api: environment.apiClient)
        }
        .task {
            if !hasCheckedSession {
                let args = ProcessInfo.processInfo.arguments
                // The scene's onAppear may not have run yet.
                environment.configure(with: state)
                dataLoader.configure(api: environment.apiClient, state: state)
                auth.configure(api: environment.apiClient)
                if TestHooks.seedHomeForUITest {
                    hasCheckedSession = true
                    hasLoadedData = true
                } else if let (email, password) = parseTestCreds(args) {
                    // Pre-flight GET to obtain a CSRF cookie before the register POST.
                    let _: StatusResponse? = try? await auth.api.get("/api/me")
                    if let user = await auth.register(email: email, password: password) {
                        state.user = user
                        if let hh = await auth.createHousehold(name: "E2E Home", initials: "EH") {
                            state.household = hh
                            _ = await auth.seedDefaults()
                            await loadAppData()
                        }
                    } else {
                        // Registration failed — show login screen
                        await auth.logout()
                    }
                    hasCheckedSession = true
                } else {
                    await checkSession()
                    hasCheckedSession = true
                    // Load regardless of /api/me's householdId: a membership an
                    // administrator has disabled has no active household, yet
                    // /api/household is what reports the suspension. Gating on
                    // householdId here would send a disabled member straight to
                    // the create/join onboarding screen.
                    if state.user != nil, !state.showingSavedHome {
                        await loadAppData()
                    }
                }
            }
        }
        .onChange(of: state.user) { oldUser, newUser in
            if newUser == nil {
                hasLoadedData = false
            } else if !state.showingSavedHome {
                // Load for every sign-in, not only ones with an active
                // household: a disabled membership has none, and /api/household
                // is what surfaces the suspension message.
                Task { await loadAppData() }
            }
            if oldUser == nil, newUser != nil {
                // Fresh sign-in: re-register the push token if permission was
                // already granted (mirrors the PWA's maybeSubscribePush after
                // login), and consume a /join link that arrived logged-out.
                if !state.showingSavedHome {
                    Task { await PushRegistrationController.shared.syncIfAuthorized() }
                }
                if let code = state.pendingInviteCode, newUser?.householdId == nil {
                    Task { await consumePendingInvite(code) }
                }
            }
        }
        .onChange(of: state.showingSavedHome) { wasSaved, isSaved in
            // The server confirmed the identity whose saved Home was showing:
            // load fresh data and replay the journal, as after a sign-in.
            guard wasSaved, !isSaved, state.user != nil else { return }
            Task { await loadAppData() }
            Task { await PushRegistrationController.shared.syncIfAuthorized() }
        }
        .task(id: state.showingSavedHome) {
            // An unreachable server sends no connectivity change; keep asking.
            while state.showingSavedHome {
                try? await Task.sleep(for: .seconds(30))
                guard !Task.isCancelled, state.showingSavedHome else { return }
                await dataLoader.foregroundRefresh()
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.willEnterForegroundNotification)) { _ in
            Task { await dataLoader.foregroundRefresh() }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didEnterBackgroundNotification)) { _ in
            dataLoader.saveHomeSnapshot()
        }
        // Universal links (verify email, magic login, invite) and the
        // quicklog deep link. SwiftUI delivers universal links through
        // onOpenURL; the NSUserActivity path covers hand-off style delivery.
        .onOpenURL { url in
            Task { await handleIncomingURL(url) }
        }
        .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { activity in
            if let url = activity.webpageURL {
                Task { await handleIncomingURL(url) }
            }
        }
    }

    /// Handles a universal link or deep link — same endpoints and outcomes as
    /// the PWA's route handling for /verify-email, /magic-login, and /join.
    func handleIncomingURL(_ url: URL) async {
        guard let link = DeepLink.parse(url), !handledLinks.contains(url) else { return }
        // onOpenURL and Handoff can deliver the same proof concurrently.
        handledLinks.insert(url)
        switch link {
        case .verifyEmail(let token):
            let _: StatusResponse? = try? await environment.apiClient.get(
                "/api/auth/email/verify", query: [URLQueryItem(name: "token", value: token)])
            // Refresh the session so emailVerified flips in Settings.
            if let user = await auth.loadSession() {
                state.user = user
            }
        case .magicLogin(let token):
            do {
                let response: UserResponse = try await environment.apiClient.get(
                    "/api/auth/magic-link/consume", query: [URLQueryItem(name: "token", value: token)])
                if let user = response.user {
                    state.user = user
                }
            } catch {
                // Invalid/expired link: stay where we are, like the PWA.
            }
        case .joinHousehold(let code):
            if state.user == nil {
                // Consumed after sign-in; OnboardingView also prefills from it.
                state.pendingInviteCode = code
            } else if state.household == nil {
                await consumePendingInvite(code)
            }
            // Already in a household: the link just opens the app (PWA parity).
        case .quickLog(let target):
            state.currentTab = .home
            state.homeView = .log
            state.pendingQuickLog = target
        case .showHomeLog:
            state.currentTab = .home
            state.homeView = .log
        case .showActivity:
            state.currentTab = .activity
        }
    }

    private func consumePendingInvite(_ code: String) async {
        if let household = await auth.joinHousehold(code: code) {
            let owner = state.revision
            state.pendingInviteCode = nil
            _ = await auth.seedDefaults()
            guard state.revision == owner else { return }
            state.household = household
            state.activeHouseholdId = household.id
        }
    }

    /// The launch session check. An unreachable server says nothing about the
    /// session, so Home opens from this device's copy for the last confirmed
    /// identity, whose journal keeps accepting logs (PWA `openSavedHome`).
    private func checkSession() async {
        guard await auth.checkSession() == .unreachable,
              let saved = environment.apiClient.identity.resumeSavedHome() else { return }
        // Adopt directly: this check can finish before `AppEnvironment.configure`
        // installs the identity observer.
        state.adopt(environment.apiClient.identity.snapshot)
        state.applyHomeSnapshot(saved)
        hasLoadedData = true
    }

    func loadAppData() async {
        guard state.user != nil else { return }
        let owner = state.revision
        // Several triggers can fire for one sign-in (`onChange(of: state.user)`,
        // `onChange(of: state.household)`, the skeleton's own `.task`). Overlapping
        // runs cancel each other: the first to finish flips `hasLoadedData`, the
        // skeleton disappears, and SwiftUI cancels the other run's in-flight reads,
        // leaving Home signed in with no chores. One run per identity revision.
        guard loadRevision != owner else {
            NSLog("[Nabu] ContentView.loadAppData skipped: a load for this identity is already running")
            return
        }
        loadRevision = owner
        defer { loadRevision = nil }
        NSLog("[Nabu] ContentView.loadAppData calling reloadAfterAuth")
        await dataLoader.reloadAfterAuth()
        guard state.revision == owner else { return }
        hasLoadedData = state.household != nil
        NSLog("[Nabu] ContentView.loadAppData done. hasLoadedData=\(hasLoadedData)")
    }

    private func parseTestCreds(_ args: [String]) -> (String, String)? {
        // Format: -nabuAutoRegister email password (three consecutive args)
        if let idx = args.firstIndex(of: "-nabuAutoRegister"), idx + 2 < args.count {
            return (args[idx + 1], args[idx + 2])
        }
        // Format: -NabuEmail email -NabuPassword password
        if let ei = args.firstIndex(of: "-NabuEmail"), ei + 1 < args.count,
           let pi = args.firstIndex(of: "-NabuPassword"), pi + 1 < args.count {
            return (args[ei + 1], args[pi + 1])
        }
        return nil
    }
}

struct MainTabView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @ObservedObject var dataLoader: DataLoader

    var body: some View {
        tabs
            .id(state.revision)
            .overlay(alignment: .top) {
                VStack(spacing: 6) {
                    TimerChipView()
                    PendingSavesButton(dataLoader: dataLoader)
                    if state.isOffline || state.showingSavedHome {
                        OfflineBanner(savedHome: state.showingSavedHome)
                            .transition(.move(edge: .top).combined(with: .opacity))
                    }
                }
                .padding(.top, 4)
                .animation(Motion.slide ?? Motion.fade, value: state.isOffline || state.showingSavedHome)
            }
    }

    private var tabs: some View {
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
                // badge(0) renders nothing, so the hidden preference simply
                // suppresses the count; notifications still accumulate.
                .badge(state.hideNotificationBadge ? 0 : state.unreadNotifications)
                .tag(MainTab.settings)
        }
        .tint(DesignColors.primary)
    }
}

struct PendingSavesButton: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @ObservedObject var dataLoader: DataLoader
    @ObservedObject private var queue = OfflineLogQueue.shared
    @State private var showingPending = false
    @State private var discardKey: String?
    @State private var storageError = false

    var body: some View {
        if queue.storageUnavailable {
            VStack {
                Text("Pending saves could not be read. Your device copy has been kept.").font(.caption)
                Button("Retry reading saved data") { queue.reload() }
            }
        }
        if let origin = environment.apiClient.identity.snapshot.origin {
            let items = queue.scopedItems(origin)
            if !items.isEmpty {
                Button("\(items.count) pending save\(items.count == 1 ? "" : "s")") { showingPending = true }
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("pending-saves")
                    .sheet(isPresented: $showingPending) {
                        NavigationStack {
                            List {
                                Section {
                                    Text("These requests are saved on this device. Server confirmation is still pending.")
                                    Button("Retry saves") { Task { await dataLoader.flushOfflineQueue(retryFailed: true) } }
                                        .disabled(!queue.inFlight.isEmpty)
                                }
                                ForEach(queue.scopedItems(origin), id: \.body.idempotencyKey) { item in
                                    VStack(alignment: .leading, spacing: 8) {
                                        Text(state.chores.first(where: { $0.id == item.body.choreId })?.name ?? "Chore save")
                                            .font(.headline)
                                        if let note = item.body.note, !note.isEmpty { Text(note) }
                                        Text(item.failure ?? "Waiting for confirmation").font(.caption)
                                        Button("Discard saved request", role: .destructive) { discardKey = item.body.idempotencyKey }
                                            .disabled(queue.inFlight.contains(item.body.idempotencyKey ?? ""))
                                    }
                                }
                                if storageError { Text("Could not remove this request from your device. Retry.").foregroundStyle(.red) }
                            }
                            .navigationTitle("Pending saves")
                            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { showingPending = false } } }
                            .confirmationDialog("Discard this saved request?", isPresented: Binding(
                                get: { discardKey != nil }, set: { if !$0 { discardKey = nil } }
                            )) {
                                Button("Discard", role: .destructive) {
                                    guard let key = discardKey,
                                          environment.apiClient.identity.snapshot.origin == origin else { return }
                                    storageError = !queue.discard(key: key, origin: origin)
                                    if !storageError {
                                        state.pendingLogs.removeAll { $0.id == key }
                                        if state.activeTimer?.idempotencyKey == key, DurationTimer.save(nil, origin: origin) {
                                            state.activeTimer = nil
                                        }
                                    }
                                    discardKey = nil
                                }
                            } message: {
                                Text("This removes the device copy. A request whose response was lost may already exist on the server; check Activity before logging it again.")
                            }
                        }
                    }
            }
        }
    }
}
