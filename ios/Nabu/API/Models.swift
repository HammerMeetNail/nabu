import Foundation

// MARK: - LocalDate

struct LocalDate: Codable, Hashable, Equatable {
    let value: String

    init(value: String) {
        self.value = value
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        value = try container.decode(String.self)
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(value)
    }
}

// MARK: - JSON Coding

let apiDecoder: JSONDecoder = {
    let d = JSONDecoder()
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    d.dateDecodingStrategy = .custom { decoder in
        let container = try decoder.singleValueContainer()
        let string = try container.decode(String.self)
        if let date = formatter.date(from: string) {
            return date
        }
        // Fallback: try without fractional seconds
        let basicFormatter = ISO8601DateFormatter()
        basicFormatter.formatOptions = [.withInternetDateTime]
        if let date = basicFormatter.date(from: string) {
            return date
        }
        throw DecodingError.dataCorruptedError(
            in: container,
            debugDescription: "Expected ISO8601 date, got: \(string)"
        )
    }
    d.keyDecodingStrategy = .convertFromSnakeCase
    return d
}()

let apiEncoder: JSONEncoder = {
    let e = JSONEncoder()
    e.dateEncodingStrategy = .iso8601
    // NOTE: Do NOT use .convertToSnakeCase — the Go server uses camelCase JSON tags.
    e.keyEncodingStrategy = .useDefaultKeys
    return e
}()

// MARK: - User

struct User: Codable, Identifiable, Equatable {
    let id: Int
    let householdId: Int?
    let email: String
    let displayName: String
    let avatarColor: String
    let emailVerified: Bool
    let role: String
    let createdAt: Date
    var hasPassword: Bool? = nil
}

// MARK: - Household

struct Household: Codable, Identifiable, Equatable {
    let id: Int
    let name: String
    let initials: String
    let inviteCode: String?
    let createdAt: Date
}

struct HouseholdWithRole: Codable, Identifiable, Equatable {
    let id: Int
    let name: String
    let initials: String
    let role: String
}

// MARK: - Member

struct Member: Codable, Identifiable, Equatable {
    let userId: Int
    let email: String
    let displayName: String
    let avatarColor: String
    let emailVerified: Bool
    let role: String

    var id: Int { userId }
}

struct HistoricalMember: Codable, Identifiable, Equatable {
    let userId: Int
    let displayName: String
    let avatarColor: String

    var id: Int { userId }
}

// MARK: - Invite

struct Invite: Codable, Identifiable, Equatable {
    let id: Int
    let householdId: Int
    let code: String
    let createdBy: Int
    let maxUses: Int
    let usedCount: Int
    let expiresAt: Date?
    let createdAt: Date
}

// MARK: - Chore

struct Chore: Codable, Identifiable, Equatable {
    let id: Int
    let householdId: Int
    let name: String
    let icon: String
    let color: String
    let sortOrder: Int
    let category: String
    let isPredefined: Bool
    let predefinedKey: String?
    let createdBy: Int?
    let createdAt: Date
    let indicatorLabels: [String]
    let indicatorDefaults: [String]
    let hasVolumeML: Bool
    let followUpEnabled: Bool
    let lastFollowUpMinutes: Int
    let hasRating: Bool
    /// Generalized metric config (migration 036): "none" | "amount" | "rating" | "duration".
    /// Source of truth going forward; hasVolumeML/hasRating are kept in sync server-side.
    let metricType: String
    /// Display unit label for "amount" metrics (e.g. "mL", "oz", "g", "min").
    let metricUnit: String
    /// Optional subject tags (migration 039), e.g. twin names for Feed Baby.
    let subjects: [String]
    /// Visibility: "household" (default) or "admins" (private household tasks, migration 042).
    let visibility: String

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(Int.self, forKey: .id)
        householdId = try container.decode(Int.self, forKey: .householdId)
        name = try container.decode(String.self, forKey: .name)
        icon = try container.decode(String.self, forKey: .icon)
        color = try container.decode(String.self, forKey: .color)
        sortOrder = try container.decode(Int.self, forKey: .sortOrder)
        category = try container.decode(String.self, forKey: .category)
        isPredefined = try container.decode(Bool.self, forKey: .isPredefined)
        predefinedKey = try container.decodeIfPresent(String.self, forKey: .predefinedKey)
        createdBy = try container.decodeIfPresent(Int.self, forKey: .createdBy)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        indicatorLabels = try container.decodeIfPresent([String].self, forKey: .indicatorLabels) ?? []
        indicatorDefaults = try container.decodeIfPresent([String].self, forKey: .indicatorDefaults) ?? []
        hasVolumeML = try container.decode(Bool.self, forKey: .hasVolumeML)
        followUpEnabled = try container.decodeIfPresent(Bool.self, forKey: .followUpEnabled) ?? true
        lastFollowUpMinutes = try container.decodeIfPresent(Int.self, forKey: .lastFollowUpMinutes) ?? 0
        hasRating = try container.decodeIfPresent(Bool.self, forKey: .hasRating) ?? false
        metricType = try container.decodeIfPresent(String.self, forKey: .metricType) ?? "none"
        metricUnit = try container.decodeIfPresent(String.self, forKey: .metricUnit) ?? ""
        subjects = try container.decodeIfPresent([String].self, forKey: .subjects) ?? []
        visibility = try container.decodeIfPresent(String.self, forKey: .visibility) ?? "household"
    }

    init(id: Int, householdId: Int, name: String, icon: String, color: String, sortOrder: Int, category: String, isPredefined: Bool, predefinedKey: String?, createdBy: Int?, createdAt: Date, indicatorLabels: [String], indicatorDefaults: [String], hasVolumeML: Bool, hasRating: Bool = false, metricType: String = "none", metricUnit: String = "", subjects: [String] = [], visibility: String = "household") {
        self.id = id
        self.householdId = householdId
        self.name = name
        self.icon = icon
        self.color = color
        self.sortOrder = sortOrder
        self.category = category
        self.isPredefined = isPredefined
        self.predefinedKey = predefinedKey
        self.createdBy = createdBy
        self.createdAt = createdAt
        self.indicatorLabels = indicatorLabels
        self.indicatorDefaults = indicatorDefaults
        self.hasVolumeML = hasVolumeML
        self.followUpEnabled = true
        self.lastFollowUpMinutes = 0
        self.hasRating = hasRating
        self.metricType = metricType
        self.metricUnit = metricUnit
        self.subjects = subjects
        self.visibility = visibility
    }

    enum CodingKeys: String, CodingKey {
        case id, householdId, name, icon, color, sortOrder, category
        case isPredefined, predefinedKey, createdBy, createdAt
        case indicatorLabels, indicatorDefaults, hasVolumeML
        case followUpEnabled, lastFollowUpMinutes
        case hasRating, metricType, metricUnit, subjects, visibility
    }
}

// MARK: - ChoreLog

struct ChoreLog: Codable, Identifiable, Equatable {
    let metricUnit: String?
    let id: Int
    let householdId: Int
    let userId: Int
    let choreId: Int
    let completedAt: Date
    let note: String
    let indicators: [String]
    let slotHour: Int?
    let createdAt: Date
    let volumeML: Int?
    let indicatorVolumes: [String: Int]?
    let title: String?
    let rating: Int?
    /// Elapsed seconds for duration-metric chores (migration 036). nil = no duration.
    let durationSeconds: Int?
    /// Subject tag (migration 039), e.g. which twin this log is about. nil = untagged.
    let subject: String?

    init(id: Int, householdId: Int, userId: Int, choreId: Int, completedAt: Date, note: String, indicators: [String], slotHour: Int?, createdAt: Date, volumeML: Int?, indicatorVolumes: [String: Int]?, title: String? = nil, rating: Int? = nil, durationSeconds: Int? = nil, subject: String? = nil, metricUnit: String? = nil) {
        self.metricUnit = metricUnit
        self.id = id
        self.householdId = householdId
        self.userId = userId
        self.choreId = choreId
        self.completedAt = completedAt
        self.note = note
        self.indicators = indicators
        self.slotHour = slotHour
        self.createdAt = createdAt
        self.volumeML = volumeML
        self.indicatorVolumes = indicatorVolumes
        self.title = title
        self.rating = rating
        self.durationSeconds = durationSeconds
        self.subject = subject
    }
}

// MARK: - DailySummary

struct DailySummary: Codable, Equatable {
    let date: String
    let totalChores: Int
    let choresDone: Int
    let byUser: [String: Int]
    let byCategory: [String: Int]
}

// MARK: - ChoreSchedule

struct ChoreSchedule: Codable, Identifiable, Equatable {
    let id: Int
    let householdId: Int
    let choreId: Int
    let frequencyType: String
    let timePeriod: String
    let specificTime: String?
    let timesOfDay: [String]
    let daysOfWeek: [Int]
    let intervalDays: Int
    let dayOfMonth: Int
    let monthWeekday: MonthWeekday?
    let monthOfYear: Int
    let recurrenceEnd: Date?
    let startDate: String?
    let targetCount: Int
    let isActive: Bool
    let isFollowUp: Bool
    let assignedUserId: Int?
    let createdAt: Date
    let updatedAt: Date

    init(id: Int, householdId: Int, choreId: Int, frequencyType: String, timePeriod: String, specificTime: String?, timesOfDay: [String], daysOfWeek: [Int], intervalDays: Int, dayOfMonth: Int, monthWeekday: MonthWeekday?, monthOfYear: Int, recurrenceEnd: Date?, startDate: String?, targetCount: Int, isActive: Bool, isFollowUp: Bool, assignedUserId: Int?, createdAt: Date, updatedAt: Date) {
        self.id = id
        self.householdId = householdId
        self.choreId = choreId
        self.frequencyType = frequencyType
        self.timePeriod = timePeriod
        self.specificTime = specificTime
        self.timesOfDay = timesOfDay
        self.daysOfWeek = daysOfWeek
        self.intervalDays = intervalDays
        self.dayOfMonth = dayOfMonth
        self.monthWeekday = monthWeekday
        self.monthOfYear = monthOfYear
        self.recurrenceEnd = recurrenceEnd
        self.startDate = startDate
        self.targetCount = targetCount
        self.isActive = isActive
        self.isFollowUp = isFollowUp
        self.assignedUserId = assignedUserId
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(Int.self, forKey: .id)
        householdId = try container.decode(Int.self, forKey: .householdId)
        choreId = try container.decode(Int.self, forKey: .choreId)
        frequencyType = try container.decode(String.self, forKey: .frequencyType)
        timePeriod = try container.decode(String.self, forKey: .timePeriod)
        specificTime = try container.decodeIfPresent(String.self, forKey: .specificTime)
        timesOfDay = try container.decodeIfPresent([String].self, forKey: .timesOfDay) ?? []
        daysOfWeek = try container.decodeIfPresent([Int].self, forKey: .daysOfWeek) ?? []
        intervalDays = try container.decode(Int.self, forKey: .intervalDays)
        dayOfMonth = try container.decodeIfPresent(Int.self, forKey: .dayOfMonth) ?? 0
        monthWeekday = try container.decodeIfPresent(MonthWeekday.self, forKey: .monthWeekday)
        monthOfYear = try container.decodeIfPresent(Int.self, forKey: .monthOfYear) ?? 0
        recurrenceEnd = try container.decodeIfPresent(Date.self, forKey: .recurrenceEnd)
        startDate = try container.decodeIfPresent(String.self, forKey: .startDate)
        targetCount = try container.decode(Int.self, forKey: .targetCount)
        isActive = try container.decode(Bool.self, forKey: .isActive)
        isFollowUp = try container.decode(Bool.self, forKey: .isFollowUp)
        assignedUserId = try container.decodeIfPresent(Int.self, forKey: .assignedUserId)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
    }

    enum CodingKeys: String, CodingKey {
        case id, householdId, choreId, frequencyType, timePeriod
        case specificTime, timesOfDay, daysOfWeek, intervalDays
        case dayOfMonth, monthWeekday, monthOfYear
        case recurrenceEnd, startDate, targetCount
        case isActive, isFollowUp, assignedUserId
        case createdAt, updatedAt
    }
}

struct MonthWeekday: Codable, Equatable {
    let week: Int
    let day: Int
}

// MARK: - Notifications

struct AppNotification: Codable, Identifiable, Equatable {
    let id: Int
    let userId: Int
    let type: String
    let title: String
    let body: String
    let isRead: Bool
    let createdAt: Date
}

struct NotificationTypeInfo: Codable, Identifiable, Equatable {
    let type: String
    let label: String
    let description: String

    var id: String { type }
}

// MARK: - Preferences

struct ReminderPreference: Codable, Equatable {
    let userId: Int
    let pushEnabled: Bool
    let emailEnabled: Bool
    let quietHoursStart: String
    let quietHoursEnd: String
    let timezone: String
    let enabledPushTypes: [String]
    let defaultReminderLeadMinutes: Int
}

struct ChoreReminderPref: Codable, Equatable {
    let userId: Int
    let choreId: Int
    let enabled: Bool
    let leadMinutes: Int
}

struct ChoreReminderPrefsResponse: Codable {
    let prefs: [ChoreReminderPref]
}

struct ChoreReminderPrefResponse: Codable {
    let pref: ChoreReminderPref
}

struct UserPreferences: Codable, Equatable {
    let choreOrder: [Int]
    let hiddenHomeChoreIds: [Int]
    let timezone: String
    /// "ml" (canonical, default) or "oz". Volumes are stored in mL; this is display-only.
    let volumeUnit: String
    let statsSectionOrder: [String]
    let statsSectionHidden: [String]
    let statsWidgets: [StatsWidget]
    /// Hides the in-app unread notifications badge; notifications still
    /// accumulate. Server default is false.
    let hideNotificationBadge: Bool

    init(choreOrder: [Int], hiddenHomeChoreIds: [Int], timezone: String, volumeUnit: String = "ml", statsSectionOrder: [String] = [], statsSectionHidden: [String] = [], statsWidgets: [StatsWidget] = [], hideNotificationBadge: Bool = false) {
        self.choreOrder = choreOrder
        self.hiddenHomeChoreIds = hiddenHomeChoreIds
        self.timezone = timezone
        self.volumeUnit = volumeUnit
        self.statsSectionOrder = statsSectionOrder
        self.statsSectionHidden = statsSectionHidden
        self.statsWidgets = statsWidgets
        self.hideNotificationBadge = hideNotificationBadge
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        choreOrder = try container.decodeIfPresent([Int].self, forKey: .choreOrder) ?? []
        hiddenHomeChoreIds = try container.decodeIfPresent([Int].self, forKey: .hiddenHomeChoreIds) ?? []
        timezone = try container.decodeIfPresent(String.self, forKey: .timezone) ?? ""
        volumeUnit = try container.decodeIfPresent(String.self, forKey: .volumeUnit) ?? "ml"
        statsSectionOrder = try container.decodeIfPresent([String].self, forKey: .statsSectionOrder) ?? []
        statsSectionHidden = try container.decodeIfPresent([String].self, forKey: .statsSectionHidden) ?? []
        statsWidgets = try container.decodeIfPresent([StatsWidget].self, forKey: .statsWidgets) ?? []
        hideNotificationBadge = try container.decodeIfPresent(Bool.self, forKey: .hideNotificationBadge) ?? false
    }

    enum CodingKeys: String, CodingKey {
        case choreOrder, hiddenHomeChoreIds, timezone, volumeUnit
        case statsSectionOrder, statsSectionHidden, statsWidgets
        case hideNotificationBadge
    }
}

/// A user-defined stats widget (migration 037). Server-validated closed schema:
/// every field is an enum/allowlist server-side; `title` is data — render it
/// only as plain Text, never as attributed/markdown content.
struct StatsWidget: Codable, Identifiable, Equatable {
    let id: String
    let type: String     // timeseries | total | last-done | interval | member-split | top-list
    let choreIds: [Int]
    let metric: String   // count | amount | rating | duration
    let agg: String      // sum | avg | min | max
    let period: String   // day | week | month | all
    let grain: String    // daily | weekly | monthly
    let title: String

    init(id: String, type: String, choreIds: [Int], metric: String, agg: String, period: String, grain: String, title: String) {
        self.id = id
        self.type = type
        self.choreIds = choreIds
        self.metric = metric
        self.agg = agg
        self.period = period
        self.grain = grain
        self.title = title
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        type = try container.decode(String.self, forKey: .type)
        choreIds = try container.decodeIfPresent([Int].self, forKey: .choreIds) ?? []
        metric = try container.decodeIfPresent(String.self, forKey: .metric) ?? ""
        agg = try container.decodeIfPresent(String.self, forKey: .agg) ?? ""
        period = try container.decodeIfPresent(String.self, forKey: .period) ?? ""
        grain = try container.decodeIfPresent(String.self, forKey: .grain) ?? ""
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
    }
}

// MARK: - Day Notes

/// One shared free-text diary note per household+date (migration 038).
struct DayNote: Codable, Equatable {
    let date: String // YYYY-MM-DD
    let note: String
    let updatedBy: Int?
    let updatedAt: Date
}

// MARK: - Stats DTOs

struct LeaderboardEntry: Codable, Equatable {
    let userId: Int
    let count: Int
}

struct Streaks: Codable, Equatable {
    let current: Int
    let longest: Int
}

struct HeatmapEntry: Codable, Equatable {
    let date: String
    let count: Int
}

struct BreakdownEntry: Codable, Equatable {
    let category: String
    let count: Int
}

struct RecapTopPerformer: Codable, Equatable {
    let userId: Int
    let count: Int
}

struct Recap: Codable, Equatable {
    let totalChores: Int
    let topPerformer: RecapTopPerformer?
    let mostActiveDay: String
    let byCategory: [BreakdownEntry]
}

struct StatsOverview: Codable, Equatable {
    let leaderboard: [LeaderboardEntry]
    let streaks: Streaks
    let breakdown: [BreakdownEntry]
    let recap: Recap
}

struct BusyHour: Codable, Equatable {
    let hour: Int
    let count: Int
}

struct TopChore: Codable, Equatable {
    let choreId: Int
    let choreName: String
    let choreIcon: String
    let count: Int
}

struct ChoreStat: Codable, Equatable {
    let choreId: Int
    let choreName: String
    let choreIcon: String
    let totalThisWeek: Int
    let totalThisMonth: Int
    /// Count within the requested range/period (`?period=day|week|month`).
    /// The PWA's Chores section renders this; defaults 0 for old fixtures.
    let totalInRange: Int
    let indicatorCounts: [String: Int]?
    let volumeHistory: [VolumePoint]?
    let avgVolume: Double?
    let hasVolume: Bool
    let hasIndicators: Bool

    init(choreId: Int, choreName: String, choreIcon: String, totalThisWeek: Int, totalThisMonth: Int, totalInRange: Int = 0, indicatorCounts: [String: Int]? = nil, volumeHistory: [VolumePoint]? = nil, avgVolume: Double? = nil, hasVolume: Bool = false, hasIndicators: Bool = false) {
        self.choreId = choreId
        self.choreName = choreName
        self.choreIcon = choreIcon
        self.totalThisWeek = totalThisWeek
        self.totalThisMonth = totalThisMonth
        self.totalInRange = totalInRange
        self.indicatorCounts = indicatorCounts
        self.volumeHistory = volumeHistory
        self.avgVolume = avgVolume
        self.hasVolume = hasVolume
        self.hasIndicators = hasIndicators
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        choreId = try container.decode(Int.self, forKey: .choreId)
        choreName = try container.decode(String.self, forKey: .choreName)
        choreIcon = try container.decodeIfPresent(String.self, forKey: .choreIcon) ?? ""
        totalThisWeek = try container.decodeIfPresent(Int.self, forKey: .totalThisWeek) ?? 0
        totalThisMonth = try container.decodeIfPresent(Int.self, forKey: .totalThisMonth) ?? 0
        totalInRange = try container.decodeIfPresent(Int.self, forKey: .totalInRange) ?? 0
        indicatorCounts = try container.decodeIfPresent([String: Int].self, forKey: .indicatorCounts)
        volumeHistory = try container.decodeIfPresent([VolumePoint].self, forKey: .volumeHistory)
        avgVolume = try container.decodeIfPresent(Double.self, forKey: .avgVolume)
        hasVolume = try container.decodeIfPresent(Bool.self, forKey: .hasVolume) ?? false
        hasIndicators = try container.decodeIfPresent(Bool.self, forKey: .hasIndicators) ?? false
    }
}

struct VolumePoint: Codable, Equatable {
    let date: String
    let totalML: Int
}

struct TimeSeriesPeriod: Codable, Equatable {
    var amountsByUnit: [String: Int]? = nil
    let start: String
    let end: String
    let count: Int
    let totalML: Int?
    let totalDuration: Int?
    let indicators: [String: Int]?
    let volumeByIndicator: [String: Int]?
}

struct TimeSeriesByMember: Codable, Equatable {
    let userId: Int
    let count: Int
}

struct ChoreTimeSeries: Codable, Equatable {
    let choreId: Int
    let choreName: String
    let choreIcon: String
    let metricType: String?
    let metricUnit: String?
    let byMember: [TimeSeriesByMember]
    let periods: [TimeSeriesPeriod]
}

/// One inter-feed gap from `GET /api/stats/feeding-gaps` (cluster-feeding
/// analysis; `?choreId=` generalizes it to any chore).
struct FeedingGap: Codable, Equatable {
    let hour: Int
    let gapMinutes: Int
    let precedingVolume: Int
    let followUpVolume: Int
    let date: String
}

/// Period-scoped aggregate for one chore (`GET /api/stats/chores/{id}/summary`).
/// Backs the total / member-split widget types.
struct ChoreSummary: Codable, Equatable {
    var amountsByUnit: [String: Int]? = nil
    let choreId: Int
    let count: Int
    let totalML: Int
    let totalDuration: Int
    let byMember: [LeaderboardEntry]
    let metricType: String?
    let metricUnit: String?
}

// MARK: - Response Wrappers

struct UserResponse: Codable {
    let user: User?
}

struct HouseholdResponse: Codable {
    let household: Household
    let members: [Member]
    let historicalMembers: [HistoricalMember]
    let invites: [Invite]

    enum CodingKeys: String, CodingKey {
        case household, members, historicalMembers, invites
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        household = try container.decode(Household.self, forKey: .household)
        members = try container.decodeIfPresent([Member].self, forKey: .members) ?? []
        historicalMembers = try container.decodeIfPresent([HistoricalMember].self, forKey: .historicalMembers) ?? []
        invites = try container.decodeIfPresent([Invite].self, forKey: .invites) ?? []
    }
}

struct HouseholdOnlyResponse: Codable {
    let household: Household
}

struct HouseholdsResponse: Codable {
    let households: [HouseholdWithRole]
}

struct InvitesResponse: Codable {
    let invites: [Invite]
}

struct InviteResponse: Codable {
    let invite: Invite
}

struct ChoresResponse: Codable {
    let chores: [Chore]
}

struct ChoreResponse: Codable {
    let chore: Chore
}

struct DefaultsResponse: Codable {
    let defaults: [Chore]
}

struct LogResponse: Codable {
    let log: ChoreLog
}

struct LogsResponse: Codable {
    let logs: [ChoreLog]
}

struct TodayResponse: Codable {
    let logs: [ChoreLog]
    let summary: DailySummary
    let date: String
}

struct HistoryResponse: Codable {
    let logs: [ChoreLog]
    let hasMore: Bool
    // Absent on text-search responses (`?q=`), which are a flat, capped,
    // newest-first list rather than a paginated window.
    let start: String?
    let end: String?
}

struct LatestLogsResponse: Codable {
    let latestLogs: [String: ChoreLog]
}

struct SchedulesResponse: Codable {
    let schedules: [ChoreSchedule]
}

struct ScheduleResponse: Codable {
    let schedule: ChoreSchedule
}

struct ScheduleForDateResponse: Codable {
    let schedules: [ChoreSchedule]
    let date: String
}

struct NotificationsResponse: Codable {
    let notifications: [AppNotification]
    let unreadCount: Int
    var nextCursor: String? = nil
}

struct NotificationPrefsResponse: Codable {
    let preferences: ReminderPreference
    let availableTypes: [NotificationTypeInfo]
}

struct UserPreferencesResponse: Codable {
    let preferences: UserPreferences
}

struct StatusResponse: Codable {
    let status: String
}

struct LeaderboardResponse: Codable {
    let leaderboard: [LeaderboardEntry]
    let start: String?
    let end: String?
}

struct StreaksResponse: Codable {
    let streaks: Streaks
}

struct HeatmapResponse: Codable {
    let heatmap: [HeatmapEntry]
}

struct BreakdownResponse: Codable {
    let breakdown: [BreakdownEntry]
    let start: String
    let end: String
}

struct RecapResponse: Codable {
    let recap: Recap
}

struct OverviewResponse: Codable {
    let overview: StatsOverview
}

struct BusyHoursResponse: Codable {
    let busyHours: [BusyHour]
    let start: String
    let end: String
}

struct TopChoresResponse: Codable {
    let topChores: [TopChore]
}

struct ChoreStatsResponse: Codable {
    let choreStats: [ChoreStat]
    let start: String
    let end: String
}

struct SingleChoreStatsResponse: Codable {
    let choreStats: ChoreStat
}

struct TimeSeriesResponse: Codable {
    let timeSeries: ChoreTimeSeries
}

struct ChoreSummaryResponse: Codable {
    let summary: ChoreSummary
}

struct FeedingGapsResponse: Codable {
    let feedingGaps: [FeedingGap]
}

struct DayNotesResponse: Codable {
    let notes: [DayNote]
}

struct DayNoteResponse: Codable {
    let note: DayNote
}

struct RecentAmountsResponse: Codable { let amounts: [Int] }
