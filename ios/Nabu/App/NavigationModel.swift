import Foundation

enum MainTab: CaseIterable {
    case stats
    case activity
    case home
    case schedule
    case settings

    var title: String {
        switch self {
        case .stats: return "Stats"
        case .activity: return "Activity"
        case .home: return "Home"
        case .schedule: return "Schedule"
        case .settings: return "Settings"
        }
    }

    var systemImage: String {
        switch self {
        case .stats: return "chart.bar"
        case .activity: return "waveform"
        case .home: return "house"
        case .schedule: return "clock"
        case .settings: return "gearshape"
        }
    }
}

enum HomeViewMode: CaseIterable, Hashable {
    case log
    case manage

    var title: String {
        switch self {
        case .log: return "Log"
        case .manage: return "Manage"
        }
    }
}

/// What a quick-log entry point wants pre-filled — from a notification
/// "Log now" action, a widget tap, or a Home-Screen quick action. Mirrors the
/// PWA's `?quicklog=` targets.
enum QuickLogTarget: Equatable {
    case chore(id: Int)
    /// Resolved against `predefinedKey` (falling back to name) once the
    /// chore list is loaded — e.g. "Feed Baby" for the Log-feed shortcut.
    case predefined(key: String)
}

enum ActiveSheet: Identifiable {
    case logSheet
    case pickChore
    case choreEdit
    case scheduleEdit
    case householdEdit
    case inviteCreate
    case memberRole

    var id: String {
        switch self {
        case .logSheet: return "logSheet"
        case .pickChore: return "pickChore"
        case .choreEdit: return "choreEdit"
        case .scheduleEdit: return "scheduleEdit"
        case .householdEdit: return "householdEdit"
        case .inviteCreate: return "inviteCreate"
        case .memberRole: return "memberRole"
        }
    }
}

/// A failed action's message, shown for a few seconds (the PWA's error
/// toast): above the tabs, or over a sheet, which would hide that one.
struct Toast: Identifiable, Equatable {
    let id = UUID()
    let message: String
}
