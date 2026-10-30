import Foundation
#if canImport(WidgetKit)
import WidgetKit
#endif

/// One chore's "time since last log" snapshot, shared with the widget
/// extension through the app-group container. Compiled into both the app and
/// NabuWidgets targets.
struct WidgetChoreSnapshot: Codable, Identifiable {
    let id: Int
    let name: String
    let icon: String
    let lastCompletedAt: Date?
}

/// Writes the widget's data through the app group. The app refreshes it
/// whenever the latest-per-chore data loads (launch + foreground), which is
/// exactly when the numbers a widget shows could have changed.
enum WidgetDataCache {
    static let appGroupID = "group.com.nabu.app"
    static let snapshotKey = "widgetChoreSnapshots"

    static func write(chores: [WidgetChoreSnapshot]) {
        guard let defaults = UserDefaults(suiteName: appGroupID),
              let data = try? JSONEncoder().encode(chores) else { return }
        defaults.set(data, forKey: snapshotKey)
        #if canImport(WidgetKit)
        WidgetCenter.shared.reloadAllTimelines()
        #endif
    }

    /// The activity a widget or control shows: the one chosen in its
    /// settings, or Home's first when none was. A chosen activity that is
    /// gone shows nothing rather than another one.
    static func activity(id: Int?, in chores: [WidgetChoreSnapshot]) -> WidgetChoreSnapshot? {
        guard let id else { return chores.first }
        return chores.first { $0.id == id }
    }

    /// The activity's log sheet, filled in: the link the notification "Log
    /// now" action and the PWA's quick log use.
    static func quickLogURL(choreID: Int) -> URL {
        URL(string: "https://nabu-app.com/?quicklog=chore:\(choreID)")!
    }

    /// What the widget gallery shows before the app has saved anything.
    static var sampleChores: [WidgetChoreSnapshot] {
        [
            WidgetChoreSnapshot(id: 1, name: "Feed Baby", icon: "🍼", lastCompletedAt: Date().addingTimeInterval(-5400)),
            WidgetChoreSnapshot(id: 2, name: "Feed Cats", icon: "🐱", lastCompletedAt: Date().addingTimeInterval(-120)),
            WidgetChoreSnapshot(id: 3, name: "Walk Dog", icon: "🐕", lastCompletedAt: nil),
            WidgetChoreSnapshot(id: 4, name: "Vitamins", icon: "💊", lastCompletedAt: Date().addingTimeInterval(-86000)),
        ]
    }

    static func read() -> [WidgetChoreSnapshot] {
        guard let defaults = UserDefaults(suiteName: appGroupID),
              let data = defaults.data(forKey: snapshotKey),
              let chores = try? JSONDecoder().decode([WidgetChoreSnapshot].self, from: data) else {
            return []
        }
        return chores
    }

}

/// Time since a log, short enough for a Lock Screen circle.
enum WidgetTimeText {
    static func compact(since date: Date, now: Date) -> String {
        let minutes = max(0, Int(now.timeIntervalSince(date) / 60))
        switch minutes {
        case ..<1: return "now"
        case ..<60: return "\(minutes)m"
        case ..<(48 * 60): return "\(minutes / 60)h"
        default: return "\(minutes / (24 * 60))d"
        }
    }
}
