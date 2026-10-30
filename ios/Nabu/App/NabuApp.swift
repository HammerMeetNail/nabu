import SwiftUI

@main
struct NabuApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var state = AppState()
    @State private var environment = AppEnvironment()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(state)
                .environment(environment)
                .onAppear {
                    environment.configure(with: state)
                    appDelegate.attach(appState: state)
                    PushRegistrationController.shared.configure(api: environment.apiClient)
                }
                .tint(DesignColors.primary)
        }
        .commands {
            CommandGroup(replacing: .newItem) {}
            // With a hardware keyboard, as / does in the PWA's wide layout:
            // in the Mac's menu bar, and in an iPad's ⌘ overlay.
            CommandMenu("Activity") {
                // Checked when pressed: the menu is built at launch, before
                // the account loads, and isn't rebuilt when it does.
                Button("Search Activity") {
                    guard state.user != nil, state.household != nil else { return }
                    state.currentTab = .activity
                    state.activitySearchRequested = true
                }
                .keyboardShortcut("f", modifiers: .command)
            }
        }
    }
}
