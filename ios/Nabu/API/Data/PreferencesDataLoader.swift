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
            // The last good preferences stay, as in the PWA.
            Log.loading.failure("Load preferences", error)
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
            Log.actions.failure("Save the Stats order", error)
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
            Log.actions.failure("Save hidden Stats sections", error)
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
            Log.actions.failure("Save Stats widgets", error)
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
            Log.actions.failure("Save the volume unit", error)
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
            Log.actions.failure("Save the notification badge setting", error)
            guard state.owns(owner) else { return }
            state.hideNotificationBadge = previous
        }
    }

    /// Puts Home in `order` now, then saves it; rolls back if the save fails
    /// (PWA `saveChoreOrder`). The change is applied before this returns, so
    /// a List's move lands on data that has already moved.
    @discardableResult
    func saveChoreOrder(_ order: [Int]) -> Task<Void, Never> {
        let api = self.api.scoped(), state = self.state
        let owner = state.beginOperation("saveChoreOrder")
        _ = state.beginOperation("loadPreferences")
        let previous = state.choreOrder
        state.choreOrder = order
        return Task {
            do {
                let patch = PatchUserPreferencesRequest(choreOrder: order)
                let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
                guard state.owns(owner) else { return }
                state.choreOrder = data.preferences.choreOrder
            } catch {
                Log.actions.failure("Save the activity order", error)
                guard state.owns(owner) else { return }
                state.choreOrder = previous
            }
        }
    }

    /// Hides `ids` from Home now, then saves it; rolls back if the save fails
    /// (PWA `saveHiddenHomeChores`).
    @discardableResult
    func saveHiddenHomeChores(_ ids: [Int]) -> Task<Void, Never> {
        let api = self.api.scoped(), state = self.state
        let owner = state.beginOperation("saveHiddenHomeChores")
        _ = state.beginOperation("loadPreferences")
        let previous = state.hiddenHomeChoreIDs
        state.hiddenHomeChoreIDs = ids
        return Task {
            do {
                let patch = PatchUserPreferencesRequest(hiddenHomeChoreIds: ids)
                let data: UserPreferencesResponse = try await api.patch("/api/preferences", body: patch)
                guard state.owns(owner) else { return }
                state.hiddenHomeChoreIDs = data.preferences.hiddenHomeChoreIds
            } catch {
                Log.actions.failure("Save the hidden activities", error)
                guard state.owns(owner) else { return }
                state.hiddenHomeChoreIDs = previous
            }
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
            Log.api.failure("Sync the time zone", error)
        }
    }
}
