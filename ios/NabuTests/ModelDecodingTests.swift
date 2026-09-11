import XCTest
@testable import Nabu

final class ModelDecodingTests: XCTestCase {
    let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        d.keyDecodingStrategy = .convertFromSnakeCase
        return d
    }()

    // MARK: - User

    func testDecodeUserResponse() throws {
        let json = #"""
        {
          "user": {
            "id": 1,
            "householdId": 1,
            "email": "test@nabu.local",
            "displayName": "Alice",
            "avatarColor": "#2E86AB",
            "emailVerified": true,
            "role": "owner",
            "createdAt": "2024-12-25T14:30:00Z"
          }
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(UserResponse.self, from: json)
        XCTAssertEqual(response.user?.id, 1)
        XCTAssertEqual(response.user?.email, "test@nabu.local")
        XCTAssertEqual(response.user?.displayName, "Alice")
        XCTAssertEqual(response.user?.avatarColor, "#2E86AB")
        XCTAssertTrue(response.user?.emailVerified ?? false)
        XCTAssertEqual(response.user?.role, "owner")
        XCTAssertEqual(response.user?.householdId, 1)
    }

    func testDecodeNullUser() throws {
        let json = #"{"user": null}"#.data(using: .utf8)!
        let response = try decoder.decode(UserResponse.self, from: json)
        XCTAssertNil(response.user)
    }

    func testClaimedAccountHasNoPassword() throws {
        let json = ##"{"user":{"id":1,"householdId":null,"email":"owner@test.local","displayName":"Owner","avatarColor":"#19323C","emailVerified":true,"hasPassword":false,"role":"","createdAt":"2026-09-10T12:00:00Z"}}"##.data(using: .utf8)!
        let user = try apiDecoder.decode(UserResponse.self, from: json).user
        XCTAssertEqual(user?.hasPassword, false)
        XCTAssertEqual(user?.emailVerified, true)
    }

    // MARK: - Household

    func testDecodeHouseholdResponse() throws {
        let json = #"""
        {
          "household": {
            "id": 1,
            "name": "My Home",
            "initials": "MH",
            "inviteCode": "ABC123",
            "createdAt": "2024-12-25T14:30:00Z"
          },
          "members": [
            {
              "userId": 1,
              "email": "alice@test.com",
              "displayName": "Alice",
              "avatarColor": "#2E86AB",
              "emailVerified": true,
              "role": "owner"
            },
            {
              "userId": 2,
              "email": "bob@test.com",
              "displayName": "Bob",
              "avatarColor": "#F59E0B",
              "emailVerified": false,
              "role": "member"
            }
          ],
          "historicalMembers": [
            {
              "userId": 3,
              "displayName": "Former Member",
              "avatarColor": "#123456"
            }
          ],
          "invites": [
            {
              "id": 1,
              "householdId": 1,
              "code": "XYZ789",
              "createdBy": 1,
              "maxUses": 10,
              "usedCount": 2,
              "expiresAt": null,
              "createdAt": "2024-12-25T14:30:00Z"
            }
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(HouseholdResponse.self, from: json)
        XCTAssertEqual(response.household.id, 1)
        XCTAssertEqual(response.household.name, "My Home")
        XCTAssertEqual(response.household.initials, "MH")
        XCTAssertEqual(response.household.inviteCode, "ABC123")
        XCTAssertEqual(response.members.count, 2)
        XCTAssertEqual(response.members[0].userId, 1)
        XCTAssertEqual(response.members[0].role, "owner")
        XCTAssertEqual(response.members[1].role, "member")
        XCTAssertEqual(response.historicalMembers.map(\.displayName), ["Former Member"])
        XCTAssertEqual(response.invites.count, 1)
        XCTAssertEqual(response.invites[0].code, "XYZ789")
        XCTAssertEqual(response.invites[0].maxUses, 10)
        XCTAssertEqual(response.invites[0].usedCount, 2)
        XCTAssertNil(response.invites[0].expiresAt)
    }

    // MARK: - Chores

    func testDecodeChoresResponse() throws {
        let json = #"""
        {
          "chores": [
            {
              "id": 1,
              "householdId": 1,
              "name": "Feed Cats",
              "icon": "\ud83d\udc31",
              "color": "#F59E0B",
              "sortOrder": 0,
              "category": "feeding",
              "isPredefined": true,
              "predefinedKey": "Feed Cats",
              "createdBy": null,
              "createdAt": "2024-12-25T14:30:00Z",
              "indicatorLabels": ["\ud83c\udf7c formula", "\ud83e\udd5b breast"],
              "indicatorDefaults": [],
              "hasVolumeML": true
            },
            {
              "id": 2,
              "householdId": 1,
              "name": "Laundry",
              "icon": "\ud83d\udc55",
              "color": "#A78BFA",
              "sortOrder": 1,
              "category": "cleaning",
              "isPredefined": false,
              "predefinedKey": "Laundry",
              "createdBy": 1,
              "createdAt": "2024-12-25T14:30:00Z",
              "indicatorLabels": [],
              "indicatorDefaults": [],
              "hasVolumeML": false
            }
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(ChoresResponse.self, from: json)
        XCTAssertEqual(response.chores.count, 2)
        let chore = response.chores[0]
        XCTAssertEqual(chore.id, 1)
        XCTAssertEqual(chore.name, "Feed Cats")
        XCTAssertEqual(chore.icon, "🐱")
        XCTAssertEqual(chore.color, "#F59E0B")
        XCTAssertTrue(chore.isPredefined)
        XCTAssertEqual(chore.predefinedKey, "Feed Cats")
        XCTAssertNil(chore.createdBy)
        XCTAssertTrue(chore.hasVolumeML)
        XCTAssertEqual(chore.indicatorLabels, ["🍼 formula", "🥛 breast"])
    }

    // MARK: - ChoreLog

    func testDecodeLogWithAllFields() throws {
        let json = #"""
        {
          "log": {
            "id": 1,
            "householdId": 1,
            "userId": 1,
            "choreId": 1,
            "completedAt": "2024-12-25T14:30:00Z",
            "note": "Used the good litter",
            "indicators": ["\ud83c\udf7c formula"],
            "slotHour": 14,
            "createdAt": "2024-12-25T14:30:01Z",
            "volumeML": 120
          }
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(LogResponse.self, from: json)
        let log = response.log
        XCTAssertEqual(log.id, 1)
        XCTAssertEqual(log.choreId, 1)
        XCTAssertEqual(log.userId, 1)
        XCTAssertEqual(log.note, "Used the good litter")
        XCTAssertEqual(log.indicators, ["🍼 formula"])
        XCTAssertEqual(log.slotHour, 14)
        XCTAssertEqual(log.volumeML, 120)
    }

    func testDecodeLogMinimalFields() throws {
        let json = #"""
        {
          "log": {
            "id": 2,
            "householdId": 1,
            "userId": 1,
            "choreId": 1,
            "completedAt": "2024-12-25T08:00:00Z",
            "note": "",
            "indicators": [],
            "createdAt": "2024-12-25T08:00:01Z"
          }
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(LogResponse.self, from: json)
        let log = response.log
        XCTAssertEqual(log.id, 2)
        XCTAssertNil(log.slotHour)
        XCTAssertNil(log.volumeML)
        XCTAssertEqual(log.note, "")
        XCTAssertTrue(log.indicators.isEmpty)
    }

    // MARK: - Today

    func testDecodeTodayResponse() throws {
        let json = #"""
        {
          "logs": [
            {
              "id": 1,
              "householdId": 1,
              "userId": 1,
              "choreId": 1,
              "completedAt": "2024-12-25T14:30:00Z",
              "note": "",
              "indicators": ["\ud83c\udf7c formula"],
              "slotHour": 14,
              "createdAt": "2024-12-25T14:30:01Z",
              "volumeML": 120
            },
            {
              "id": 2,
              "householdId": 1,
              "userId": 1,
              "choreId": 2,
              "completedAt": "2024-12-25T10:00:00Z",
              "note": "",
              "indicators": [],
              "slotHour": 10,
              "createdAt": "2024-12-25T10:00:01Z"
            }
          ],
          "summary": {
            "date": "2024-12-25",
            "totalChores": 5,
            "choresDone": 2,
            "byUser": {"1": 2},
            "byCategory": {"feeding": 1, "cleaning": 1}
          },
          "date": "2024-12-25"
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(TodayResponse.self, from: json)
        XCTAssertEqual(response.logs.count, 2)
        XCTAssertEqual(response.summary.date, "2024-12-25")
        XCTAssertEqual(response.summary.totalChores, 5)
        XCTAssertEqual(response.summary.choresDone, 2)
        XCTAssertEqual(response.summary.byUser, ["1": 2])
        XCTAssertEqual(response.summary.byCategory, ["feeding": 1, "cleaning": 1])
        XCTAssertEqual(response.date, "2024-12-25")
    }

    // MARK: - Schedule

    func testDecodeSchedulesResponse() throws {
        let json = #"""
        {
          "schedules": [
            {
              "id": 1,
              "householdId": 1,
              "choreId": 1,
              "frequencyType": "daily",
              "timePeriod": "anytime",
              "specificTime": "08:00",
              "timesOfDay": [],
              "daysOfWeek": [],
              "intervalDays": 0,
              "dayOfMonth": 0,
              "monthWeekday": null,
              "monthOfYear": 0,
              "recurrenceEnd": null,
              "startDate": null,
              "targetCount": 0,
              "isActive": true,
              "isFollowUp": false,
              "assignedUserId": 1,
              "createdAt": "2024-12-25T14:30:00Z",
              "updatedAt": "2024-12-25T14:30:00Z"
            },
            {
              "id": 2,
              "householdId": 1,
              "choreId": 3,
              "frequencyType": "weekly",
              "timePeriod": "anytime",
              "specificTime": null,
              "timesOfDay": [],
              "daysOfWeek": [1, 3, 5],
              "intervalDays": 0,
              "dayOfMonth": 0,
              "monthWeekday": null,
              "monthOfYear": 0,
              "recurrenceEnd": "2025-06-01T00:00:00Z",
              "startDate": "2024-01-01",
              "targetCount": 0,
              "isActive": true,
              "isFollowUp": false,
              "assignedUserId": null,
              "createdAt": "2024-12-25T14:30:00Z",
              "updatedAt": "2024-12-25T14:30:00Z"
            }
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(SchedulesResponse.self, from: json)
        XCTAssertEqual(response.schedules.count, 2)

        let daily = response.schedules[0]
        XCTAssertEqual(daily.frequencyType, "daily")
        XCTAssertEqual(daily.specificTime, "08:00")
        XCTAssertEqual(daily.assignedUserId, 1)
        XCTAssertTrue(daily.isActive)
        XCTAssertNil(daily.recurrenceEnd)
        XCTAssertNil(daily.startDate)

        let weekly = response.schedules[1]
        XCTAssertEqual(weekly.frequencyType, "weekly")
        XCTAssertEqual(weekly.daysOfWeek, [1, 3, 5])
        XCTAssertNil(weekly.specificTime)
        XCTAssertNotNil(weekly.recurrenceEnd)
        XCTAssertEqual(weekly.startDate, "2024-01-01")
        XCTAssertNil(weekly.assignedUserId)
    }

    func testDecodeScheduleWithNullAndOmittedFields() throws {
        // Reproduces actual Go server JSON: null daysOfWeek for non-weekly,
        // omitted dayOfMonth/monthOfYear when zero (omitempty).
        let json = #"""
        {
          "schedules": [
            {
              "id": 1,
              "householdId": 1,
              "choreId": 1,
              "frequencyType": "daily",
              "timePeriod": "anytime",
              "timesOfDay": [],
              "daysOfWeek": null,
              "intervalDays": 0,
              "targetCount": 0,
              "isActive": true,
              "isFollowUp": false,
              "assignedUserId": null,
              "createdAt": "2024-12-25T14:30:00Z",
              "updatedAt": "2024-12-25T14:30:00Z"
            }
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(SchedulesResponse.self, from: json)
        XCTAssertEqual(response.schedules.count, 1)
        let sch = response.schedules[0]
        XCTAssertEqual(sch.frequencyType, "daily")
        XCTAssertEqual(sch.daysOfWeek, [])
        XCTAssertEqual(sch.dayOfMonth, 0)
        XCTAssertEqual(sch.monthOfYear, 0)
        XCTAssertNil(sch.specificTime)
        XCTAssertNil(sch.recurrenceEnd)
        XCTAssertNil(sch.startDate)
        XCTAssertNil(sch.monthWeekday)
        XCTAssertNil(sch.assignedUserId)
    }

    func testDecodeScheduleWithAPIDecoder() throws {
        // Use the real apiDecoder with Go-style fractional-second timestamps
        let json = #"""
        {
          "schedules": [
            {
              "id": 1,
              "householdId": 1,
              "choreId": 1,
              "frequencyType": "weekly",
              "timePeriod": "anytime",
              "specificTime": "09:00",
              "timesOfDay": [],
              "daysOfWeek": [1, 3],
              "intervalDays": 0,
              "targetCount": 0,
              "isActive": true,
              "isFollowUp": false,
              "assignedUserId": null,
              "createdAt": "2026-06-08T12:00:00.123456Z",
              "updatedAt": "2026-06-08T12:00:00.123456Z"
            }
          ]
        }
        """#.data(using: .utf8)!
        let response = try apiDecoder.decode(SchedulesResponse.self, from: json)
        XCTAssertEqual(response.schedules.count, 1)
        let sch = response.schedules[0]
        XCTAssertEqual(sch.frequencyType, "weekly")
        XCTAssertEqual(sch.daysOfWeek, [1, 3])
        XCTAssertEqual(sch.specificTime, "09:00")
        XCTAssertTrue(sch.isActive)
    }

    func testDecodeScheduleWithNullDaysOfWeekAndGoTimestamps() throws {
        // Realistic Go server response: null daysOfWeek, omitted dayOfMonth/monthOfYear,
        // RecurrenceEnd as Date, fractional-second timestamps.
        let json = #"""
        {
          "schedules": [
            {
              "id": 2,
              "householdId": 1,
              "choreId": 3,
              "frequencyType": "daily",
              "timePeriod": "anytime",
              "timesOfDay": [],
              "daysOfWeek": null,
              "intervalDays": 0,
              "targetCount": 0,
              "isActive": true,
              "isFollowUp": false,
              "assignedUserId": null,
              "recurrenceEnd": "2026-12-31T00:00:00Z",
              "createdAt": "2026-06-08T15:30:00.123456789Z",
              "updatedAt": "2026-06-08T15:30:00.123456789Z"
            }
          ]
        }
        """#.data(using: .utf8)!
        let response = try apiDecoder.decode(SchedulesResponse.self, from: json)
        XCTAssertEqual(response.schedules.count, 1)
        let sch = response.schedules[0]
        XCTAssertEqual(sch.frequencyType, "daily")
        XCTAssertEqual(sch.daysOfWeek, [])
        XCTAssertEqual(sch.dayOfMonth, 0)
        XCTAssertEqual(sch.monthOfYear, 0)
        XCTAssertNil(sch.specificTime)
        XCTAssertNotNil(sch.recurrenceEnd)
        XCTAssertNil(sch.monthWeekday)
    }

    // MARK: - Notifications

    func testDecodeNotificationsResponse() throws {
        let json = #"""
        {
          "notifications": [
            {
              "id": 1,
              "userId": 1,
              "type": "chore_logged",
              "title": "\ud83d\udc31 Feed Cats",
              "body": "Alice logged this",
              "isRead": false,
              "createdAt": "2024-12-25T14:30:00Z"
            },
            {
              "id": 2,
              "userId": 1,
              "type": "household_joined",
              "title": "Bob joined",
              "body": "Bob joined My Home",
              "isRead": true,
              "createdAt": "2024-12-24T10:00:00Z"
            }
          ],
          "unreadCount": 1
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(NotificationsResponse.self, from: json)
        XCTAssertEqual(response.notifications.count, 2)
        XCTAssertEqual(response.unreadCount, 1)
        XCTAssertEqual(response.notifications[0].type, "chore_logged")
        XCTAssertFalse(response.notifications[0].isRead)
        XCTAssertEqual(response.notifications[1].type, "household_joined")
        XCTAssertTrue(response.notifications[1].isRead)
    }

    // MARK: - Preferences

    func testDecodeNotificationPrefs() throws {
        let json = #"""
        {
          "preferences": {
            "userId": 1,
            "pushEnabled": true,
            "emailEnabled": false,
            "quietHoursStart": "22:00",
            "quietHoursEnd": "07:00",
            "timezone": "America/New_York",
            "enabledPushTypes": ["chore_logged", "household_joined"],
            "defaultReminderLeadMinutes": 10
          },
          "availableTypes": [
            {"type": "chore_logged", "label": "Chore Logged", "description": "When someone else in your household logs a chore."},
            {"type": "household_joined", "label": "Household Joined", "description": "When someone joins your household."}
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(NotificationPrefsResponse.self, from: json)
        XCTAssertTrue(response.preferences.pushEnabled)
        XCTAssertFalse(response.preferences.emailEnabled)
        XCTAssertEqual(response.preferences.quietHoursStart, "22:00")
        XCTAssertEqual(response.preferences.quietHoursEnd, "07:00")
        XCTAssertEqual(response.preferences.timezone, "America/New_York")
        XCTAssertEqual(response.preferences.enabledPushTypes, ["chore_logged", "household_joined"])
        XCTAssertEqual(response.availableTypes.count, 2)
    }

    func testDecodeUserPreferences() throws {
        let json = #"""
        {
          "preferences": {
            "choreOrder": [1, 3, 2],
            "hiddenHomeChoreIds": [2],
            "timezone": "America/New_York"
          }
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(UserPreferencesResponse.self, from: json)
        XCTAssertEqual(response.preferences.choreOrder, [1, 3, 2])
        XCTAssertEqual(response.preferences.hiddenHomeChoreIds, [2])
        XCTAssertEqual(response.preferences.timezone, "America/New_York")
    }

    // MARK: - Stats

    func testDecodeLeaderboard() throws {
        let json = #"""
        {
          "leaderboard": [{"userId": 1, "count": 12}, {"userId": 2, "count": 8}],
          "start": "2024-12-18",
          "end": "2024-12-25"
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(LeaderboardResponse.self, from: json)
        XCTAssertEqual(response.leaderboard.count, 2)
        XCTAssertEqual(response.leaderboard[0].userId, 1)
        XCTAssertEqual(response.leaderboard[0].count, 12)
        XCTAssertEqual(response.start, "2024-12-18")
        XCTAssertEqual(response.end, "2024-12-25")
    }

    func testDecodeLeaderboardAllTime() throws {
        let json = #"""
        {
          "leaderboard": [{"userId": 1, "count": 99}],
          "period": "all"
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(LeaderboardResponse.self, from: json)
        XCTAssertEqual(response.leaderboard.count, 1)
        XCTAssertEqual(response.leaderboard[0].userId, 1)
        XCTAssertEqual(response.leaderboard[0].count, 99)
        XCTAssertNil(response.start)
        XCTAssertNil(response.end)
    }

    func testDecodeStreaks() throws {
        let json = #"{"streaks": {"current": 5, "longest": 14}}"#.data(using: .utf8)!
        let response = try decoder.decode(StreaksResponse.self, from: json)
        XCTAssertEqual(response.streaks.current, 5)
        XCTAssertEqual(response.streaks.longest, 14)
    }

    func testDecodeOverview() throws {
        let json = #"""
        {
          "overview": {
            "leaderboard": [{"userId": 1, "count": 12}],
            "streaks": {"current": 5, "longest": 14},
            "breakdown": [{"category": "feeding", "count": 5}],
            "recap": {
              "totalChores": 12,
              "topPerformer": {"userId": 1, "count": 8},
              "mostActiveDay": "Monday",
              "byCategory": [{"category": "feeding", "count": 3}]
            }
          }
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(OverviewResponse.self, from: json)
        XCTAssertEqual(response.overview.leaderboard.count, 1)
        XCTAssertEqual(response.overview.streaks.current, 5)
        XCTAssertEqual(response.overview.recap.totalChores, 12)
        XCTAssertEqual(response.overview.recap.topPerformer?.userId, 1)
        XCTAssertEqual(response.overview.recap.mostActiveDay, "Monday")
    }

    func testDecodeBusyHours() throws {
        var hours = ""
        for h in 0...23 {
            let count = h == 8 ? 5 : (h == 7 ? 3 : 0)
            if h > 0 { hours += "," }
            hours += #"{"hour": \#(h), "count": \#(count)}"#
        }
        let json = #"{"busyHours": [\#(hours)], "start": "2024-12-18", "end": "2024-12-25"}"#.data(using: .utf8)!
        let response = try decoder.decode(BusyHoursResponse.self, from: json)
        XCTAssertEqual(response.busyHours.count, 24)
        XCTAssertEqual(response.busyHours[8].hour, 8)
        XCTAssertEqual(response.busyHours[8].count, 5)
    }

    func testDecodeTopChores() throws {
        let json = #"""
        {
          "topChores": [
            {"choreId": 1, "choreName": "Feed Cats", "choreIcon": "\ud83d\udc31", "count": 12},
            {"choreId": 3, "choreName": "Walk Dog", "choreIcon": "\ud83d\udc15", "count": 4}
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(TopChoresResponse.self, from: json)
        XCTAssertEqual(response.topChores.count, 2)
        XCTAssertEqual(response.topChores[0].choreId, 1)
        XCTAssertEqual(response.topChores[0].count, 12)
        XCTAssertEqual(response.topChores[1].choreId, 3)
        XCTAssertEqual(response.topChores[1].count, 4)
    }

    // MARK: - Error

    func testDecodeAPIError() throws {
        let json = #"{"error": "Invalid input"}"#.data(using: .utf8)!
        let error = try decoder.decode(APIErrorResponse.self, from: json)
        XCTAssertEqual(error.error, "Invalid input")
    }

    // MARK: - Status

    func testDecodeStatusResponse() throws {
        let json = #"{"status":"ok"}"#.data(using: .utf8)!
        let response = try decoder.decode(StatusResponse.self, from: json)
        XCTAssertEqual(response.status, "ok")
    }

    // MARK: - Metric config (migration 036/039)

    func testDecodeChoreWithMetricFields() throws {
        let json = #"""
        {
          "chore": {
            "id": 5,
            "householdId": 1,
            "name": "Feed Baby",
            "icon": "🍼",
            "color": "#2E86AB",
            "sortOrder": 0,
            "category": "Baby",
            "isPredefined": true,
            "predefinedKey": "feed_baby",
            "createdBy": null,
            "createdAt": "2024-12-25T14:30:00Z",
            "indicatorLabels": ["🍼 formula", "🤱 breast"],
            "indicatorDefaults": [],
            "hasVolumeML": true,
            "hasRating": false,
            "metricType": "amount",
            "metricUnit": "mL",
            "subjects": ["Ada", "Grace"]
          }
        }
        """#.data(using: .utf8)!
        let chore = try decoder.decode(ChoreResponse.self, from: json).chore
        XCTAssertEqual(chore.metricType, "amount")
        XCTAssertEqual(chore.metricUnit, "mL")
        XCTAssertEqual(chore.subjects, ["Ada", "Grace"])
        XCTAssertFalse(chore.hasRating)
    }

    func testDecodeChoreLegacyPayloadDefaultsMetricFields() throws {
        // A server (or fixture) that predates migrations 036/039 sends no
        // metric fields; the model must default rather than fail.
        let json = #"""
        {
          "chore": {
            "id": 6,
            "householdId": 1,
            "name": "Dishes",
            "icon": "🍽️",
            "color": "#386641",
            "sortOrder": 1,
            "category": "Kitchen",
            "isPredefined": false,
            "createdAt": "2024-12-25T14:30:00Z",
            "indicatorLabels": null,
            "indicatorDefaults": null,
            "hasVolumeML": false,
            "subjects": null
          }
        }
        """#.data(using: .utf8)!
        let chore = try decoder.decode(ChoreResponse.self, from: json).chore
        XCTAssertEqual(chore.metricType, "none")
        XCTAssertEqual(chore.metricUnit, "")
        XCTAssertEqual(chore.subjects, [])
        XCTAssertFalse(chore.hasRating)
    }

    func testDecodeLogWithDurationSubjectAndRating() throws {
        let json = #"""
        {
          "log": {
            "id": 3,
            "householdId": 1,
            "userId": 1,
            "choreId": 5,
            "completedAt": "2024-12-25T14:30:00Z",
            "title": "Nap",
            "note": "",
            "indicators": [],
            "createdAt": "2024-12-25T14:30:01Z",
            "rating": 45,
            "durationSeconds": 1800,
            "subject": "Ada"
          }
        }
        """#.data(using: .utf8)!
        let log = try decoder.decode(LogResponse.self, from: json).log
        XCTAssertEqual(log.title, "Nap")
        XCTAssertEqual(log.rating, 45)
        XCTAssertEqual(log.durationSeconds, 1800)
        XCTAssertEqual(log.subject, "Ada")
    }

    // MARK: - Preferences (volumeUnit / stats sections / widgets)

    func testDecodeUserPreferencesWithWidgetFields() throws {
        let json = #"""
        {
          "preferences": {
            "choreOrder": [1],
            "hiddenHomeChoreIds": [],
            "timezone": "UTC",
            "volumeUnit": "oz",
            "statsSectionOrder": ["overview", "widget:abc-123"],
            "statsSectionHidden": ["recap"],
            "statsWidgets": [
              {
                "id": "abc-123",
                "type": "total",
                "choreIds": [5],
                "metric": "amount",
                "agg": "sum",
                "period": "week",
                "grain": "daily",
                "title": "Bottles this week"
              }
            ]
          }
        }
        """#.data(using: .utf8)!
        let prefs = try decoder.decode(UserPreferencesResponse.self, from: json).preferences
        XCTAssertEqual(prefs.volumeUnit, "oz")
        XCTAssertEqual(prefs.statsSectionOrder, ["overview", "widget:abc-123"])
        XCTAssertEqual(prefs.statsSectionHidden, ["recap"])
        XCTAssertEqual(prefs.statsWidgets.count, 1)
        let w = try XCTUnwrap(prefs.statsWidgets.first)
        XCTAssertEqual(w.id, "abc-123")
        XCTAssertEqual(w.type, "total")
        XCTAssertEqual(w.choreIds, [5])
        XCTAssertEqual(w.metric, "amount")
        XCTAssertEqual(w.agg, "sum")
        XCTAssertEqual(w.period, "week")
        XCTAssertEqual(w.grain, "daily")
        XCTAssertEqual(w.title, "Bottles this week")
    }

    func testDecodeUserPreferencesHideNotificationBadge() throws {
        // Explicit true and false both decode; a payload from an older
        // server that omits the field defaults to false (badge shown).
        let on = #"""
        {
          "preferences": {
            "choreOrder": [],
            "hiddenHomeChoreIds": [],
            "timezone": "UTC",
            "volumeUnit": "ml",
            "hideNotificationBadge": true
          }
        }
        """#.data(using: .utf8)!
        XCTAssertEqual(try decoder.decode(UserPreferencesResponse.self, from: on).preferences.hideNotificationBadge, true)

        let off = #"""
        {
          "preferences": {
            "choreOrder": [],
            "hiddenHomeChoreIds": [],
            "timezone": "UTC",
            "volumeUnit": "ml",
            "hideNotificationBadge": false
          }
        }
        """#.data(using: .utf8)!
        XCTAssertEqual(try decoder.decode(UserPreferencesResponse.self, from: off).preferences.hideNotificationBadge, false)

        let legacy = #"""
        {
          "preferences": {
            "choreOrder": [],
            "hiddenHomeChoreIds": [],
            "timezone": "UTC",
            "volumeUnit": "ml"
          }
        }
        """#.data(using: .utf8)!
        XCTAssertEqual(try decoder.decode(UserPreferencesResponse.self, from: legacy).preferences.hideNotificationBadge, false)
    }

    func testDecodeUserPreferencesLegacyDefaultsNewFields() throws {
        // Go marshals nil slices as null and older payloads omit the fields
        // entirely — both must decode with defaults.
        let json = #"""
        {
          "preferences": {
            "choreOrder": [2, 1],
            "hiddenHomeChoreIds": null,
            "timezone": "America/New_York",
            "statsWidgets": null
          }
        }
        """#.data(using: .utf8)!
        let prefs = try decoder.decode(UserPreferencesResponse.self, from: json).preferences
        XCTAssertEqual(prefs.choreOrder, [2, 1])
        XCTAssertEqual(prefs.hiddenHomeChoreIds, [])
        XCTAssertEqual(prefs.volumeUnit, "ml")
        XCTAssertEqual(prefs.statsSectionOrder, [])
        XCTAssertEqual(prefs.statsSectionHidden, [])
        XCTAssertEqual(prefs.statsWidgets, [])
        XCTAssertEqual(prefs.hideNotificationBadge, false)
    }

    // MARK: - Day notes

    func testDecodeDayNotesResponse() throws {
        let json = #"""
        {
          "notes": [
            {"date": "2026-07-01", "note": "First solid food!", "updatedBy": 1, "updatedAt": "2026-07-01T19:00:00Z"},
            {"date": "2026-06-30", "note": "Rough night", "updatedAt": "2026-06-30T22:00:00Z"}
          ]
        }
        """#.data(using: .utf8)!
        let response = try decoder.decode(DayNotesResponse.self, from: json)
        XCTAssertEqual(response.notes.count, 2)
        XCTAssertEqual(response.notes[0].date, "2026-07-01")
        XCTAssertEqual(response.notes[0].note, "First solid food!")
        XCTAssertEqual(response.notes[0].updatedBy, 1)
        XCTAssertNil(response.notes[1].updatedBy)
    }

    func testDecodeDayNoteResponse() throws {
        let json = #"""
        {"note": {"date": "2026-07-02", "note": "Checkup day", "updatedBy": 2, "updatedAt": "2026-07-02T09:00:00Z"}}
        """#.data(using: .utf8)!
        let response = try decoder.decode(DayNoteResponse.self, from: json)
        XCTAssertEqual(response.note.date, "2026-07-02")
        XCTAssertEqual(response.note.note, "Checkup day")
    }

    // MARK: - Chore summary + time-series metric fields

    func testDecodeChoreSummaryResponse() throws {
        let json = #"""
        {
          "summary": {
            "choreId": 5,
            "count": 12,
            "totalML": 960,
            "totalDuration": 0,
            "byMember": [{"userId": 1, "count": 8}, {"userId": 2, "count": 4}],
            "metricType": "amount",
            "metricUnit": "mL"
          }
        }
        """#.data(using: .utf8)!
        let summary = try decoder.decode(ChoreSummaryResponse.self, from: json).summary
        XCTAssertEqual(summary.choreId, 5)
        XCTAssertEqual(summary.count, 12)
        XCTAssertEqual(summary.totalML, 960)
        XCTAssertEqual(summary.byMember.count, 2)
        XCTAssertEqual(summary.metricType, "amount")
    }

    func testDecodeTimeSeriesWithDurationAndMetric() throws {
        let json = #"""
        {
          "timeSeries": {
            "choreId": 7,
            "choreName": "Nap",
            "choreIcon": "😴",
            "metricType": "duration",
            "byMember": [{"userId": 1, "count": 3}],
            "periods": [
              {"start": "2026-06-29", "end": "2026-06-30", "count": 2, "totalML": 0, "totalDuration": 5400}
            ]
          }
        }
        """#.data(using: .utf8)!
        let ts = try decoder.decode(TimeSeriesResponse.self, from: json).timeSeries
        XCTAssertEqual(ts.metricType, "duration")
        XCTAssertNil(ts.metricUnit)
        XCTAssertEqual(ts.periods[0].totalDuration, 5400)
    }
}
