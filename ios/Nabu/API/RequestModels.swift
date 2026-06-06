import Foundation

// MARK: - Auth

struct RegisterRequest: Codable {
    let email: String
    let password: String
}

struct LoginRequest: Codable {
    let email: String
    let password: String
}

struct MagicLinkRequest: Codable {
    let email: String
}

struct ForgotPasswordRequest: Codable {
    let email: String
}

struct ResetPasswordRequest: Codable {
    let token: String
    let password: String
}

struct ChangePasswordRequest: Codable {
    let currentPassword: String
    let newPassword: String
}

// MARK: - Household

struct CreateHouseholdRequest: Codable {
    let name: String
    let initials: String
}

struct UpdateHouseholdRequest: Codable {
    let name: String
    let initials: String
}

struct JoinHouseholdRequest: Codable {
    let inviteCode: String
}

struct UpdateMemberRoleRequest: Codable {
    let role: String
}

struct TransferOwnershipRequest: Codable {
    let newOwnerId: Int
}

// MARK: - Chores

struct CreateChoreRequest: Codable {
    let name: String
    let icon: String?
    let color: String?
    let category: String?
    let indicatorLabels: [String]?
    let indicatorDefaults: [String]?

    enum CodingKeys: String, CodingKey {
        case name, icon, color, category
        case indicatorLabels, indicatorDefaults
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(name, forKey: .name)
        try container.encodeIfPresent(icon, forKey: .icon)
        try container.encodeIfPresent(color, forKey: .color)
        try container.encodeIfPresent(category, forKey: .category)
        try container.encodeIfPresent(indicatorLabels, forKey: .indicatorLabels)
        try container.encodeIfPresent(indicatorDefaults, forKey: .indicatorDefaults)
    }
}

struct ReorderChoresRequest: Codable {
    let choreIds: [Int]
}

// MARK: - Logs

struct CreateLogRequest: Codable {
    let choreId: Int
    let note: String?
    let indicators: [String]?
    let date: String?
    let hour: Int?
    let completedAt: String?
    let volumeML: Int?
    let userId: Int?

    enum CodingKeys: String, CodingKey {
        case choreId, note, indicators, date, hour, completedAt
        case volumeML = "volumeML"
        case userId
    }
}

struct UpdateLogRequest: Codable {
    let note: String?
    let indicators: [String]?
    let volumeML: Int?
    let userId: Int?
    let completedAt: String?
    let hour: Int?
    let date: String?

    enum CodingKeys: String, CodingKey {
        case note, indicators, date, hour, completedAt
        case volumeML = "volumeMl"
        case userId
    }
}

// MARK: - Schedules

struct CreateScheduleRequest: Codable {
    let choreId: Int
    let frequencyType: String?
    let timePeriod: String?
    let specificTime: String?
    let daysOfWeek: [Int]?
    let intervalDays: Int?
    let dayOfMonth: Int?
    let monthWeekday: MonthWeekday?
    let monthOfYear: Int?
    let startDate: String?
    let recurrenceEnd: String?
    let targetCount: Int?
    let isActive: Bool?
    let assignedUserId: Int?
}

struct PatchScheduleRequest: Codable {
    let choreId: Int?
    let timePeriod: String?
    let specificTime: String??
    let frequencyType: String?
    let isActive: Bool?
    let daysOfWeek: [Int]?
    let intervalDays: Int?
    let dayOfMonth: Int?
    let monthOfYear: Int?
    let startDate: String??
    let recurrenceEnd: String??

    enum CodingKeys: String, CodingKey {
        case choreId, timePeriod, specificTime, frequencyType
        case isActive, daysOfWeek, intervalDays, dayOfMonth
        case monthOfYear, startDate, recurrenceEnd
    }
}

// MARK: - Preferences

struct PatchNotificationPrefsRequest: Codable {
    let pushEnabled: Bool?
    let emailEnabled: Bool?
    let enabledPushTypes: [String]?
}

struct PatchUserPreferencesRequest: Codable {
    var choreOrder: [Int]? = nil
    var hiddenHomeChoreIds: [Int]? = nil
    var timezone: String? = nil
}

// MARK: - Push

struct PushSubscribeRequest: Codable {
    let subscription: PushSubscription
}

struct PushSubscription: Codable {
    let endpoint: String
    let keys: PushKeys
}

struct PushKeys: Codable {
    let p256dh: String
    let auth: String
}

struct PushUnsubscribeRequest: Codable {
    let endpoint: String
}

// MARK: - APNs

struct APNsRegisterRequest: Codable {
    let token: String
    let environment: String
    let bundleId: String
    let deviceName: String
}

struct APNsUnregisterRequest: Codable {
    let token: String
    let environment: String
}
