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
        let api = self.api.scoped()
        let owner = state.beginOperation("loadPreferences")
        do {
            let data: UserPreferencesResponse = try await api.get("/api/preferences")
            guard state.owns(owner) else { return }
            state.choreOrder = data.preferences.choreOrder
            state.hiddenHomeChoreIDs = data.preferences.hiddenHomeChoreIds
            state.volumeUnit = data.preferences.volumeUnit == "oz" ? "oz" : "ml"
            state.hideNotificationBadge = data.preferences.hideNotificationBadge
            state.statsSectionOrder = data.preferences.statsSectionOrder
            state.statsSectionHidden = data.preferences.statsSectionHidden
            state.statsWidgets = data.preferences.statsWidgets
        } catch {
            guard state.owns(owner) else { return }
            // Silent failure
        }
    }

    /// Persists the stats section order; the server echo wins (PWA
    /// `saveStatsSectionOrder`).
    @discardableResult
    func saveStatsSectionOrder(_ order: [String]) async -> Bool {
        let api = self.api.scoped()
        let owner = state.beginOperation("saveStatsSectionOrder")
        _ = state.beginOperation("loadPreferences")
        state.statsSectionOrder = order
        do {
            let patch = PatchUserPreferencesRequest(statsSectionOrder: order)
            let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
            guard state.owns(owner) else { return false }
            state.statsSectionOrder = data.preferences.statsSectionOrder
            return true
        } catch {
            guard state.owns(owner) else { return false }
            return false
        }
    }

    /// Persists the hidden stats sections (PWA `saveStatsSectionHidden`).
    @discardableResult
    func saveStatsSectionHidden(_ hidden: [String]) async -> Bool {
        let api = self.api.scoped()
        let owner = state.beginOperation("saveStatsSectionHidden")
        _ = state.beginOperation("loadPreferences")
        state.statsSectionHidden = hidden
        do {
            let patch = PatchUserPreferencesRequest(statsSectionHidden: hidden)
            let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
            guard state.owns(owner) else { return false }
            state.statsSectionHidden = data.preferences.statsSectionHidden
            return true
        } catch {
            guard state.owns(owner) else { return false }
            return false
        }
    }

    /// Persists the user-defined stats widgets. The server validates the
    /// schema and echoes back the normalized list (with server-assigned ids),
    /// which we store (PWA `saveStatsWidgets`).
    @discardableResult
    func saveStatsWidgets(_ widgets: [StatsWidget]) async -> Bool {
        let api = self.api.scoped()
        let owner = state.beginOperation("saveStatsWidgets")
        _ = state.beginOperation("loadPreferences")
        do {
            let patch = PatchUserPreferencesRequest(statsWidgets: widgets)
            let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
            guard state.owns(owner) else { return false }
            state.statsWidgets = data.preferences.statsWidgets
            return true
        } catch {
            guard state.owns(owner) else { return false }
            return false
        }
    }

    /// Optimistically applies the unit, then persists it; rolls back on
    /// failure (mirrors the PWA's `setVolumeUnit`).
    func setVolumeUnit(_ unit: String) async {
        let api = self.api.scoped()
        let owner = state.beginOperation("setVolumeUnit")
        _ = state.beginOperation("loadPreferences")
        let previous = state.volumeUnit
        state.volumeUnit = unit
        do {
            let patch = PatchUserPreferencesRequest(volumeUnit: unit)
            let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
            guard state.owns(owner) else { return }
            state.volumeUnit = data.preferences.volumeUnit == "oz" ? "oz" : "ml"
        } catch {
            guard state.owns(owner) else { return }
            state.volumeUnit = previous
        }
    }

    /// Optimistically applies the badge visibility, then persists it; rolls
    /// back on failure (mirrors the PWA's `saveHideNotificationBadge`).
    func setHideNotificationBadge(_ hide: Bool) async {
        let api = self.api.scoped()
        let owner = state.beginOperation("setHideNotificationBadge")
        _ = state.beginOperation("loadPreferences")
        let previous = state.hideNotificationBadge
        state.hideNotificationBadge = hide
        do {
            let patch = PatchUserPreferencesRequest(hideNotificationBadge: hide)
            let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
            guard state.owns(owner) else { return }
            state.hideNotificationBadge = data.preferences.hideNotificationBadge
        } catch {
            guard state.owns(owner) else { return }
            state.hideNotificationBadge = previous
        }
    }

    func syncTimezone() async {
        let api = self.api.scoped()
        let owner = state.beginOperation("syncTimezone")
        _ = state.beginOperation("loadPreferences")
        let systemTZ = TimeZone.current.identifier
        guard state.household != nil else { return }
        do {
            let data: UserPreferencesResponse = try await api.get("/api/preferences")
            guard state.owns(owner) else { return }
            if data.preferences.timezone != systemTZ {
                let patch = PatchUserPreferencesRequest(timezone: systemTZ)
                let _: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
                guard state.owns(owner) else { return }
            }
        } catch {
            guard state.owns(owner) else { return }
            // Silent failure
        }
    }
}
