import SwiftUI

struct ScheduleView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var showingEditSheet = false
    @State private var editingSchedule: ChoreSchedule?
    @State private var showingPickChore = false
    @State private var loadError: String? = nil
    @State private var deletingSchedule: ChoreSchedule?
    @State private var logging: Set<String> = []
    @State private var logKeys: [String: String] = [:]
    @State private var logError: String?

    private let scheduleStore: ScheduleStore

    private var schedules: [ChoreSchedule] { state.schedules }

    init(scheduleStore: ScheduleStore) {
        self.scheduleStore = scheduleStore
    }

    var body: some View {
        NavigationStack {
            Group {
                if state.chores.isEmpty {
                    ContentUnavailableView {
                        Label("No chores set up yet", systemImage: "house")
                    } description: {
                        Text("Use the Home tab to add chores.")
                    } actions: {
                        Button("Go to Home") {
                            state.currentTab = .home
                        }
                        .buttonStyle(.borderedProminent)
                    }
                } else if upcomingRows.isEmpty {
                    ContentUnavailableView {
                        Label("Nothing upcoming", systemImage: "calendar.badge.clock")
                    } description: {
                        Text("No active schedules for the next 14 days.")
                    } actions: {
                        Button("Schedule a chore") { showingPickChore = true }
                            .buttonStyle(.borderedProminent)
                            .accessibilityIdentifier("empty-schedule-create")
                    }
                } else {
                    List {
                        ForEach(groupedUpcoming(), id: \.key) { group in
                            Section(group.key) {
                                ForEach(group.rows, id: \.schedule.id) { item in
                                    scheduleRow(item: item)
                                        .swipeActions(edge: .trailing) {
                                            Button(role: .destructive) {
                                                deletingSchedule = item.schedule
                                            } label: {
                                                Label("Delete", systemImage: "trash")
                                            }
                                            Button {
                                                editingSchedule = item.schedule
                                            } label: {
                                                Label("Edit", systemImage: "pencil")
                                            }
                                        }
                                        .contextMenu {
                                            Button {
                                                editingSchedule = item.schedule
                                            } label: {
                                                Label("Edit", systemImage: "pencil")
                                            }
                                            Button(role: .destructive) {
                                                deletingSchedule = item.schedule
                                            } label: {
                                                Label("Delete", systemImage: "trash")
                                            }
                                        }
                                }
                            }
                        }
                    }
                    .listStyle(.plain)
                    .refreshable {
                        await loadSchedules()
                    }
                }
            }
            .navigationTitle("Schedule")
            .safeAreaInset(edge: .bottom) {
                if let logError { Text(logError).font(.caption).foregroundStyle(.red).padding() }
            }
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    Button {
                        showingPickChore = true
                    } label: {
                        Image(systemName: "plus")
                    }
                }
            }
            .sheet(isPresented: $showingPickChore) {
                PickChoreSheet(state: state, scheduleStore: scheduleStore)
            }
            .sheet(item: $editingSchedule) { sch in
                EditScheduleSheet(state: state, schedule: sch, scheduleStore: scheduleStore)
            }
            .confirmationDialog(
                "Delete this schedule?",
                isPresented: Binding(
                    get: { deletingSchedule != nil },
                    set: { if !$0 { deletingSchedule = nil } }
                ),
                titleVisibility: .visible
            ) {
                Button("Delete", role: .destructive) {
                    if let sch = deletingSchedule {
                        Task { await deleteSchedule(sch) }
                    }
                    deletingSchedule = nil
                }
                Button("Cancel", role: .cancel) { deletingSchedule = nil }
            }
        }
        .task {
            await loadSchedules()
        }
    }

    struct UpcomingItem {
        let schedule: ChoreSchedule
        let chore: Chore
        let date: String
        let isDone: Bool
    }

    private var upcomingRows: [UpcomingItem] {
        let today = todayISO()
        var items: [UpcomingItem] = []
        for dayOffset in 0..<14 {
            let date = shiftISO(today, by: dayOffset)
            for sch in schedules where isActiveForDay(sch, date) {
                guard let chore = state.chores.first(where: { $0.id == sch.choreId }) else { continue }
                let f = DateFormatter()
                f.dateFormat = "yyyy-MM-dd"
                let isDone = sch.frequencyType != "once" && state.todayLogs.contains {
                    $0.choreId == sch.choreId && f.string(from: $0.completedAt) == date
                }
                items.append(UpcomingItem(schedule: sch, chore: chore, date: date, isDone: isDone))
            }
        }
        return items
    }

    private func groupedUpcoming() -> [(key: String, rows: [UpcomingItem])] {
        let today = todayISO()
        var groups: [String: [UpcomingItem]] = [:]
        for item in upcomingRows {
            let label = item.date == today ? "Today" : fmtShortDate(item.date)
            groups[label, default: []].append(item)
        }
        return groups.sorted { $0.key < $1.key }.map { (key: $0.key, rows: $0.value) }
    }

    @ViewBuilder
    private func scheduleRow(item: UpcomingItem) -> some View {
        HStack(spacing: 12) {
            Text(item.chore.icon)
                .font(.title3)
                .frame(width: 36, height: 36)
                .background(Color(hex: item.chore.color) ?? .gray)
                .clipShape(RoundedRectangle(cornerRadius: 8))

            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(item.chore.name)
                        .font(.subheadline)
                        .fontWeight(.medium)
                    if let time = item.schedule.specificTime {
                        Text(fmtScheduleTime(time))
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                }
                Text(recurrenceSummary(item.schedule))
                    .font(.caption)
                    .foregroundColor(.secondary)
            }

            Spacer()

            if item.date == todayISO() && !item.isDone {
                Button {
                    Task { await tapLog(item) }
                } label: {
                    Image(systemName: "checkmark")
                        .font(.caption)
                        .fontWeight(.bold)
                        .padding(6)
                        .background(Color.accentColor)
                        .foregroundColor(.white)
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
                .disabled(logging.contains("\(item.schedule.id):\(item.date)"))
            }

            Button {
                editingSchedule = item.schedule
            } label: {
                Image(systemName: "pencil")
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
            .buttonStyle(.plain)
        }
        .background(item.isDone ? Color(hex: "#fef3c7") : Color.clear)
        .clipShape(RoundedRectangle(cornerRadius: 8))
        .padding(.horizontal, 4)
        .padding(.vertical, 1)
        .overlay(
            Rectangle()
                .fill(Color(hex: item.chore.color) ?? .gray)
                .frame(width: 3),
            alignment: .leading
        )
    }

    private func tapLog(_ item: UpcomingItem) async {
        let owner = state.revision
        let itemKey = "\(item.schedule.id):\(item.date)"
        guard logging.insert(itemKey).inserted else { return }
        defer { logging.remove(itemKey) }
        if logKeys[itemKey] == nil { logKeys[itemKey] = UUID().uuidString }
        let key = logKeys[itemKey]!
        let store = LogStore(api: scheduleStore.api)
        logError = nil
        let now = Date()
        let isoFormatter = ISO8601DateFormatter()
        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"
        let completedAt = isoFormatter.string(from: now)
        let dateStr = df.string(from: now)
        let hour = Calendar.current.component(.hour, from: now)

        do {
            let outcome = try await store.createLog(choreId: item.chore.id, date: dateStr,
                slotHour: hour, completedAt: completedAt, idempotencyKey: key)
            guard state.revision == owner else { return }
            switch outcome {
            case .created(let response):
                state.todayLogs.insert(response.log, at: 0)
                state.latestLogs[item.chore.id] = response.log
                logKeys.removeValue(forKey: itemKey)
                await loadSchedules()
            case .queued(let pending):
                if !state.pendingLogs.contains(where: { $0.id == pending.id }) { state.pendingLogs.insert(pending, at: 0) }
            }
        } catch {
            guard state.revision == owner else { return }
            logError = (error as? APIError)?.errorDescription ?? "Could not confirm the save. Retry from Pending saves."
        }
    }

    private func deleteSchedule(_ schedule: ChoreSchedule) async {
        let owner = state.revision
        do {
            let _: StatusResponse = try await scheduleStore.deleteSchedule(id: schedule.id)
            guard state.revision == owner else { return }
            state.schedules.removeAll { $0.id == schedule.id }
        } catch {}
    }

    private func loadSchedules() async {
        let owner = state.beginOperation("loadSchedules")
        do {
            let schedules = try await scheduleStore.loadSchedules()
            guard state.owns(owner) else { return }
            state.schedules = schedules
            loadError = nil
        } catch {
            guard state.owns(owner) else { return }
            loadError = error.localizedDescription
        }
    }
}

// MARK: - Pick Chore Sheet

struct PickChoreSheet: View {
    @Environment(\.dismiss) private var dismiss
    let state: AppState
    let scheduleStore: ScheduleStore
    @State private var selectedTime = Calendar.current.component(.hour, from: Date())
    @State private var selectedMinute = 0
    @State private var frequencyType: FreqType = .once
    @State private var selectedDays: Set<Int> = []
    @State private var intervalDays = 2
    @State private var hasEndDate = false
    @State private var endDate = Date().addingTimeInterval(86400 * 90)

    var body: some View {
        NavigationStack {
            Form {
                Section("Time") {
                    HStack {
                        Picker("Hour", selection: $selectedTime) {
                            ForEach(0..<24, id: \.self) { h in
                                Text(fmtHour(h)).tag(h)
                            }
                        }
                        Text(":")
                        Picker("Min", selection: $selectedMinute) {
                            ForEach(Array(stride(from: 0, to: 60, by: 5)), id: \.self) { m in
                                Text(String(format: "%02d", m)).tag(m)
                            }
                        }
                    }
                }

                Section("Repeat") {
                    Picker("Frequency", selection: $frequencyType) {
                        ForEach(FreqType.allCases, id: \.self) { f in
                            Text(f.label).tag(f)
                        }
                    }

                    if frequencyType == .weekly {
                        dayPills
                    }
                    if frequencyType == .everyNDays {
                        Stepper("Every \(intervalDays) days", value: $intervalDays, in: 2...365)
                    }
                    if frequencyType != .once {
                        Toggle("Repeat through (inclusive)", isOn: $hasEndDate)
                        if hasEndDate {
                            DatePicker("End date", selection: $endDate, displayedComponents: .date)
                        }
                    }
                }

                Section("Chores") {
                    ForEach(state.chores) { chore in
                        Button {
                            Task { await scheduleChore(chore) }
                        } label: {
                            HStack {
                                Text(chore.icon)
                                Text(chore.name)
                                    .foregroundColor(.primary)
                                Spacer()
                            }
                        }
                    }
                }
            }
            .navigationTitle("Add to Schedule")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
    }

    private var dayPills: some View {
        HStack {
            ForEach(0..<7, id: \.self) { day in
                Button {
                    if selectedDays.contains(day) {
                        selectedDays.remove(day)
                    } else {
                        selectedDays.insert(day)
                    }
                } label: {
                    Text(DAY_NAMES_SHORT[day])
                        .font(.caption)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(selectedDays.contains(day) ? Color.accentColor : DesignColors.surfaceSecondary)
                        .foregroundColor(selectedDays.contains(day) ? .white : .primary)
                        .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func scheduleChore(_ chore: Chore) async {
        let hh = String(format: "%02d", selectedTime)
        let mm = String(format: "%02d", selectedMinute)
        let specificTime = "\(hh):\(mm)"

        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"

        let body = CreateScheduleRequest(
            choreId: chore.id,
            frequencyType: frequencyType.rawValue,
            timePeriod: "anytime",
            specificTime: specificTime,
            daysOfWeek: frequencyType == .weekly ? Array(selectedDays) : nil,
            intervalDays: frequencyType == .everyNDays ? intervalDays : nil,
            dayOfMonth: nil,
            monthWeekday: nil,
            monthOfYear: nil,
            startDate: df.string(from: Date()),
            recurrenceEnd: hasEndDate ? scheduleEndTimestamp(endDate) : nil,
            targetCount: nil,
            isActive: true,
            assignedUserId: nil
        )

        do {
            let _ = try await scheduleStore.createSchedule(body: body)
            dismiss()
        } catch {}
    }
}

// MARK: - Edit Schedule Sheet

struct EditScheduleSheet: View {
    @Environment(\.dismiss) private var dismiss
    let state: AppState
    let schedule: ChoreSchedule
    let scheduleStore: ScheduleStore

    @State private var selectedHour = 8
    @State private var selectedMinute = 0
    @State private var frequencyType: FreqType = .once
    @State private var selectedDays: Set<Int> = []
    @State private var intervalDays = 2
    @State private var hasEndDate = false
    @State private var endDate = Date()
    @State private var isDeleting = false

    private var chore: Chore? { state.chores.first(where: { $0.id == schedule.choreId }) }

    var body: some View {
        NavigationStack {
            Form {
                if let chore = chore {
                    Section {
                        HStack {
                            Text(chore.icon)
                                .font(.largeTitle)
                            Text(chore.name)
                                .font(.headline)
                        }
                    }
                }

                Section("Time") {
                    HStack {
                        Picker("Hour", selection: $selectedHour) {
                            ForEach(0..<24, id: \.self) { h in
                                Text(fmtHour(h)).tag(h)
                            }
                        }
                        Text(":")
                        Picker("Min", selection: $selectedMinute) {
                            ForEach(Array(stride(from: 0, to: 60, by: 5)), id: \.self) { m in
                                Text(String(format: "%02d", m)).tag(m)
                            }
                        }
                    }
                }

                Section("Repeat") {
                    Picker("Frequency", selection: $frequencyType) {
                        ForEach(FreqType.allCases, id: \.self) { f in
                            Text(f.label).tag(f)
                        }
                    }
                    if frequencyType == .weekly { dayPills }
                    if frequencyType == .everyNDays {
                        Stepper("Every \(intervalDays) days", value: $intervalDays, in: 2...365)
                    }
                    if frequencyType != .once {
                        Toggle("Repeat through (inclusive)", isOn: $hasEndDate)
                        if hasEndDate {
                            DatePicker("End date", selection: $endDate, displayedComponents: .date)
                        }
                    }
                }

                Section {
                    Button("Save") { Task { await save() } }
                        .disabled(isDeleting)
                }

                Section {
                    Button("Remove from schedule", role: .destructive) {
                        Task { await delete() }
                    }
                    .disabled(isDeleting)
                }
            }
            .navigationTitle("Edit Schedule")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .onAppear {
            loadFromSchedule()
        }
    }

    private var dayPills: some View {
        HStack {
            ForEach(0..<7, id: \.self) { day in
                Button {
                    if selectedDays.contains(day) { selectedDays.remove(day) }
                    else { selectedDays.insert(day) }
                } label: {
                    Text(DAY_NAMES_SHORT[day])
                        .font(.caption)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(selectedDays.contains(day) ? Color.accentColor : DesignColors.surfaceSecondary)
                        .foregroundColor(selectedDays.contains(day) ? .white : .primary)
                        .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func loadFromSchedule() {
        if let time = schedule.specificTime {
            let parts = time.split(separator: ":")
            selectedHour = Int(parts[0]) ?? 8
            selectedMinute = Int(parts[1]) ?? 0
        }
        frequencyType = FreqType(rawValue: schedule.frequencyType) ?? .once
        selectedDays = Set(schedule.daysOfWeek)
        intervalDays = max(schedule.intervalDays, 2)
        if let end = schedule.recurrenceEnd {
            hasEndDate = true
            endDate = scheduleEndSelection(end)
        }
    }

    private func save() async {
        let hh = String(format: "%02d", selectedHour)
        let mm = String(format: "%02d", selectedMinute)
        let specificTime = "\(hh):\(mm)"

        let body = PatchScheduleRequest(
            choreId: nil, timePeriod: nil, specificTime: specificTime,
            frequencyType: frequencyType.rawValue, isActive: nil,
            daysOfWeek: frequencyType == .weekly ? Array(selectedDays) : nil,
            intervalDays: frequencyType == .everyNDays ? intervalDays : nil,
            dayOfMonth: nil, monthOfYear: nil,
            startDate: nil,
            recurrenceEnd: .some(hasEndDate ? scheduleEndTimestamp(endDate) : nil)
        )

        do {
            let _ = try await scheduleStore.updateSchedule(id: schedule.id, body: body)
            dismiss()
        } catch {}
    }

    private func delete() async {
        isDeleting = true
        do {
            let _ = try await scheduleStore.deleteSchedule(id: schedule.id)
            dismiss()
        } catch {
            isDeleting = false
        }
    }
}
