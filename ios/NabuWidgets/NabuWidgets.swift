import SwiftUI
import WidgetKit

/// Home-Screen widget: "time since last ⟨chore⟩" (P6/B8), plus the Lock
/// Screen widget and the Control Center button (TimeSinceWidget.swift). Reads the
/// app-group snapshot the app refreshes on launch/foreground; relative
/// times render with self-updating date text so the widget stays honest
/// between timeline reloads.
@main
struct NabuWidgetsBundle: WidgetBundle {
    var body: some Widget {
        LastLoggedWidget()
        TimeSinceWidget()
        OpenLogSheetControl()
    }
}

struct LastLoggedEntry: TimelineEntry {
    let date: Date
    let chores: [WidgetChoreSnapshot]
}

struct LastLoggedProvider: TimelineProvider {
    func placeholder(in context: Context) -> LastLoggedEntry {
        LastLoggedEntry(date: Date(), chores: LastLoggedProvider.sampleChores)
    }

    func getSnapshot(in context: Context, completion: @escaping (LastLoggedEntry) -> Void) {
        let chores = WidgetDataCache.read()
        completion(LastLoggedEntry(date: Date(), chores: chores.isEmpty ? Self.sampleChores : chores))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<LastLoggedEntry>) -> Void) {
        let entry = LastLoggedEntry(date: Date(), chores: WidgetDataCache.read())
        // Relative times self-update; a refresh every 30 minutes keeps the
        // chore list itself from going stale if the app isn't opened.
        let next = Calendar.current.date(byAdding: .minute, value: 30, to: Date()) ?? Date().addingTimeInterval(1800)
        completion(Timeline(entries: [entry], policy: .after(next)))
    }

    static var sampleChores: [WidgetChoreSnapshot] { WidgetDataCache.sampleChores }
}

struct LastLoggedWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "NabuLastLogged", provider: LastLoggedProvider()) { entry in
            LastLoggedWidgetView(entry: entry)
                .containerBackground(.fill.tertiary, for: .widget)
        }
        .configurationDisplayName("Last Logged")
        .description("Time since each chore was last logged.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

struct LastLoggedWidgetView: View {
    @Environment(\.widgetFamily) private var family
    let entry: LastLoggedEntry

    var body: some View {
        if entry.chores.isEmpty {
            VStack(spacing: 6) {
                Text("🏠")
                Text("Open Nabu to set up chores")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
        } else if family == .systemSmall, let first = entry.chores.first {
            // Small: the top chore, front and center. Tapping deep-links to
            // its pre-filled log sheet (same target as "Log now").
            VStack(alignment: .leading, spacing: 4) {
                Text(first.icon)
                    .font(.title)
                Text(first.name)
                    .font(.headline)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
                lastLoggedText(first.lastCompletedAt)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
            .widgetURL(quickLogURL(for: first))
        } else {
            // Medium: up to four chores with relative times.
            VStack(alignment: .leading, spacing: 8) {
                ForEach(entry.chores.prefix(4)) { chore in
                    Link(destination: quickLogURL(for: chore)) {
                        HStack(spacing: 8) {
                            Text(chore.icon)
                            Text(chore.name)
                                .font(.subheadline.weight(.medium))
                                .lineLimit(1)
                            Spacer(minLength: 4)
                            lastLoggedText(chore.lastCompletedAt)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder
    private func lastLoggedText(_ date: Date?) -> some View {
        if let date {
            Text(date, style: .relative)
        } else {
            Text("never")
        }
    }

    private func quickLogURL(for chore: WidgetChoreSnapshot) -> URL {
        WidgetDataCache.quickLogURL(choreID: chore.id)
    }
}
