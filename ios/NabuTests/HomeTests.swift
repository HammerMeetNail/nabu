import XCTest
import SwiftUI
@testable import Nabu

final class HomeTests: XCTestCase {

    // MARK: - HomeViewMode

    func testHomeViewModeLog() {
        XCTAssertEqual(HomeViewMode.log.title, "Log")
    }

    func testHomeViewModeManage() {
        XCTAssertEqual(HomeViewMode.manage.title, "Manage")
    }

    func testHomeViewModeAllCases() {
        XCTAssertEqual(HomeViewMode.allCases.count, 2)
        XCTAssertTrue(HomeViewMode.allCases.contains(.log))
        XCTAssertTrue(HomeViewMode.allCases.contains(.manage))
    }

    // MARK: - Color hex

    func testColorHexValid() {
        let color = Color(hex: "#FF0000")
        XCTAssertNotNil(color)
    }

    func testColorHexValidWithoutHash() {
        let color = Color(hex: "00FF00")
        XCTAssertNotNil(color)
    }

    func testColorHexInvalid() {
        XCTAssertNil(Color(hex: "not-a-color"))
        XCTAssertNil(Color(hex: "12345"))  // too short
    }

    // MARK: - Chore sorting

    func testSortChoresByPreferenceOrder() {
        let a = Chore(id: 1, householdId: 1, name: "A", icon: "a", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
        let b = Chore(id: 2, householdId: 1, name: "B", icon: "b", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
        let c = Chore(id: 3, householdId: 1, name: "C", icon: "c", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)

        let chores = [a, b, c]
        let order = [2, 1]  // b first, then a, c not in order (goes last)

        let orderMap = Dictionary(uniqueKeysWithValues: order.enumerated().map { ($1, $0) })
        let sorted = chores.sorted {
            (orderMap[$0.id] ?? Int.max) < (orderMap[$1.id] ?? Int.max)
        }

        XCTAssertEqual(sorted.map(\.id), [2, 1, 3])
    }

    func testSortChoresNoPreferenceOrder() {
        let a = Chore(id: 10, householdId: 1, name: "A", icon: "a", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)
        let b = Chore(id: 5, householdId: 1, name: "B", icon: "b", color: "#000000",
                      sortOrder: 0, category: "test", isPredefined: false,
                      predefinedKey: nil, createdBy: nil, createdAt: Date(),
                      indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false)

        let sorted = [a, b].sorted { $0.id < $1.id }
        XCTAssertEqual(sorted.map(\.id), [5, 10])
    }

    // MARK: - formatTimeAgo

    func testTimeAgoJustNow() {
        let now = Date()
        XCTAssertEqual(formatTimeAgo(now), "just now")
    }

    func testTimeAgoMinutes() {
        let date = Date().addingTimeInterval(-300)  // 5 min ago
        XCTAssertEqual(formatTimeAgo(date), "5m ago")
    }

    func testTimeAgoHours() {
        let date = Date().addingTimeInterval(-7200)  // 2 hours ago
        XCTAssertEqual(formatTimeAgo(date), "2h ago")
    }

    func testTimeAgoYesterday() {
        let date = Date().addingTimeInterval(-90000)  // 25 hours ago
        XCTAssertEqual(formatTimeAgo(date), "yesterday")
    }

    func testTimeAgoDays() {
        let date = Date().addingTimeInterval(-259200)  // 3 days ago
        XCTAssertEqual(formatTimeAgo(date), "3d ago")
    }

    func testTimeAgoWeeks() {
        let date = Date().addingTimeInterval(-1209600)  // 2 weeks ago
        XCTAssertEqual(formatTimeAgo(date), "2w ago")
    }
}
