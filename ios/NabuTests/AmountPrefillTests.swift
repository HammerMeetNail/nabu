import XCTest
@testable import Nabu

/// Unit tests for the new-log amount prefill rule (PWA parity with
/// `renderLogSheet`'s `cachedAmount`): a plain amount chore, such as a
/// medication dose, starts from the latest log's amount when it was recorded
/// in the sheet's unit.
final class AmountPrefillTests: XCTestCase {
    private let now = Date()

    private func makeChore(indicatorLabels: [String] = [], hasVolumeML: Bool = true) -> Chore {
        Chore(
            id: 1, householdId: 1, name: "Cat Meds", icon: "💊", color: "#000",
            sortOrder: 0, category: "predefined", isPredefined: true, predefinedKey: "cat-meds",
            createdBy: nil, createdAt: now, indicatorLabels: indicatorLabels, indicatorDefaults: [],
            hasVolumeML: hasVolumeML, metricType: "amount", metricUnit: "mg")
    }

    private func makeLog(volumeML: Int?, metricUnit: String?) -> ChoreLog {
        ChoreLog(
            id: 9, householdId: 1, userId: 1, choreId: 1, completedAt: now,
            note: "", indicators: [], slotHour: 12, createdAt: now,
            volumeML: volumeML, indicatorVolumes: nil, metricUnit: metricUnit)
    }

    func testPlainAmountChoreStartsFromLatestDose() {
        XCTAssertEqual(initialAmountForNewLog(makeChore(), latestLogs: [1: makeLog(volumeML: 5, metricUnit: "mg")], unit: "mg"), 5)
    }

    func testLegacyLogWithoutUnitStillPrefills() {
        XCTAssertEqual(initialAmountForNewLog(makeChore(), latestLogs: [1: makeLog(volumeML: 5, metricUnit: nil)], unit: "mg"), 5)
    }

    func testDoseInAnotherUnitIsNotShown() {
        XCTAssertNil(initialAmountForNewLog(makeChore(), latestLogs: [1: makeLog(volumeML: 5, metricUnit: "mL")], unit: "mg"))
    }

    func testTypeChoresAndChoresWithoutAmountsAreUnaffected() {
        let latest = [1: makeLog(volumeML: 120, metricUnit: "mL")]
        XCTAssertNil(initialAmountForNewLog(makeChore(indicatorLabels: ["🍼 formula"]), latestLogs: latest, unit: "mL"))
        XCTAssertNil(initialAmountForNewLog(makeChore(hasVolumeML: false), latestLogs: latest, unit: "mL"))
        XCTAssertNil(initialAmountForNewLog(makeChore(), latestLogs: [:], unit: "mg"))
    }
}
