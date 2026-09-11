import SwiftUI

// User-defined stats widgets (Phase 4) and the customize panel. Widget
// semantics mirror the PWA (`stats.js` renderWidgetSection / wizard,
// `app.js` widget-* handlers): the six schema types render from the same
// endpoints; per-card day/week/month toggles persist into the stored widget.
// Titles are user data — rendered only as plain `Text`, never attributed.

// MARK: - Widget card

struct WidgetCard: View {
    @ObservedObject var model: StatsModel
    let widget: StatsWidget
    let chores: [Chore]
    let members: [Member]
    let latestLogs: [Int: ChoreLog]

    var body: some View {
        StatsCard {
            HStack {
                Text(widget.title.isEmpty ? "Widget" : widget.title)
                    .font(.headline).lineLimit(1)
                Spacer()
                // last-done has no period scope (PWA parity).
                if widget.type != "last-done" {
                    StatsPeriodToggle(period: widget.period.isEmpty ? "week" : widget.period, includeAll: false) { p in
                        Task { await model.setWidgetPeriod(p, widgetId: widget.id) }
                    }
                }
                Button {
                    Task { await model.removeWidget(id: widget.id) }
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(DesignColors.textSecondary.opacity(0.6))
                }
                .accessibilityLabel("Remove widget")
            }
            widgetBody
        }
    }

    @ViewBuilder
    private var widgetBody: some View {
        switch widget.type {
        case "last-done":
            let ordered = widget.choreIds.compactMap { id in chores.first { $0.id == id } }
            if ordered.isEmpty {
                Text("No chores").font(.caption).foregroundStyle(DesignColors.textSecondary)
            } else {
                LastDoneList(orderedChores: ordered, latestLogs: latestLogs)
            }
        case "member-split":
            // Period-scoped byMember merged across the widget's chores
            // (summary endpoint), sorted by count.
            let merged = Dictionary(
                (model.widgetSummaries[widget.id] ?? [])
                    .flatMap { $0.byMember }
                    .map { ($0.userId, $0.count) },
                uniquingKeysWith: +
            )
            let byMember = merged.map { (userId: $0.key, count: $0.value) }.sorted { $0.count > $1.count }
            MemberBarList(byMember: byMember, members: members)
        case "timeseries":
            // A chart needs buckets: the first chore's series at the
            // widget's grain (PWA uses data[0].ts).
            if let ts = model.widgetTimeSeries[widget.id]?.first, !ts.periods.isEmpty {
                let unit = Self.metricUnit(widget: widget, metricUnit: ts.metricUnit)
                if widget.metric == "amount" {
                    UnitAmountCharts(periods: ts.periods, fallback: unit, grain: StatsSections.widgetGrain(widget))
                } else {
                PeriodBarChart(
                    segments: PeriodBarData.valueSegments(ts.periods) { Double(Self.metricValue($0, metric: widget.metric)) },
                    periods: ts.periods,
                    grain: StatsSections.widgetGrain(widget),
                    unitLabel: unit.isEmpty ? "count" : unit,
                    seriesColors: ["value": DesignColors.primary],
                    summaryText: { p in
                        let v = Self.metricValue(p, metric: widget.metric)
                        return unit.isEmpty ? "\(v)" : "\(v) \(unit)"
                    }
                )
                }
            } else {
                Text("No data").font(.caption).foregroundStyle(DesignColors.textSecondary)
            }
        default:
            // "total" (and interval/top-list, like the PWA) → a big-number,
            // period-scoped aggregate from the summary endpoint.
            let summaries = model.widgetSummaries[widget.id] ?? []
            let total = summaries.reduce(0) { $0 + Self.metricValue($1, metric: widget.metric) }
            let unit = Self.metricUnit(widget: widget, metricUnit: summaries.first?.metricUnit)
            if widget.metric == "amount" {
                let amounts = Dictionary(summaries.flatMap { summary in
                    Array((summary.amountsByUnit ?? [summary.metricUnit ?? "": summary.totalML]).map { ($0.key, $0.value) })
                }, uniquingKeysWith: +)
                ForEach(amounts.keys.sorted(), id: \.self) { unit in
                    HStack(alignment: .firstTextBaseline, spacing: 4) {
                        Text("\(amounts[unit] ?? 0)").font(.system(size: 34, weight: .bold)).foregroundStyle(DesignColors.primary)
                        if !unit.isEmpty { Text(unit).font(.subheadline).foregroundStyle(DesignColors.textSecondary) }
                    }
                    .frame(maxWidth: .infinity, alignment: .center)
                }
            } else {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text("\(total)").font(.system(size: 34, weight: .bold))
                    .foregroundStyle(DesignColors.primary)
                if !unit.isEmpty {
                    Text(unit).font(.subheadline).foregroundStyle(DesignColors.textSecondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .center)
            }
        }
    }

    /// PWA `widgetMetricValue`: amount → stored total, duration → minutes,
    /// else occurrence count.
    static func metricValue(_ p: TimeSeriesPeriod, metric: String) -> Int {
        switch metric {
        case "amount": return p.totalML ?? 0
        case "duration": return Int((Double(p.totalDuration ?? 0) / 60.0).rounded())
        default: return p.count
        }
    }

    static func metricValue(_ s: ChoreSummary, metric: String) -> Int {
        switch metric {
        case "amount": return s.totalML
        case "duration": return Int((Double(s.totalDuration) / 60.0).rounded())
        default: return s.count
        }
    }

    /// PWA `widgetMetricUnit`.
    static func metricUnit(widget: StatsWidget, metricUnit: String?) -> String {
        switch widget.metric {
        case "amount": return metricUnit ?? ""
        case "duration": return "min"
        default: return ""
        }
    }
}

// MARK: - Widget wizard

/// The "Add widget" sheet (PWA `renderWidgetWizard`): name, multi-select
/// chores, presentation, value. Period is not chosen here — new widgets
/// default to "week" and carry a day/week/month toggle on the card.
struct WidgetWizardView: View {
    @ObservedObject var model: StatsModel
    let chores: [Chore]
    @Environment(\.dismiss) private var dismiss

    @State private var title = ""
    @State private var type = "total"
    @State private var metric = "count"
    @State private var selectedChoreIds: Set<Int> = []
    @State private var saveFailed = false

    private static let presentations = [
        ("total", "Big number"),
        ("timeseries", "Bar chart"),
        ("member-split", "Member split"),
        ("last-done", "Last done"),
    ]
    private static let metrics = [
        ("count", "Count"),
        ("amount", "Amount"),
        ("duration", "Duration"),
    ]

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("e.g. Bottles this week", text: $title)
                        .onChange(of: title) {
                            if title.count > 60 { title = String(title.prefix(60)) }
                        }
                } header: {
                    Text("Name")
                }

                Section {
                    if chores.isEmpty {
                        Text("No chores yet").foregroundStyle(DesignColors.textSecondary)
                    }
                    ForEach(chores) { chore in
                        Button {
                            if selectedChoreIds.contains(chore.id) {
                                selectedChoreIds.remove(chore.id)
                            } else {
                                selectedChoreIds.insert(chore.id)
                            }
                        } label: {
                            HStack {
                                Text("\(chore.icon) \(chore.name)")
                                    .foregroundStyle(DesignColors.textPrimary)
                                Spacer()
                                if selectedChoreIds.contains(chore.id) {
                                    Image(systemName: "checkmark")
                                        .foregroundStyle(DesignColors.primary)
                                }
                            }
                        }
                    }
                } header: {
                    Text("Chores")
                }

                Section {
                    Picker("Show as", selection: $type) {
                        ForEach(Self.presentations, id: \.0) { value, label in
                            Text(label).tag(value)
                        }
                    }
                    Picker("Value", selection: $metric) {
                        ForEach(Self.metrics, id: \.0) { value, label in
                            Text(label).tag(value)
                        }
                    }
                }

                if saveFailed {
                    Text("Failed to add widget")
                        .foregroundStyle(DesignColors.danger)
                }
            }
            .navigationTitle("Add widget")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") {
                        Task {
                            // The widget's chore list preserves the on-screen
                            // chore order (PWA checkbox order).
                            let ids = chores.map(\.id).filter { selectedChoreIds.contains($0) }
                            if await model.addWidget(title: title, type: type, metric: metric, choreIds: ids) {
                                dismiss()
                            } else {
                                saveFailed = true
                            }
                        }
                    }
                    .disabled(selectedChoreIds.isEmpty)
                }
            }
        }
    }
}

// MARK: - Customize panel

/// Section reorder/hide + widget management (PWA `renderCustomizePanel`):
/// native list reordering instead of drag-and-drop rows.
struct CustomizeStatsView: View {
    @ObservedObject var model: StatsModel
    let chores: [Chore]
    let widgets: [StatsWidget]
    let hidden: Set<String>
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List {
                Section {
                    ForEach(model.customizeKeys, id: \.self) { key in
                        HStack {
                            Toggle(isOn: Binding(
                                get: { !hidden.contains(key) },
                                set: { visible in Task { await model.setSectionVisible(key, visible: visible) } }
                            )) {
                                Text(StatsSections.label(for: key, chores: chores, widgets: widgets))
                                    .lineLimit(1)
                            }
                        }
                    }
                    .onMove { source, destination in
                        Task { await model.moveSections(from: source, to: destination) }
                    }
                } footer: {
                    Text("Drag to reorder. Toggles hide a section without deleting it.")
                }

                Section {
                    Button {
                        model.widgetWizardOpen = true
                    } label: {
                        Label("Add widget", systemImage: "plus")
                    }
                }
            }
            .environment(\.editMode, .constant(.active))
            .navigationTitle("Customize Stats")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }
}
