import SwiftUI

struct LogSheet: View {
    @Environment(\.dismiss) private var dismiss
    let state: AppState
    let chore: Chore
    let log: ChoreLog?
    let logStore: LogStore
    var onUndo: ((Int, String) -> Void)?

    @State private var note = ""
    @State private var submissionKey = UUID().uuidString
    @State private var submissionStarted = false
    @State private var recentAmounts: [Int] = []
    @State private var durationSeconds: Int?
    @State private var didInitialize = false
    @State private var title = ""
    @State private var rating: Int = 0
    @State private var selectedIndicators: [String] = []
    @State private var indicatorVolumes: [String: Int] = [:]
    @State private var volumeML: Int? = nil
    @State private var selectedSubject: String? = nil
    @State private var selectedUserId: Int?
    @State private var whenDate: Date = TestHooks.reviewDate ?? Date()
    @State private var isSaving = false
    @State private var errorMessage: String?
    @State private var followUpDays: Int = 0
    @State private var followUpHours: Int = 0
    @State private var followUpMins: Int = 0

    private var isEditing: Bool { log != nil }
    private var volumeUnit: String { state.volumeUnit == "oz" ? "oz" : "ml" }
    private var isVolume: Bool { chore.hasVolumeML && ["", "ml", "oz"].contains(chore.metricUnit.lowercased()) }
    private var amountLabel: String { isVolume ? "Volume" : "Amount (\(chore.metricUnit))" }

    var body: some View {
        NavigationStack {
            Form {
                Group {
                if hasWhenPicker {
                    Section {
                        DatePicker("When", selection: $whenDate)
                            .datePickerStyle(.compact)
                            .accessibilityIdentifier("when-picker")
                    }
                }

                if !chore.subjects.isEmpty {
                    Section("Who") {
                        subjectChips
                    }
                }

                if chore.hasVolumeML && !recentVolumeValues.isEmpty {
                    Section("Recent") {
                        recentVolumeChips
                    }
                }

                if hasIndicators && chore.hasVolumeML {
                    Section("Type") {
                        ForEach(chore.indicatorLabels, id: \.self) { label in
                            let isOn = selectedIndicators.contains(label)
                            HStack(spacing: 8) {
                                Button {
                                    toggleIndicator(label)
                                } label: {
                                    Text(label)
                                        .font(.subheadline)
                                        .padding(.horizontal, 14)
                                        .padding(.vertical, 8)
                                        .frame(minHeight: 36)
                                        .background(isOn ? Color.accentColor : DesignColors.surfaceSecondary)
                                        .foregroundColor(isOn ? .white : .primary)
                                        .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)

                                if isOn {
                                    volumePicker(selection: Binding(
                                        get: { indicatorVolumes[label] ?? nil as Int? },
                                        set: { indicatorVolumes[label] = $0 }
                                    ))
                                    .frame(maxWidth: 140)
                                } else {
                                    Spacer().frame(width: 140)
                                }
                            }
                        }
                    }
                } else if hasIndicators {
                    Section("How did it go?") {
                        chipGrid
                    }
                }

                // Volume-only chores (no indicators, but hasVolumeML)
                if chore.hasVolumeML && !hasIndicators {
                    Section(amountLabel) {
                        volumePicker(selection: $volumeML)
                    }
                }

                if chore.metricType == "duration" {
                    Section("Duration (seconds)") {
                        TextField("Optional", value: $durationSeconds, format: .number)
                            .keyboardType(.numberPad)
                            .accessibilityIdentifier("duration-seconds")
                    }
                }
                if !isEditing && chore.metricType == "duration" {
                    Section {
                        Button {
                            startTimer()
                        } label: {
                            Label("Start timer", systemImage: "play.fill")
                                .frame(maxWidth: .infinity)
                                .fontWeight(.semibold)
                        }
                        .accessibilityIdentifier("start-timer-button")
                    } footer: {
                        Text("Records elapsed time. Stop from the timer chip up top.")
                    }
                }

                if chore.hasRating {
                    Section("Title") {
                        TextField("Enter a title...", text: $title)
                    }
                    Section("Rating") {
                        HStack {
                            StarRatingView(rating: $rating)
                            Spacer()
                            if rating > 0 {
                                Button("clear") { rating = 0 }
                                    .font(.caption)
                                    .foregroundColor(.secondary)
                                    .buttonStyle(.plain)
                            }
                        }
                    }
                }

                if state.members.count > 1 {
                    Section("Done by") {
                        Picker("Done by", selection: Binding(
                            get: { selectedUserId ?? state.user?.id },
                            set: { selectedUserId = $0 }
                        )) {
                            ForEach(state.members) { member in
                                Text(member.displayName.isEmpty ? member.email : member.displayName)
                                    .tag(Optional(member.userId))
                            }
                        }
                        .pickerStyle(.menu)
                    }
                }

                if !isEditing && chore.followUpEnabled {
                    Section("Follow-up in") {
                        HStack(spacing: 6) {
                            Picker("Days", selection: $followUpDays) {
                                ForEach(0..<15, id: \.self) { d in Text("\(d)d").tag(d) }
                            }
                            .pickerStyle(.menu)
                            Picker("Hours", selection: $followUpHours) {
                                ForEach(0..<24, id: \.self) { h in Text("\(h)h").tag(h) }
                            }
                            .pickerStyle(.menu)
                            Picker("Mins", selection: $followUpMins) {
                                ForEach([0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55], id: \.self) { m in
                                    Text("\(m)m").tag(m)
                                }
                            }
                            .pickerStyle(.menu)
                        }
                    }
                }

                Section("Note") {
                    TextField("Add a note...", text: $note, axis: .vertical)
                        .lineLimit(2...4)
                }

                }
                .disabled(isSaving || submissionStarted)
                Section {
                    if submissionStarted {
                        Text("This saved request is fixed for safe retry. You can retry it here or manage it in Pending saves.")
                            .font(.caption)
                    }
                    if let error = errorMessage {
                        Text(error)
                            .foregroundColor(.red)
                            .font(.subheadline)
                    }

                    Button {
                        saveLog()
                    } label: {
                        if isSaving {
                            ProgressView()
                                .frame(maxWidth: .infinity)
                        } else {
                            Text(isEditing ? "Update" : (submissionStarted ? "Retry save" : "Log"))
                                .frame(maxWidth: .infinity)
                                .fontWeight(.semibold)
                        }
                    }
                    .disabled(isSaving)
                    .accessibilityIdentifier("save-log-button")
                }

                if isEditing, let logId = log?.id {
                    Section {
                        Button("Remove log", role: .destructive) {
                            dismiss()
                            onUndo?(logId, chore.name)
                        }
                    }
                }
            }
            .navigationTitle("\(chore.icon) \(chore.name)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
        .onAppear {
            if !didInitialize { setupFromLog(); didInitialize = true }
        }
        .task {
            guard !isEditing, chore.hasVolumeML else { return }
            recentAmounts = state.recentAmounts[chore.id] ?? []
            if let amounts = await logStore.loadRecentAmounts(choreId: chore.id, state: state) {
                recentAmounts = amounts
            }
        }
    }

    private var hasWhenPicker: Bool { true }
    private var hasIndicators: Bool { !chore.indicatorLabels.isEmpty }

    private var recentVolumeValues: [Int] { isEditing ? [] : recentAmounts }

    @ViewBuilder
    private func volumePicker(selection: Binding<Int?>) -> some View {
        if isVolume {
            let options = VolumeUnits.volumeOptions(unit: volumeUnit, selectedML: selection.wrappedValue)
            Picker("Volume", selection: selection) {
                Text("--").tag(Int?.none)
                ForEach(options, id: \.ml) { option in
                    Text(option.label).tag(Optional(option.ml))
                }
            }
            .pickerStyle(.menu)
            .accessibilityIdentifier("volume-picker")
        } else {
            TextField(amountLabel, value: selection, format: .number)
                .keyboardType(.numberPad)
                .accessibilityLabel(amountLabel)
                .accessibilityIdentifier("amount-input")
        }
    }

    private var subjectChips: some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 80), spacing: 8)], spacing: 8) {
            ForEach(chore.subjects, id: \.self) { subject in
                let isOn = selectedSubject == subject
                Button {
                    // Single-select: tapping the active chip deselects it.
                    selectedSubject = isOn ? nil : subject
                } label: {
                    Text(subject)
                        .font(.subheadline)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(isOn ? Color.accentColor : DesignColors.surfaceSecondary)
                        .foregroundColor(isOn ? .white : .primary)
                        .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var recentVolumeChips: some View {
        HStack(spacing: 8) {
            ForEach(recentVolumeValues, id: \.self) { ml in
                Button {
                    applyRecentVolume(ml)
                } label: {
                    Text(isVolume ? VolumeUnits.formatVolume(ml, unit: volumeUnit) : "\(ml) \(chore.metricUnit)")
                        .font(.subheadline)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(DesignColors.surfaceSecondary)
                        .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
    }

    /// Fills the volume input(s) with a recent amount without changing the
    /// selected indicator types — PWA `set-recent-volume` behavior.
    private func applyRecentVolume(_ ml: Int) {
        let selection = applyRecentVolumeSelection(
            ml,
            hasIndicators: hasIndicators,
            selectedIndicators: selectedIndicators,
            indicatorVolumes: indicatorVolumes,
            volumeML: volumeML
        )
        indicatorVolumes = selection.indicatorVolumes
        volumeML = selection.volumeML
    }

    private func startTimer() {
        guard let origin = logStore.api.identity.snapshot.origin else { return }
        guard state.activeTimer == nil else { errorMessage = "Finish the existing timer first."; return }
        let timer = ActiveTimer(
            choreId: chore.id, choreName: chore.name,
            choreIcon: chore.icon, startedAt: Date(), origin: origin
        )
        guard DurationTimer.save(timer) else { errorMessage = APIError.saveNotDurable.errorDescription; return }
        state.activeTimer = timer
        dismiss()
    }

    private var chipGrid: some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 80), spacing: 8)], spacing: 8) {
            ForEach(chore.indicatorLabels, id: \.self) { label in
                Button {
                    toggleIndicator(label)
                } label: {
                    Text(label)
                        .font(.subheadline)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(selectedIndicators.contains(label) ? Color.accentColor : DesignColors.surfaceSecondary)
                        .foregroundColor(selectedIndicators.contains(label) ? .white : .primary)
                        .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func toggleIndicator(_ label: String) {
        if let idx = selectedIndicators.firstIndex(of: label) {
            selectedIndicators.remove(at: idx)
            indicatorVolumes.removeValue(forKey: label)
        } else {
            selectedIndicators.append(label)
        }
    }

    private func setupFromLog() {
        if let log = log {
            note = log.note
            title = log.title ?? ""
            rating = log.rating ?? 0
            selectedIndicators = log.indicators
            indicatorVolumes = log.indicatorVolumes ?? [:]
            volumeML = log.volumeML
            durationSeconds = log.durationSeconds
            selectedSubject = log.subject
            selectedUserId = log.userId
            whenDate = log.completedAt
        } else {
            // Echo the latest log's own indicator selection (type) so the
            // sheet matches what the user last selected — PWA parity. Only
            // volume-metric indicator chores (Feed Baby pattern) get this;
            // plain chip chores (e.g. Laundry) always start from their chore
            // defaults. Volumes are only rendered/submitted for selected
            // indicators, so stray cached volumes for unselected types never
            // appear.
            if chore.hasVolumeML && hasIndicators, let latestLog = state.latestLogs[chore.id] {
                selectedIndicators = latestLog.indicators
                volumeML = latestLog.volumeML
                // Only keep volumes for the types actually selected in the
                // previous log; a cached volume for any other label must
                // never surface when its chip is toggled on.
                indicatorVolumes = (latestLog.indicatorVolumes ?? [:]).filter {
                    selectedIndicators.contains($0.key)
                }
            } else {
                selectedIndicators = chore.indicatorDefaults
            }
            let totalMins = chore.lastFollowUpMinutes
            followUpDays = totalMins / 1440
            followUpHours = (totalMins % 1440) / 60
            followUpMins = ((totalMins % 60) / 5) * 5
        }
    }

    private func saveLog() {
        guard !isSaving else { return }
        let amounts = Array(indicatorVolumes.values) + [volumeML].compactMap { $0 }
        guard amounts.allSatisfy({ (0...100000).contains($0) }), durationSeconds.map({ (0...86400).contains($0) }) ?? true else {
            errorMessage = "Amount must be 0–100000; duration must be 0–86400 seconds."
            return
        }
        let owner = state.revision
        isSaving = true
        errorMessage = nil

        let isoFormatter = ISO8601DateFormatter()
        let dateFormatter = DateFormatter()
        dateFormatter.locale = Locale(identifier: "en_US_POSIX")
        dateFormatter.calendar = Calendar(identifier: .gregorian)
        dateFormatter.dateFormat = "yyyy-MM-dd"

        let completedAtISO = isoFormatter.string(from: whenDate)
        let dateStr = dateFormatter.string(from: whenDate)
        let hour = Calendar.current.component(.hour, from: whenDate)

        // For chores with both indicators and volume: require at least one
        // indicator selected with a volume set.
        if chore.hasVolumeML && hasIndicators {
            let activeVolumes = indicatorVolumes.filter { k, _ in
                selectedIndicators.contains(k)
            }.compactMapValues { $0 }
            if activeVolumes.isEmpty || selectedIndicators.isEmpty {
                errorMessage = "Select an amount and type"
                isSaving = false
                return
            }
        }

        // Only send volumes for selected indicators
        let activeVolumes: [String: Int] = indicatorVolumes.filter { k, _ in
            selectedIndicators.contains(k)
        }

        let trimmedTitle = title.trimmingCharacters(in: .whitespaces)

        if !isEditing { submissionStarted = true }
        Task {
            do {
                if let logId = log?.id {
                    let changedWhen = whenDate != log?.completedAt
                    let _ = try await logStore.updateLog(
                        logId: logId, note: note, indicators: selectedIndicators,
                        volumeML: chore.hasVolumeML ? .some(volumeML) : nil, userId: selectedUserId,
                        completedAt: changedWhen ? completedAtISO : nil, hour: changedWhen ? hour : nil,
                        date: changedWhen ? dateStr : nil,
                        indicatorVolumes: chore.hasVolumeML && hasIndicators ? activeVolumes : nil,
                        rating: chore.hasRating ? .some(rating > 0 ? rating : nil) : nil,
                        title: chore.hasRating ? .some(trimmedTitle.isEmpty ? nil : trimmedTitle) : nil,
                        durationSeconds: chore.metricType == "duration" ? .some(durationSeconds) : nil,
                        subject: chore.subjects.isEmpty ? nil : .some(selectedSubject)
                    )
                    guard state.revision == owner else { return }
                    let loader = LogDataLoader(api: logStore.api, state: state)
                    await loader.loadTodayData()
                    guard state.revision == owner else { return }
                    await loader.loadLatestLogsData()
                } else {
                    let followUpMinutes = followUpDays * 1440 + followUpHours * 60 + followUpMins
                    let followUpTime: String? = {
                        if followUpMinutes > 0 {
                            let fu = whenDate.addingTimeInterval(TimeInterval(followUpMinutes * 60))
                            let f = DateFormatter()
                            f.dateFormat = "yyyy-MM-dd'T'HH:mm"
                            return f.string(from: fu)
                        }
                        return nil
                    }()
                    let outcome = try await logStore.createLog(
                        choreId: chore.id, note: note, date: dateStr,
                        indicators: selectedIndicators, slotHour: hour,
                        completedAt: completedAtISO, volumeML: volumeML,
                        userId: selectedUserId,
                        indicatorVolumes: activeVolumes.isEmpty ? nil : activeVolumes,
                        followUpMinutes: followUpMinutes > 0 ? followUpMinutes : nil,
                        followUpTime: followUpTime,
                        rating: rating > 0 ? rating : nil,
                        title: trimmedTitle.isEmpty ? nil : trimmedTitle,
                        durationSeconds: durationSeconds, subject: selectedSubject,
                        idempotencyKey: submissionKey
                    )
                    guard state.revision == owner else { return }
                    switch outcome {
                    case .created(let response):
                        state.todayLogs.insert(response.log, at: 0)
                        state.latestLogs[chore.id] = response.log
                    case .queued(let pending):
                        var row = pending
                        if row.userId == nil { row.userId = state.user?.id }
                        state.pendingLogs.insert(row, at: 0)
                    }
                }
                guard state.revision == owner else { return }
                dismiss()
            } catch {
                guard state.revision == owner else { return }
                errorMessage = error.localizedDescription
                isSaving = false
                refreshChores()
            }
        }
    }

    private func refreshChores() {
        let owner = state.revision
        Task {
            do {
                let data: ChoresResponse = try await logStore.api.get("/api/chores")
                guard state.revision == owner else { return }
                state.chores = data.chores
            } catch {}
        }
    }
}

// MARK: - Star rating

/// 0–50 rating ("tenths of stars", half-star resolution) matching the PWA's
/// star-rating slider. Tap or drag across the stars to set the value.
struct StarRatingView: View {
    @Binding var rating: Int

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Text("☆☆☆☆☆")
                    .foregroundColor(.secondary)
                Text("★★★★★")
                    .foregroundColor(.yellow)
                    .mask(alignment: .leading) {
                        Rectangle().frame(width: geo.size.width * CGFloat(rating) / 50)
                    }
            }
            .font(.title2)
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { value in
                        let pct = min(max(value.location.x / geo.size.width, 0), 1)
                        // Round to the nearest half star (multiple of 5).
                        rating = Int((pct * 50 / 5).rounded()) * 5
                    }
            )
        }
        .frame(width: 140, height: 30)
        .accessibilityElement()
        .accessibilityLabel("Rating")
        .accessibilityValue(String(format: "%.1f stars", Double(rating) / 10))
    }
}
