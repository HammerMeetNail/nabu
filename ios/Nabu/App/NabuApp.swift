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
        }
    }
}
