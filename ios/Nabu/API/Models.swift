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
    d.dateDecodingStrategy = .iso8601
    d.keyDecodingStrategy = .convertFromSnakeCase
    return d
}()

let apiEncoder: JSONEncoder = {
    let e = JSONEncoder()
    e.dateEncodingStrategy = .iso8601
    e.keyEncodingStrategy = .convertToSnakeCase
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
}

// MARK: - ChoreLog

struct ChoreLog: Codable, Identifiable, Equatable {
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
    let assignedUserId: Int?
    let createdAt: Date
    let updatedAt: Date
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
}

struct UserPreferences: Codable, Equatable {
    let choreOrder: [Int]
    let hiddenHomeChoreIds: [Int]
    let timezone: String
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
    let today: Int
    let thisWeek: Int
    let thisMonth: Int
}

struct ChoreStat: Codable, Equatable {
    let choreId: Int
    let choreName: String
    let choreIcon: String
    let totalThisWeek: Int
    let totalThisMonth: Int
    let indicatorCounts: [String: Int]?
    let volumeHistory: [VolumePoint]?
    let avgVolume: Double?
    let hasVolume: Bool
    let hasIndicators: Bool
}

struct VolumePoint: Codable, Equatable {
    let date: String
    let totalML: Int
}

struct TimeSeriesPeriod: Codable, Equatable {
    let start: String
    let end: String
    let count: Int
    let totalML: Int?
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
    let byMember: [TimeSeriesByMember]
    let periods: [TimeSeriesPeriod]
}

// MARK: - Response Wrappers

struct UserResponse: Codable {
    let user: User?
}

struct HouseholdResponse: Codable {
    let household: Household
    let members: [Member]
    let invites: [Invite]
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
    let start: String
    let end: String
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
    let start: String
    let end: String
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
