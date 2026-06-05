import SwiftUI

@main
struct NabuApp: App {
    @StateObject private var state = AppState()
    @StateObject private var environment = AppEnvironment()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(state)
                .environmentObject(environment)
                .onAppear {
                    environment.configure(with: state)
                }
                .tint(DesignColors.primary)
        }
        .commands {
            CommandGroup(replacing: .newItem) {}
        }
    }
}
