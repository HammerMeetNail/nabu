import Foundation

@MainActor
final class PreferencesDataLoader {
    let api: APIClient
    let state: AppState

    init(api: APIClient, state: AppState) {
        self.api = api
        self.state = state
    }

    func loadPreferences() async {
        do {
            let data: UserPreferencesResponse = try await api.get("/api/preferences")
            state.choreOrder = data.preferences.choreOrder
            state.hiddenHomeChoreIDs = data.preferences.hiddenHomeChoreIds
        } catch {
            // Silent failure
        }
    }

    func syncTimezone() async {
        let systemTZ = TimeZone.current.identifier
        guard state.household != nil else { return }
        do {
            let data: UserPreferencesResponse = try await api.get("/api/preferences")
            if data.preferences.timezone != systemTZ {
                let patch = PatchUserPreferencesRequest(timezone: systemTZ)
                let _: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
            }
        } catch {
            // Silent failure
        }
    }
}
