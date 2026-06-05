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

enum ActivityViewMode: CaseIterable, Hashable {
    case history
    case day
    case week

    var title: String {
        switch self {
        case .history: return "History"
        case .day: return "Day"
        case .week: return "Week"
        }
    }
}

enum CalendarViewMode {
    case day
    case week
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
