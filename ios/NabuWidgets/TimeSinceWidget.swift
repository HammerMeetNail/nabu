import AppIntents
import SwiftUI
import WidgetKit

// MARK: - Activities

/// An activity a Lock Screen widget or a control can show. The extension
/// can't reach the app's data, so it offers the activities Home last showed,
/// from the app-group snapshot.
struct WidgetActivityEntity: AppEntity {
    static let typeDisplayRepresentation = TypeDisplayRepresentation(name: "Activity")
    static let defaultQuery = WidgetActivityQuery()

    let id: Int
    let name: String
    let icon: String

    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(icon) \(name)") }
}

struct WidgetActivityQuery: EntityQuery {
    func entities(for identifiers: [Int]) async throws -> [WidgetActivityEntity] {
        activities().filter { identifiers.contains($0.id) }
    }

    func suggestedEntities() async throws -> [WidgetActivityEntity] {
        activities()
    }

    private func activities() -> [WidgetActivityEntity] {
        WidgetDataCache.read().map { WidgetActivityEntity(id: $0.id, name: $0.name, icon: $0.icon) }
    }
}

// MARK: - Lock Screen widget

/// The Lock Screen widget's setting. Without one it shows Home's first activity.
struct SelectActivityIntent: WidgetConfigurationIntent {
    static let title: LocalizedStringResource = "Activity"
    static let description = IntentDescription("The activity whose last log the widget shows.")

    @Parameter(title: "Activity")
    var activity: WidgetActivityEntity?
}

struct TimeSinceEntry: TimelineEntry {
    let date: Date
    let chore: WidgetChoreSnapshot?
}

struct TimeSinceProvider: AppIntentTimelineProvider {
    func placeholder(in context: Context) -> TimeSinceEntry {
        TimeSinceEntry(date: Date(), chore: WidgetDataCache.sampleChores.first)
    }

    func snapshot(for configuration: SelectActivityIntent, in context: Context) async -> TimeSinceEntry {
        let saved = WidgetDataCache.read()
        let chores = saved.isEmpty ? WidgetDataCache.sampleChores : saved
        return TimeSinceEntry(date: Date(), chore: WidgetDataCache.activity(id: configuration.activity?.id, in: chores))
    }

    func timeline(for configuration: SelectActivityIntent, in context: Context) async -> Timeline<TimeSinceEntry> {
        let chore = WidgetDataCache.activity(id: configuration.activity?.id, in: WidgetDataCache.read())
        // The circle's "3h" can't update itself, so an entry every 15 minutes
        // keeps it right for six hours. The app reloads the timeline on a log.
        let now = Date()
        let entries = (0..<24).map { TimeSinceEntry(date: now.addingTimeInterval(Double($0) * 15 * 60), chore: chore) }
        return Timeline(entries: entries, policy: .atEnd)
    }
}

/// "Fed 2 hr, 10 min ago" on the Lock Screen. A tap opens the activity's log
/// sheet, filled in, like the Home-Screen widget.
struct TimeSinceWidget: Widget {
    var body: some WidgetConfiguration {
        AppIntentConfiguration(kind: "NabuTimeSince", intent: SelectActivityIntent.self, provider: TimeSinceProvider()) { entry in
            TimeSinceView(entry: entry)
                .containerBackground(for: .widget) { Color.clear }
        }
        .configurationDisplayName("Time Since")
        .description("How long ago an activity was last logged.")
        .supportedFamilies([.accessoryCircular, .accessoryRectangular, .accessoryInline])
    }
}

struct TimeSinceView: View {
    @Environment(\.widgetFamily) private var widgetFamily
    let entry: TimeSinceEntry
    /// Overrides the family the widget reads, to render one outside WidgetKit.
    var family: WidgetFamily?

    var body: some View {
        if let chore = entry.chore {
            content(chore, family: family ?? widgetFamily)
                .widgetURL(WidgetDataCache.quickLogURL(choreID: chore.id))
        } else if (family ?? widgetFamily) == .accessoryCircular {
            Text("🏠")
                .font(.title2)
                .accessibilityLabel("Open Nabu")
        } else {
            Text("Open Nabu")
                .font(.caption)
        }
    }

    @ViewBuilder
    private func content(_ chore: WidgetChoreSnapshot, family: WidgetFamily) -> some View {
        switch family {
        case .accessoryCircular:
            ZStack {
                AccessoryWidgetBackground()
                VStack(spacing: 0) {
                    Text(chore.icon)
                        .font(.title3)
                    Text(chore.lastCompletedAt.map { WidgetTimeText.compact(since: $0, now: entry.date) } ?? "–")
                        .font(.caption2.weight(.semibold))
                        .minimumScaleFactor(0.7)
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityLabel(chore.name)
        case .accessoryInline:
            // The time first: a long name would push it off the line.
            if let date = chore.lastCompletedAt {
                Text("\(chore.icon) \(Text(date, style: .relative)) ago · \(chore.name)")
            } else {
                Text("\(chore.icon) \(chore.name): not yet")
            }
        default:
            VStack(alignment: .leading, spacing: 2) {
                Text("\(chore.icon) \(chore.name)")
                    .font(.headline)
                    .lineLimit(1)
                    .widgetAccentable()
                if let date = chore.lastCompletedAt {
                    Text("\(Text(date, style: .relative)) ago")
                        .lineLimit(1)
                        .minimumScaleFactor(0.8)
                } else {
                    Text("Not logged yet")
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

// MARK: - Control Center

/// The Control Center button's setting.
struct SelectControlActivityIntent: ControlConfigurationIntent {
    static let title: LocalizedStringResource = "Activity"

    @Parameter(title: "Activity")
    var activity: WidgetActivityEntity?
}

/// Opens an activity's log sheet, filled in. A control opens the sheet
/// rather than logging in place, since most activities need more than a tap.
struct OpenLogSheetIntent: AppIntent {
    static let title: LocalizedStringResource = "Open Log Sheet"
    static let isDiscoverable = false

    @Parameter(title: "Activity")
    var activity: WidgetActivityEntity?

    init() {}

    init(activity: WidgetActivityEntity?) {
        self.activity = activity
    }

    func perform() async throws -> some IntentResult & OpensIntent {
        // Without an activity, or before the app has saved any, Home's log grid.
        let chore = WidgetDataCache.activity(id: activity?.id, in: WidgetDataCache.read())
        let url = chore.map { WidgetDataCache.quickLogURL(choreID: $0.id) }
            ?? URL(string: "https://nabu-app.com/?quicklog=chore")!
        return .result(opensIntent: OpenURLIntent(url))
    }
}

struct OpenLogSheetControl: ControlWidget {
    var body: some ControlWidgetConfiguration {
        AppIntentControlConfiguration(kind: "NabuOpenLogSheet", intent: SelectControlActivityIntent.self) { configuration in
            ControlWidgetButton(action: OpenLogSheetIntent(activity: configuration.activity)) {
                Label(configuration.activity?.name ?? "Log Activity", systemImage: "plus.circle")
            }
        }
        .displayName("Log Activity")
        .description("Opens an activity's log sheet.")
    }
}
