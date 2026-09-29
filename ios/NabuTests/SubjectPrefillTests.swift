import XCTest
@testable import Nabu

/// Unit tests for the new-log subject prefill rule (PWA parity with
/// `renderLogSheet`'s `cachedSubject`): a new log sheet echoes the latest
/// log's subject — e.g. the previously selected cat on Cat Meds — only when
/// that subject is still one of the chore's subjects.
final class SubjectPrefillTests: XCTestCase {
    private let now = Date()

    private func makeChore(subjects: [String]) -> Chore {
        Chore(
            id: 1, householdId: 1, name: "Cat Meds", icon: "💊", color: "#000",
            sortOrder: 0, category: "predefined", isPredefined: true, predefinedKey: "cat-meds",
            createdBy: nil, createdAt: now, indicatorLabels: [], indicatorDefaults: [],
            hasVolumeML: true, subjects: subjects)
    }

    private func makeLog(subject: String?) -> ChoreLog {
        ChoreLog(
            id: 9, householdId: 1, userId: 1, choreId: 1, completedAt: now,
            note: "", indicators: [], slotHour: 12, createdAt: now,
            volumeML: 20, indicatorVolumes: nil, subject: subject)
    }

    func testNewLogPrefillsLatestLogSubject() {
        let chore = makeChore(subjects: ["Milo", "Nala"])
        XCTAssertEqual(initialSubjectForNewLog(chore, latestLogs: [1: makeLog(subject: "Milo")]), "Milo")
    }

    func testNewLogPrefillsNothingWhenSubjectWasRemovedFromChore() {
        let chore = makeChore(subjects: ["Nala"])
        XCTAssertNil(initialSubjectForNewLog(chore, latestLogs: [1: makeLog(subject: "Milo")]))
    }

    func testNewLogPrefillsNothingWithoutLatestLog() {
        let chore = makeChore(subjects: ["Milo", "Nala"])
        XCTAssertNil(initialSubjectForNewLog(chore, latestLogs: [:]))
    }

    func testNewLogPrefillsNothingWhenChoreHasNoSubjects() {
        let chore = makeChore(subjects: [])
        XCTAssertNil(initialSubjectForNewLog(chore, latestLogs: [1: makeLog(subject: "Milo")]))
    }

    func testNewLogPrefillsNothingWhenLatestLogHasNoSubject() {
        let chore = makeChore(subjects: ["Milo", "Nala"])
        XCTAssertNil(initialSubjectForNewLog(chore, latestLogs: [1: makeLog(subject: nil)]))
    }
}
