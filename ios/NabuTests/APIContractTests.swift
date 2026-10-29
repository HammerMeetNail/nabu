import XCTest
@testable import Nabu

final class APIContractTests: XCTestCase {
    let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        d.keyDecodingStrategy = .convertFromSnakeCase
        return d
    }()

    // MARK: - Error Decoding

    func testDecodeServerError() throws {
        let json = #"{"error":"Something went wrong"}"#.data(using: .utf8)!
        let error = try decoder.decode(APIErrorResponse.self, from: json)
        XCTAssertEqual(error.error, "Something went wrong")
    }

    func testDecodeErrorWithEmoji() throws {
        let json = #"{"error":"\u26a0\ufe0f Invalid chore name"}"#.data(using: .utf8)!
        let error = try decoder.decode(APIErrorResponse.self, from: json)
        XCTAssertTrue(error.error.contains("Invalid chore name"))
    }

    // MARK: - Retry-After

    func testRateLimitedErrorHasRetryAfter() {
        let error = APIError.rateLimited(retryAfter: "30")
        XCTAssertTrue(error.errorDescription?.contains("30") ?? false)
    }

    func testRateLimitedErrorWithoutRetryAfter() {
        let error = APIError.rateLimited(retryAfter: nil)
        XCTAssertTrue(error.errorDescription?.contains("try again") ?? false)
    }

    // MARK: - HTTP Status Codes

    func testHTTP401IsUnauthorized() {
        let error = APIError.httpError(statusCode: 401)
        XCTAssertTrue(error.errorDescription?.contains("401") ?? false)
    }

    func testServerErrorPreservesMessage() {
        let error = APIError.serverError(statusCode: 422, message: "Name is required")
        XCTAssertEqual(error.errorDescription, "Name is required")
    }

    func testAccessSuspendedErrorPreservesServerMessage() {
        let error = APIError.accessSuspended(message: "Your access to Sitter House is temporarily disabled. Contact an administrator to re-enable access.", household: "Sitter House")
        XCTAssertTrue(error.errorDescription?.contains("temporarily disabled") ?? false)
    }

    func testAccessSuspendedResponseFieldsDecode() throws {
        let json = #"{"error":"Your access to Sitter House is temporarily disabled.","status":"access_suspended","household":"Sitter House"}"#.data(using: .utf8)!
        let response = try JSONDecoder().decode(APIErrorResponse.self, from: json)
        XCTAssertEqual(response.status, "access_suspended")
        XCTAssertEqual(response.household, "Sitter House")
        XCTAssertTrue(response.error.contains("temporarily disabled"))
    }

    func testOrdinaryErrorResponseHasNoSuspendedStatus() throws {
        let json = #"{"error":"not authorized for this household"}"#.data(using: .utf8)!
        let response = try JSONDecoder().decode(APIErrorResponse.self, from: json)
        XCTAssertNil(response.status)
        XCTAssertNil(response.household)
    }

    // MARK: - Anti-Enumeration

    func testDuplicateRegistrationReturns200() {
        let error = APIError.serverError(statusCode: 409, message: "if this email is new, check your inbox")
        XCTAssertEqual(error.errorDescription, "if this email is new, check your inbox")
    }

    // MARK: - CSRF Token Provider

    func testCSRFTokenProviderReadsFromCookieStore() {
        let store = CookieStore()
        let provider = CSRFTokenProvider(cookieStore: store)
        XCTAssertNil(provider.token)
    }

    // MARK: - Cookie Store

    func testCookieStoreClearAll() {
        let store = CookieStore()
        store.clearAll()
        XCTAssertNil(store.sessionCookie)
        XCTAssertNil(store.csrfCookie)
    }

    // MARK: - Decoder Configuration

    func testDecoderConvertsSnakeCase() throws {
        let json = #"{"user_id": 1, "display_name": "Alice", "email_verified": true}"#.data(using: .utf8)!
        struct TestUser: Codable {
            let userId: Int
            let displayName: String
            let emailVerified: Bool
        }
        let user = try decoder.decode(TestUser.self, from: json)
        XCTAssertEqual(user.userId, 1)
        XCTAssertEqual(user.displayName, "Alice")
        XCTAssertTrue(user.emailVerified)
    }

    func testDecoderISO8601Dates() throws {
        let json = #"{"created_at": "2024-12-25T14:30:00Z"}"#.data(using: .utf8)!
        struct TestModel: Codable {
            let createdAt: Date
        }
        let model = try decoder.decode(TestModel.self, from: json)
        let components = Calendar.current.dateComponents(in: TimeZone(secondsFromGMT: 0)!, from: model.createdAt)
        XCTAssertEqual(components.year, 2024)
        XCTAssertEqual(components.month, 12)
        XCTAssertEqual(components.day, 25)
        XCTAssertEqual(components.hour, 14)
        XCTAssertEqual(components.minute, 30)
    }

    // MARK: - Server contract

    /// `api/contract` holds the server's answer to each request the app
    /// makes, recorded by `TestAPIContract` (`internal/app`). Every answer
    /// must decode as the type the app reads it into, keeping every item.
    func testEveryContractAnswerDecodes() throws {
        let files = try FileManager.default.contentsOfDirectory(at: contractDirectory, includingPropertiesForKeys: nil)
            .filter { $0.pathExtension == "json" }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }
        let names = Set(files.map { $0.deletingPathExtension().lastPathComponent })
        XCTAssertFalse(names.isEmpty, "no answers in \(contractDirectory.path)")
        XCTAssertEqual(names.subtracting(contractChecks.keys).sorted(), [],
                       "answers without a check: add the type the app reads each into")
        XCTAssertEqual(Set(contractChecks.keys).subtracting(names).sorted(), [],
                       "checks for answers api/contract no longer has")
        for file in files {
            let name = file.deletingPathExtension().lastPathComponent
            guard let check = contractChecks[name] else { continue }
            do {
                try check(Data(contentsOf: file))
            } catch {
                XCTFail("\(name).json: \(error)")
            }
        }
    }
}

// MARK: - Server contract checks

private let contractDirectory = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent()
    .deletingLastPathComponent()
    .deletingLastPathComponent()
    .appendingPathComponent("api/contract", isDirectory: true)

private typealias ContractCheck = (Data) throws -> Void

private struct ContractMismatch: Error, CustomStringConvertible {
    let description: String
}

private struct ContractExchange<Response: Decodable>: Decodable {
    let status: Int
    let response: Response
}

private func contractExpect(_ condition: Bool, _ message: @autoclosure () -> String) throws {
    if !condition { throw ContractMismatch(description: message()) }
}

/// Decodes the answer as `APIClient` does: a 2xx answer as the request's
/// type, any other as an `APIErrorResponse`.
private func contractAnswer<T: Decodable>(_ type: T.Type, _ data: Data) throws -> T {
    let exchange = try apiDecoder.decode(ContractExchange<T>.self, from: data)
    let isError = type == APIErrorResponse.self
    try contractExpect((200...299).contains(exchange.status) != isError, "status \(exchange.status) for a \(type)")
    return exchange.response
}

/// How many items the server sent in the answer's list or map `key`.
private func contractItems(_ data: Data, _ key: String) throws -> Int {
    let exchange = try JSONSerialization.jsonObject(with: data) as? [String: Any]
    switch (exchange?["response"] as? [String: Any])?[key] {
    case let list as [Any]: return list.count
    case let map as [String: Any]: return map.count
    default: return 0
    }
}

/// Decodes the answer and checks that it kept all the server's items.
/// `EmptyIfNull` reads a missing list as `[]`, so a list the server renamed
/// would otherwise decode as an empty one.
private func contractList<T: Decodable>(_ type: T.Type, _ key: String, _ count: @escaping (T) -> Int) -> ContractCheck {
    { data in
        let sent = try contractItems(data, key)
        let answer = try contractAnswer(type, data)
        let kept = count(answer)
        try contractExpect(sent > 0, "no \(key) in the answer")
        try contractExpect(kept == sent, "\(key): decoded \(kept) of \(sent)")
    }
}

private func contractDecodes<T: Decodable>(_ type: T.Type) -> ContractCheck {
    { data in _ = try contractAnswer(type, data) }
}

private let contractUser: ContractCheck = { data in
    let user = try contractAnswer(UserResponse.self, data).user
    try contractExpect(user != nil, "no user")
}

private let contractStatus: ContractCheck = { data in
    let status = try contractAnswer(StatusResponse.self, data).status
    try contractExpect(!status.isEmpty, "no status")
}

/// Creating or joining a household is read as both types, by sign-up and
/// by Settings.
private let contractHousehold: ContractCheck = { data in
    _ = try contractAnswer(HouseholdOnlyResponse.self, data)
    _ = try contractAnswer(HouseholdResponse.self, data)
}

private let contractLogChange: ContractCheck = { data in
    let change = try contractAnswer(LogChangeResponse.self, data)
    try contractExpect(change.reportsLatest, "no latest log")
}

private let contractError: ContractCheck = { data in
    let error = try contractAnswer(APIErrorResponse.self, data)
    try contractExpect(!error.error.isEmpty, "no message")
}

/// The type the app reads each answer into, by file name.
private let contractChecks: [String: ContractCheck] = [
    "apns-register": contractStatus,
    "auth-login": contractUser,
    "auth-logout": contractStatus,
    "auth-magic-link": contractStatus,
    "auth-password-change": contractUser,
    "auth-password-forgot": contractStatus,
    "auth-register": contractUser,
    "auth-register-member": contractUser,
    "auth-verification-resend": contractStatus,
    "chore-create": contractDecodes(ChoreResponse.self),
    "chore-delete": contractStatus,
    "chore-reminder-pref-update": contractDecodes(ChoreReminderPrefResponse.self),
    "chore-reminder-prefs": contractList(ChoreReminderPrefsResponse.self, "prefs") { $0.prefs.count },
    "chore-restore-default": contractDecodes(ChoreResponse.self),
    "chore-update": contractDecodes(ChoreResponse.self),
    "chores": contractList(ChoresResponse.self, "chores") { $0.chores.count },
    "chores-add-packs": contractList(AddPacksResponse.self, "added") { $0.added.count },
    "chores-seed-defaults": contractStatus,
    "day-note-set": contractDecodes(DayNoteResponse.self),
    "day-notes": contractList(DayNotesResponse.self, "notes") { $0.notes.count },
    "error-access-suspended": { data in
        let error = try contractAnswer(APIErrorResponse.self, data)
        try contractExpect(error.status == "access_suspended" && error.household != nil, "not a suspension")
    },
    "error-invalid": contractError,
    "error-signed-out": contractError,
    "household": { data in
        let members = contractList(HouseholdResponse.self, "members") { $0.members.count }
        let invites = contractList(HouseholdResponse.self, "invites") { $0.invites.count }
        try members(data)
        try invites(data)
    },
    "household-activate": contractStatus,
    "household-create": contractHousehold,
    "household-join": contractHousehold,
    "household-leave": contractStatus,
    "household-transfer": contractStatus,
    "household-update": contractDecodes(HouseholdResponse.self),
    "households": contractList(HouseholdsResponse.self, "households") { $0.households.count },
    "invite-create": contractDecodes(InviteResponse.self),
    "invite-create-pending": contractDecodes(InviteResponse.self),
    "invite-delete": contractStatus,
    "invite-preview": { data in
        let preview = try contractAnswer(InvitePreview.self, data)
        try contractExpect(preview.inviter != nil, "no inviter")
    },
    "log-create": { data in
        let log = try contractAnswer(LogResponse.self, data)
        try contractExpect(log.latest != nil, "no latest log")
    },
    "log-delete": contractLogChange,
    "log-update": contractLogChange,
    "logs-history": contractList(HistoryResponse.self, "logs") { $0.logs.count },
    "logs-history-search": contractList(HistoryResponse.self, "logs") { $0.logs.count },
    "logs-latest-per-chore": contractList(LatestLogsResponse.self, "latestLogs") { $0.latestLogs.count },
    "logs-recent-amounts": contractList(RecentAmountsResponse.self, "amounts") { $0.amounts.count },
    "logs-today": contractList(TodayResponse.self, "logs") { $0.logs.count },
    "me": contractUser,
    "me-member": contractUser,
    "member-access": contractStatus,
    "member-remove": contractStatus,
    "member-role": contractStatus,
    "notification-delete": contractStatus,
    "notification-preferences": contractList(NotificationPrefsResponse.self, "availableTypes") { $0.availableTypes.count },
    "notification-preferences-update": contractList(NotificationPrefsResponse.self, "availableTypes") { $0.availableTypes.count },
    "notification-read": contractStatus,
    "notifications": contractList(NotificationsResponse.self, "notifications") { $0.notifications.count },
    "notifications-clear": contractStatus,
    "notifications-read-all": contractStatus,
    "packs": contractList(ActivityPacksResponse.self, "packs") { $0.packs.count },
    "preferences": contractDecodes(UserPreferencesResponse.self),
    "preferences-update": contractDecodes(UserPreferencesResponse.self),
    "schedule-create": contractDecodes(ScheduleResponse.self),
    "schedule-delete": contractStatus,
    "schedule-update": contractDecodes(ScheduleResponse.self),
    "schedules": contractList(SchedulesResponse.self, "schedules") { $0.schedules.count },
]
