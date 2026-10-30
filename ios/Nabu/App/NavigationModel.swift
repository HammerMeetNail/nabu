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

    /// The activity the target opens, from the household's activities and
    /// Home's tiles in order. "Feed Baby", the Log Feed shortcut, falls back
    /// to the first feeding tile on Home, so that a named baby's "Feed Ada" or
    /// a household's "Feed Cats" opens too (the PWA's `quickLogChore`). Nil
    /// lands on Home's grid.
    func chore(in chores: [Chore], home: [Chore]) -> Chore? {
        switch self {
        case .chore(let id):
            return chores.first { $0.id == id }
        case .predefined(let key):
            let named = chores.first { $0.predefinedKey == key } ?? chores.first { $0.name == key }
            guard named == nil, key == "Feed Baby" else { return named }
            return home.first { $0.category == "feeding" }
        }
    }
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

struct Toast: Identifiable {
    let id = UUID()
    let message: String
    let isUndo: Bool
    var undoAction: (() -> Void)?
}
