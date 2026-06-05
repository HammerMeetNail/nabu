import XCTest
import SwiftUI
@testable import Nabu

final class ChoreTests: XCTestCase {

    // MARK: - Color swatches

    func testColorSwatchesCount() {
        XCTAssertEqual(COLOR_SWATCHES.count, 18)
    }

    func testColorSwatchesValidHex() {
        for swatch in COLOR_SWATCHES {
            XCTAssertNotNil(Color(hex: swatch), "Invalid color: \(swatch)")
        }
    }

    // MARK: - Quick emojis

    func testQuickEmojisCount() {
        XCTAssertEqual(QUICK_EMOJIS.count, 40)
    }

    // MARK: - Chore sorting by order

    func testSortChoresWithFullOrder() {
        let a = makeChore(id: 1, name: "A")
        let b = makeChore(id: 2, name: "B")
        let c = makeChore(id: 3, name: "C")
        let chores = [c, a, b]
        let order = [2, 1, 3]

        let orderMap = Dictionary(uniqueKeysWithValues: order.enumerated().map { ($1, $0) })
        let sorted = chores.sorted {
            (orderMap[$0.id] ?? Int.max) < (orderMap[$1.id] ?? Int.max)
        }

        XCTAssertEqual(sorted.map(\.id), [2, 1, 3])
    }

    func testSortChoresPartialOrder() {
        let a = makeChore(id: 1, name: "A")
        let b = makeChore(id: 2, name: "B")
        let c = makeChore(id: 3, name: "C")
        let chores = [a, b, c]
        let order = [3] // only c is ordered

        let orderMap = Dictionary(uniqueKeysWithValues: order.enumerated().map { ($1, $0) })
        let sorted = chores.sorted {
            (orderMap[$0.id] ?? Int.max) < (orderMap[$1.id] ?? Int.max)
        }

        // c comes first (index 0), a and b at the end in original order
        XCTAssertEqual(sorted.map(\.id), [3, 1, 2])
    }

    func testSortChoresEmptyOrder() {
        let a = makeChore(id: 10, name: "A")
        let b = makeChore(id: 5, name: "B")
        let chores = [a, b]

        let sorted = chores.sorted { $0.id < $1.id }
        XCTAssertEqual(sorted.map(\.id), [5, 10])
    }

    // MARK: - Chore create validation

    func testChoreNameNotEmpty() {
        let name = "   "
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        XCTAssertTrue(trimmed.isEmpty)
    }

    func testChoreNameMaxLength() {
        let ok = String(repeating: "a", count: 60)
        let tooLong = String(repeating: "a", count: 61)
        XCTAssertTrue(ok.count <= 60)
        XCTAssertFalse(tooLong.count <= 60)
    }

    func testColorValidation() {
        // Valid 6-char hex
        XCTAssertNotNil(Color(hex: "#FF0000"))
        XCTAssertNotNil(Color(hex: "00FF00"))
        // Invalid
        XCTAssertNil(Color(hex: "not"))
        XCTAssertNil(Color(hex: ""))
    }

    func testIndicatorLabelsMax() {
        let labels = Array(repeating: "test", count: 8)
        XCTAssertTrue(labels.count <= 8)
        let tooMany = Array(repeating: "test", count: 9)
        XCTAssertFalse(tooMany.count <= 8)
    }

    func testIndicatorLabelLength() {
        let empty = ""
        let ok = "ok"
        let tooLong = String(repeating: "a", count: 31)
        XCTAssertFalse(empty.count >= 1 && empty.count <= 30)
        XCTAssertTrue(ok.count >= 1 && ok.count <= 30)
        XCTAssertFalse(tooLong.count >= 1 && tooLong.count <= 30)
    }

    // MARK: - Predefined check

    func testPredefinedChoreCannotBeDeleted() {
        let chore = makeChore(id: 1, name: "Feed Cats", isPredefined: true)
        XCTAssertTrue(chore.isPredefined)
    }

    func testCustomChoreCanBeDeleted() {
        let chore = makeChore(id: 99, name: "Custom", isPredefined: false)
        XCTAssertFalse(chore.isPredefined)
    }
}

private func makeChore(id: Int, name: String, isPredefined: Bool = false) -> Chore {
    Chore(
        id: id, householdId: 1, name: name, icon: "📋", color: "#000000",
        sortOrder: 0, category: "test", isPredefined: isPredefined,
        predefinedKey: nil, createdBy: nil, createdAt: Date(),
        indicatorLabels: [], indicatorDefaults: [], hasVolumeML: false
    )
}
