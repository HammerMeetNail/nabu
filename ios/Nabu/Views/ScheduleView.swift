import SwiftUI

struct ScheduleView: View {
    @EnvironmentObject var state: AppState
    @EnvironmentObject var environment: AppEnvironment
    @State private var showingEditSheet = false
    @State private var editingSchedule: ChoreSchedule?
    @State private var showingPickChore = false

    private let scheduleStore: ScheduleStore

    private var schedules: [ChoreSchedule] { state.schedules }

    init(scheduleStore: ScheduleStore) {
        self.scheduleStore = scheduleStore
    }

    var body: some View {
        NavigationStack {
            Group {
                if state.chores.isEmpty {
                    emptyView(icon: "🏠", title: "No chores set up yet",
                              message: "Use the Home tab to add chores.")
                } else if upcomingRows.isEmpty {
                    emptyView(icon: "📅", title: "Nothing upcoming",
                              message: "No active schedules for the next 14 days.")
                } else {
                    List {
                        ForEach(groupedUpcoming(), id: \.key) { group in
                            Section(group.key) {
                                ForEach(group.rows, id: \.schedule.id) { item in
                                    scheduleRow(item: item)
                                }
                            }
                        }
                    }
                    .listStyle(.plain)
                }
            }
            .navigationTitle("Schedule")
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

    private func emptyView(icon: String, title: String, message: String) -> some View {
        VStack(spacing: 16) {
            Text(icon)
                .font(.system(size: 48))
            Text(title)
                .font(.title3)
                .fontWeight(.semibold)
            Text(message)
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal)
        }
        .frame(maxHeight: .infinity)
    }

    private func tapLog(_ item: UpcomingItem) async {
        let now = Date()
        let isoFormatter = ISO8601DateFormatter()
        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"
        let completedAt = isoFormatter.string(from: now)
        let dateStr = df.string(from: now)
        let hour = Calendar.current.component(.hour, from: now)

        do {
            let body = CreateLogRequest(choreId: item.chore.id, note: nil, indicators: nil,
                                          date: dateStr, hour: hour, completedAt: completedAt,
                                          volumeML: nil, userId: nil, indicatorVolumes: nil,
                                          followUpMinutes: nil)
            let resp: LogResponse = try await environment.apiClient.post("/api/logs", body: body)
            state.todayLogs.insert(resp.log, at: 0)
        } catch {}
    }

    private func loadSchedules() async {
        do {
            state.schedules = try await scheduleStore.loadSchedules()
        } catch {}
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
                        Toggle("Stop repeating", isOn: $hasEndDate)
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
            recurrenceEnd: hasEndDate ? df.string(from: endDate) : nil,
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
                        Toggle("Stop repeating", isOn: $hasEndDate)
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
            endDate = end
        }
    }

    private func save() async {
        let hh = String(format: "%02d", selectedHour)
        let mm = String(format: "%02d", selectedMinute)
        let specificTime = "\(hh):\(mm)"

        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"

        let body = PatchScheduleRequest(
            choreId: nil, timePeriod: nil, specificTime: specificTime,
            frequencyType: frequencyType.rawValue, isActive: nil,
            daysOfWeek: frequencyType == .weekly ? Array(selectedDays) : nil,
            intervalDays: frequencyType == .everyNDays ? intervalDays : nil,
            dayOfMonth: nil, monthOfYear: nil,
            startDate: nil,
            recurrenceEnd: hasEndDate ? df.string(from: endDate) : nil
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
