import XCTest
import UIKit
import SnapshotTesting

private extension XCTestCase {
    func captureReviewScreen(_ app: XCUIApplication, named name: String, element: XCUIElement? = nil,
                             file: StaticString = #filePath, testName: String = #function, line: UInt = #line) throws {
        guard UIDevice.current.systemVersion.hasPrefix("26.") else { return }
        let recording = ProcessInfo.processInfo.environment["NABU_RECORD_SNAPSHOTS"] == "1"
        let previous = continueAfterFailure
        // Keep exercising recovery after a pixel mismatch; the image assertion
        // still fails the test and must be fixed before the gate can pass.
        continueAfterFailure = true
        defer { continueAfterFailure = previous }
        assertSnapshot(of: (element ?? app).screenshot().image,
                       as: .image(precision: 0.99, perceptualPrecision: 0.98),
                       named: name, record: recording, file: file, testName: testName, line: line)
    }
}

/// The local server the real-server tests use: `http://localhost:8080`, or
/// another one set with `TEST_RUNNER_NABU_E2E_BASE_URL=<url> xcodebuild …`.
private let e2eServerURL = ProcessInfo.processInfo.environment["NABU_E2E_BASE_URL"] ?? "http://localhost:8080"

private extension XCUIApplication {
    func launchForLocalTest() {
        if launchArguments.contains("-seedHomeForUITest"),
           !launchArguments.contains("-preserveTestState"), !launchArguments.contains("-resetState") {
            launchArguments.append("-resetState")
        }
        if !launchArguments.contains("-nabuBaseURL") {
            launchArguments += ["-nabuBaseURL", "http://localhost:8080"]
        }
        launch()
    }
}

/// Screen regressions for the review's dedicated reads and recovery controls.
/// The fixture responds inside the app process; no production server is used.
final class NabuReviewRecoveryUITests: XCTestCase {
    private func launch(_ scenario: String, accessibilitySize: Bool = false) -> XCUIApplication {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState", "-seedHomeForUITest", "-useMockAPI",
                               "-reviewScenario", scenario, "-nabuBaseURL", "http://localhost:9998",
                               "-reviewDate", "2026-09-10T12:34:00Z"]
        if accessibilitySize {
            app.launchArguments += ["-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityL"]
        }
        app.launchForLocalTest()
        XCTAssertTrue(app.tabBars.buttons["Home"].waitForExistence(timeout: 5))
        return app
    }

    /// Scrolls until the whole control is on screen. A control cut off by the
    /// screen edge still reports hittable, but its center, where a tap lands,
    /// can be off screen.
    private func reveal(_ element: XCUIElement, in app: XCUIApplication, requireHit: Bool = true) {
        let reachable = { element.exists && (!requireHit || (element.isHittable && app.frame.contains(element.frame))) }
        for upward in [true, false] {
            for _ in 0..<12 {
                if reachable() { return }
                if upward { app.swipeUp() } else { app.swipeDown() }
            }
        }
        XCTAssertTrue(reachable(), "Expected reachable control: \(element)")
    }

    /// Scrolls a sheet's form until `element` is fully below the sheet's header
    /// (its Cancel button). Focusing a field scrolls the form, which can leave a
    /// control above it under the header, where a tap lands on the header.
    private func revealBelowSheetHeader(_ element: XCUIElement, in app: XCUIApplication) {
        let header = app.buttons["Cancel"].firstMatch
        for _ in 0..<6 where element.frame.minY < header.frame.maxY + 4 {
            let start = element.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).withOffset(CGVector(dx: 0, dy: 100))
            start.press(forDuration: 0.05, thenDragTo: start.withOffset(CGVector(dx: 0, dy: 80)))
        }
        XCTAssertGreaterThanOrEqual(element.frame.minY, header.frame.maxY, "\(element) is under the sheet header")
    }

    /// Opens the log sheet's unit menu, first bringing it out from under the
    /// sheet header if a focused amount field scrolled the form.
    private func openUnitMenu(in app: XCUIApplication) {
        let picker = app.buttons["log-unit-picker"]
        revealBelowSheetHeader(picker, in: app)
        picker.tap()
    }

    /// The number wheel's value once its sheet has finished presenting; read
    /// any earlier and the wheel reports an empty value.
    private func expectWheel(_ value: String, in app: XCUIApplication, file: StaticString = #filePath, line: UInt = #line) {
        let wheel = app.pickerWheels.firstMatch
        XCTAssertTrue(wheel.waitForExistence(timeout: 5), file: file, line: line)
        let settled = XCTNSPredicateExpectation(predicate: NSPredicate(format: "value == %@", value), object: wheel)
        _ = XCTWaiter.wait(for: [settled], timeout: 5)
        XCTAssertEqual(wheel.value as? String, value, file: file, line: line)
    }

    private func waitForStableScreen(_ app: XCUIApplication) {
        var previous: Data?
        var matches = 0
        let stable = NSPredicate { _, _ in
            let current = app.screenshot().pngRepresentation
            matches = current == previous ? matches + 1 : 0
            previous = current
            return matches >= 2
        }
        let expectation = XCTNSPredicateExpectation(predicate: stable, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [expectation], timeout: 10), .completed,
                       "Screen did not settle before capture")
    }

    private func centerForCapture(_ element: XCUIElement, in app: XCUIApplication) {
        let target = app.frame.height * 0.45
        var dragAdjustment: CGFloat = 0
        for _ in 0..<8 {
            let before = element.frame.midY
            let remaining = target - before
            if abs(remaining) <= 1 { break }
            let drag = max(-app.frame.height * 0.3, min(app.frame.height * 0.3, remaining + dragAdjustment))
            let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.65))
            start.press(forDuration: 0.1, thenDragTo: start.withOffset(CGVector(dx: 0, dy: drag)),
                        withVelocity: .slow, thenHoldForDuration: 0.3)
            // Compensate for measured gesture slop instead of assuming a drag
            // moves the scroll content by exactly the requested distance.
            dragAdjustment = drag - (element.frame.midY - before)
        }
        XCTAssertEqual(element.frame.midY, target, accuracy: 1)
        waitForStableScreen(app)
    }

    private func adjustNumber(_ value: String, in wheel: XCUIElement) {
        // A long native wheel gesture can settle one or two rows beyond its
        // target. Correct from the observed row before confirming the choice.
        for _ in 0..<3 {
            wheel.adjust(toPickerWheelValue: value)
            if wheel.value as? String == value { break }
        }
        XCTAssertEqual(wheel.value as? String, value)
    }

    private func chooseNumber(_ value: String, from button: XCUIElement, in app: XCUIApplication) {
        reveal(button, in: app)
        button.tap()
        let wheel = app.pickerWheels.firstMatch
        XCTAssertTrue(wheel.waitForExistence(timeout: 5))
        adjustNumber(value, in: wheel)
        app.buttons["number-choice-done"].tap()
        XCTAssertTrue(wheel.waitForNonExistence(timeout: 5))
    }

    func testFeedBabyNumberChoicesCancelToggleAndSave() throws {
        let app = launch("midnight")
        openChore("Feed Baby", in: app)
        let formula = app.buttons["Choose Formula amount"]
        XCTAssertTrue(formula.waitForExistence(timeout: 5))
        let inputs = app.textFields.matching(identifier: "indicator-amount-input")
        XCTAssertEqual(inputs.firstMatch.value as? String, "120")
        formula.tap()
        let wheel = app.pickerWheels.firstMatch
        XCTAssertTrue(wheel.waitForExistence(timeout: 5))
        XCTAssertEqual(wheel.value as? String, "120 mL")
        adjustNumber("135 mL", in: wheel)
        try captureReviewScreen(app, named: "feed-number-wheel", element: wheel)
        app.buttons["number-choice-cancel"].tap()
        XCTAssertTrue(wheel.waitForNonExistence(timeout: 5))
        XCTAssertEqual(inputs.firstMatch.value as? String, "120")
        chooseNumber("135 mL", from: formula, in: app)
        XCTAssertEqual(inputs.firstMatch.value as? String, "135")
        reveal(app.buttons["Breast"], in: app)
        app.buttons["Breast"].tap()
        chooseNumber("30 mL", from: app.buttons["Choose Breast amount"], in: app)
        reveal(app.buttons["Formula"], in: app)
        app.buttons["Formula"].tap()
        XCTAssertFalse(formula.exists)
        app.buttons["Formula"].tap()
        XCTAssertEqual(inputs.firstMatch.value as? String, "135")
        XCTAssertEqual(inputs.element(boundBy: 1).value as? String, "30")
        try captureReviewScreen(app, named: "feed-number-fields")
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        // Reopening uses the saved latest log, including each type's amount.
        openChore("Feed Baby", in: app)
        reveal(app.buttons["Choose Breast amount"], in: app)
        XCTAssertEqual(inputs.firstMatch.value as? String, "135")
        XCTAssertEqual(inputs.element(boundBy: 1).value as? String, "30")
    }

    func testCustomNumberChoicesUnitsExactValuesAndEdit() throws {
        let app = launch("units")
        openChore("Weigh flour", in: app)
        let input = app.textFields["amount-input"]
        app.buttons["120 g"].tap()
        let choices = app.buttons["Choose amount"]
        choices.tap()
        expectWheel("120 g", in: app)
        app.buttons["number-choice-cancel"].tap()
        input.tap()
        input.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 3) + "12345")
        choices.tap()
        expectWheel("12345 g", in: app)
        app.buttons["number-choice-done"].tap()
        openUnitMenu(in: app)
        app.buttons["tablets"].tap()
        XCTAssertEqual(input.value as? String, "12345")
        chooseNumber("2 tablets", from: choices, in: app)
        XCTAssertEqual(input.value as? String, "2")
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Activity"].tap()
        let row = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "2 tablets")).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        row.tap()
        XCTAssertEqual(input.value as? String, "2")
        openUnitMenu(in: app)
        app.buttons["oz"].tap()
        chooseNumber("2.5 oz", from: choices, in: app)
        XCTAssertEqual(input.value as? String, "2.5")
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        let ounceRow = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "2.5 oz")).firstMatch
        XCTAssertTrue(ounceRow.waitForExistence(timeout: 5))
        ounceRow.tap()
        let exactText = input.value as? String ?? ""
        XCTAssertEqual((Double(exactText) ?? -1) * 29.5735, 74, accuracy: 0.03)
        choices.tap()
        expectWheel("\(exactText) oz", in: app)
        app.buttons["number-choice-cancel"].tap()
        chooseNumber("0 oz", from: choices, in: app)
        XCTAssertEqual(input.value as? String, "0")
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        let zeroRow = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "0 oz")).firstMatch
        XCTAssertTrue(zeroRow.waitForExistence(timeout: 5))
        zeroRow.tap()
        XCTAssertEqual(Double(input.value as? String ?? ""), 0)
        chooseNumber("No value", from: choices, in: app)
        XCTAssertEqual(input.value as? String, "Amount")
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        let emptyRow = app.staticTexts["Weigh flour"].firstMatch
        XCTAssertTrue(emptyRow.waitForExistence(timeout: 5))
        emptyRow.tap()
        XCTAssertEqual(input.value as? String, "Amount")
    }

    func testDurationNumberChoicesKeepExactSecondsAtLargeText() throws {
        let app = launch("timer-retry", accessibilitySize: true)
        openChore("Nap", in: app)
        let input = app.textFields["duration-seconds"]
        let choices = app.buttons["Choose duration"]
        chooseNumber("5 min", from: choices, in: app)
        XCTAssertEqual(input.value as? String, "300")
        input.tap()
        input.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 3) + "301")
        app.buttons["amount-keyboard-done"].tap()
        XCTAssertFalse(app.keyboards.firstMatch.exists)
        choices.tap()
        expectWheel("301 sec", in: app)
        try captureReviewScreen(app, named: "duration-number-wheel-large-text", element: app.pickerWheels.firstMatch)
        app.buttons["number-choice-done"].tap()
        XCTAssertEqual(input.value as? String, "301")
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Activity"].tap()
        let nap = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Nap")).firstMatch
        XCTAssertTrue(nap.waitForExistence(timeout: 5))
        nap.tap()
        reveal(input, in: app)
        XCTAssertEqual(input.value as? String, "301")
    }

    func testAmountUnitSelectorCancelAndSavedActivity() throws {
        let app = launch("units")
        openChore("Weigh flour", in: app)
        openUnitMenu(in: app)
        app.buttons["mg"].tap()
        app.buttons["Cancel"].tap()
        openChore("Weigh flour", in: app)
        XCTAssertEqual(app.buttons["log-unit-picker"].label, "Unit, g")
        openUnitMenu(in: app)
        app.buttons["mg"].tap()
        app.textFields["amount-input"].tap()
        app.textFields["amount-input"].typeText("200")
        openUnitMenu(in: app)
        app.buttons["L"].tap()
        XCTAssertEqual(app.textFields["amount-input"].value as? String, "200")
        app.buttons["amount-keyboard-done"].tap()
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Activity"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "200 L")).firstMatch.waitForExistence(timeout: 5))
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "200 L")).firstMatch.tap()
        XCTAssertTrue(app.buttons["log-unit-picker"].waitForExistence(timeout: 5))
        XCTAssertEqual(app.buttons["log-unit-picker"].label, "Unit, L")
        // Capture the saved unit in a freshly opened form. Keep the live
        // activity timestamp outside this unit-control snapshot.
        waitForStableScreen(app)
        try captureReviewScreen(app, named: "medication-unit-selector",
                                element: app.cells.containing(.button, identifier: "log-unit-picker").firstMatch)
        openUnitMenu(in: app)
        app.buttons["g"].tap()
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "200 g")).firstMatch.waitForExistence(timeout: 5))

    }

    func testGramEntryRecentChipAndFailedSaveRetainValuesForRetry() throws {
        let app = launch("amount")
        let chore = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Weigh flour")).firstMatch
        reveal(chore, in: app)
        chore.tap()
        XCTAssertTrue(app.buttons["120 g"].waitForExistence(timeout: 5))
        app.buttons["120 g"].tap()
        let amount = app.textFields["amount-input"]
        XCTAssertEqual(amount.value as? String, "120")
        try captureReviewScreen(app, named: "gram-recent")
        amount.tap()
        amount.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 3) + "37")
        app.buttons["amount-keyboard-done"].tap()
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        reveal(app.staticTexts["Could not save. Please retry."], in: app)
        XCTAssertTrue(app.staticTexts["Could not save. Please retry."].waitForExistence(timeout: 5))
        try captureReviewScreen(app, named: "gram-save-error")
        let form = app.collectionViews.firstMatch
        XCTAssertTrue(form.exists)
        for _ in 0..<8 {
            if amount.exists { break }
            form.swipeDown()
        }
        XCTAssertTrue(amount.exists)
        XCTAssertEqual(amount.value as? String, "37")
        XCTAssertFalse(amount.isEnabled)
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(chore.waitForExistence(timeout: 5))
        app.tabBars.buttons["Activity"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "37 g")).firstMatch.waitForExistence(timeout: 5))
    }

    func testNotificationOlderPageFailureRetainsReadRowsAndRetryWorks() throws {
        let app = launch("notifications", accessibilitySize: true)
        app.tabBars.buttons["Settings"].tap()
        let notifications = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Notifications")).firstMatch
        reveal(notifications, in: app)
        notifications.tap()
        XCTAssertTrue(app.staticTexts["Read notice 1"].waitForExistence(timeout: 5))
        try captureReviewScreen(app, named: "notification-history-large-text")
        let more = app.buttons["notifications-load-more"]
        reveal(more, in: app)
        more.tap()
        XCTAssertTrue(app.staticTexts["notification-error"].waitForExistence(timeout: 5))
        reveal(app.staticTexts["notification-error"], in: app)
        try captureReviewScreen(app, named: "notification-page-error-large-text")
        reveal(app.staticTexts["Read notice 1"], in: app)
        XCTAssertTrue(app.staticTexts["Read notice 1"].exists)
        reveal(more, in: app)
        more.tap()
        XCTAssertTrue(app.staticTexts["Older notice 3"].waitForExistence(timeout: 5))
        reveal(app.staticTexts["Older notice 3"], in: app)
        try captureReviewScreen(app, named: "notification-older-page-large-text")
        let mark = app.buttons["notification-mark-read-3"]
        reveal(mark, in: app)
        mark.tap()
        XCTAssertTrue(mark.waitForNonExistence(timeout: 5))
        app.staticTexts["Older notice 3"].swipeLeft()
        app.buttons["Delete"].tap()
        XCTAssertTrue(app.staticTexts["Older notice 3"].waitForNonExistence(timeout: 5))
        XCTAssertFalse(more.exists)
        app.buttons["notifications-mark-all-read"].tap()
        let readFinished = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
            !app.buttons["notifications-mark-all-read"].isEnabled && app.buttons["notifications-clear-all"].isEnabled
        }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [readFinished], timeout: 5), .completed)
        app.buttons["Refresh notifications"].tap()
        XCTAssertTrue(app.staticTexts["Read notice 1"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["notifications-mark-all-read"].isEnabled)
    }

    func testNotificationBulkActionsAreLargeAndClearAllRetries() throws {
        let app = launch("notifications")
        app.tabBars.buttons["Settings"].tap()
        let notifications = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Notifications")).firstMatch
        reveal(notifications, in: app)
        notifications.tap()
        XCTAssertTrue(app.staticTexts["Read notice 1"].waitForExistence(timeout: 5))
        let markAll = app.buttons["notifications-mark-all-read"]
        let clearAll = app.buttons["notifications-clear-all"]
        XCTAssertTrue(app.staticTexts["Notifications"].isHittable)
        XCTAssertTrue(markAll.isHittable)
        XCTAssertTrue(clearAll.isHittable)
        XCTAssertGreaterThanOrEqual(markAll.frame.height, 48)
        XCTAssertGreaterThanOrEqual(clearAll.frame.height, 48)
        XCTAssertEqual(markAll.frame.width, clearAll.frame.width, accuracy: 1)
        XCTAssertEqual(markAll.frame.height, clearAll.frame.height, accuracy: 1)
        try captureReviewScreen(app, named: "notification-bulk-actions")
        clearAll.tap()
        XCTAssertTrue(app.staticTexts["Could not clear notifications."].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Read notice 1"].exists)
        XCTAssertTrue(clearAll.isEnabled)
        markAll.tap()
        let readFinished = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in !markAll.isEnabled && clearAll.isEnabled }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [readFinished], timeout: 5), .completed)
        XCTAssertTrue(clearAll.isEnabled)
        clearAll.tap()
        XCTAssertTrue(clearAll.waitForNonExistence(timeout: 5))
        XCTAssertTrue(notifications.waitForExistence(timeout: 5))
        XCTAssertTrue(notifications.isHittable)
        notifications.tap()
        XCTAssertTrue(app.staticTexts["No notifications"].waitForExistence(timeout: 5))
        XCTAssertFalse(clearAll.isEnabled)
        XCTAssertFalse(app.buttons["notifications-load-more"].exists)
        XCTAssertFalse(markAll.isEnabled)
        XCTAssertFalse(clearAll.isEnabled)
        try captureReviewScreen(app, named: "notification-cleared")
    }

    func testExportRangeErrorCancelAndShareRecovery() throws {
        let app = launch("export")
        app.tabBars.buttons["Settings"].tap()
        let allDates = app.switches["All dates"]
        reveal(allDates, in: app)
        allDates.switches.firstMatch.tap()
        XCTAssertEqual(allDates.value as? String, "0")
        let startDate = app.descendants(matching: .any)["export-start-date"]
        let endDate = app.descendants(matching: .any)["export-end-date"]
        XCTAssertTrue(startDate.waitForExistence(timeout: 5))
        XCTAssertTrue(endDate.exists)
        XCTAssertTrue(startDate.isEnabled)
        XCTAssertTrue(endDate.isEnabled)
        reveal(endDate, in: app)
        centerForCapture(endDate, in: app)
        try captureReviewScreen(app, named: "export-date-range")
        let export = app.buttons["Export logs as CSV"]
        reveal(export, in: app)
        export.tap()
        let error = app.staticTexts["export-error"]
        reveal(error, in: app)
        XCTAssertTrue(error.waitForExistence(timeout: 5))
        try captureReviewScreen(app, named: "export-error")
        reveal(export, in: app)
        export.tap()
        let cancel = app.buttons["Cancel export"]
        reveal(cancel, in: app)
        XCTAssertTrue(cancel.waitForExistence(timeout: 5))
        XCTAssertFalse(startDate.isEnabled)
        try captureReviewScreen(app, named: "export-cancel")
        cancel.tap()
        XCTAssertTrue(cancel.waitForNonExistence(timeout: 5))
        reveal(allDates, in: app)
        XCTAssertTrue(allDates.isEnabled)
        reveal(export, in: app)
        export.tap()
        let share = app.otherElements["ActivityListView"]
        XCTAssertTrue(share.waitForExistence(timeout: 5))
        app.buttons["Close"].tap()
        XCTAssertTrue(share.waitForNonExistence(timeout: 5))
    }

    func testActivityErrorRetryAndSearchSurviveTabRoundtrip() {
        let app = launch("activity")
        app.tabBars.buttons["Activity"].tap()
        let retry = app.buttons["Retry"]
        XCTAssertTrue(retry.waitForExistence(timeout: 5))
        XCTAssertFalse(app.staticTexts["No activity yet"].exists)
        retry.tap()
        let result = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Needle result")).firstMatch
        XCTAssertTrue(result.waitForExistence(timeout: 5))
        let search = app.searchFields.firstMatch
        search.tap();search.typeText("Needle")
        app.tabBars.buttons["Home"].tap()
        app.tabBars.buttons["Activity"].tap()
        XCTAssertEqual(search.value as? String, "Needle")
        XCTAssertTrue(result.waitForExistence(timeout: 5))
    }

    func testStatsLoadingScreen() throws {
        let app = launch("stats-loading", accessibilitySize: true)
        app.tabBars.buttons["Stats"].tap()
        XCTAssertTrue(app.descendants(matching: .any)["stats-loading"].waitForExistence(timeout: 5))
        try captureReviewScreen(app, named: "stats-loading-large-text")
    }

    func testStatsErrorRetryRetainsEmptyControls() throws {
        let app = launch("stats", accessibilitySize: true)
        app.tabBars.buttons["Stats"].tap()
        let error = app.staticTexts["Stats couldn't be loaded. Try again."]
        XCTAssertTrue(error.waitForExistence(timeout: 8))
        XCTAssertTrue(app.buttons["stats-customize"].exists)
        try captureReviewScreen(app, named: "stats-error-large-text")
        app.buttons["Retry"].firstMatch.tap()
        XCTAssertTrue(error.waitForNonExistence(timeout: 5))
        XCTAssertEqual(app.staticTexts.matching(NSPredicate(format: "label == %@", "0")).count, 3)
        XCTAssertTrue(app.buttons["Log Your First Chore"].exists)
        try captureReviewScreen(app, named: "stats-recovered-large-text")
    }

    func testExportDateControlsAtLargeText() {
        let app = launch("export", accessibilitySize: true)
        app.tabBars.buttons["Settings"].tap()
        let allDates = app.switches["All dates"]
        reveal(allDates, in: app)
        allDates.switches.firstMatch.tap()
        let end = app.descendants(matching: .any)["export-end-date"]
        reveal(end, in: app)
        XCTAssertTrue(end.isEnabled)
        let export = app.buttons["Export logs as CSV"]
        reveal(export, in: app)
        XCTAssertTrue(export.isEnabled)
    }

    private func openChore(_ name: String, in app: XCUIApplication) {
        let chore = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
        reveal(chore, in: app)
        chore.tap()
    }

    func testCountEntryAtLargeTextAndActivityRoundtrip() throws {
        let app = launch("count", accessibilitySize: true)
        openChore("Count reps", in: app)
        let recent = app.buttons["12 reps"]
        reveal(recent, in: app)
        recent.tap()
        let amount = app.textFields["amount-input"]
        reveal(amount, in: app)
        XCTAssertEqual(amount.value as? String, "12")
        XCTAssertEqual(app.buttons["log-unit-picker"].label, "Unit, reps")
        XCTAssertFalse(app.textFields["indicator-amount-input"].exists)
        try captureReviewScreen(app, named: "count-large-text")
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Activity"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "12 reps")).firstMatch.waitForExistence(timeout: 5))
    }

    func testRecentReadEmptyAndFailureKeepAmountEntryAvailable() throws {
        for scenario in ["recent-empty", "recent-error"] {
            let app = launch(scenario)
            openChore("Weigh flour", in: app)
            let input = app.textFields["amount-input"]
            reveal(input, in: app)
            XCTAssertTrue(input.isEnabled)
            XCTAssertFalse(app.buttons["120 g"].exists)
            if scenario == "recent-error" { XCTAssertTrue(app.buttons["75 g"].exists) }
            waitForStableScreen(app)
            try captureReviewScreen(app, named: scenario)
            app.buttons["Cancel"].tap()
        }
    }

    private func rejectSavedRequest() -> XCUIApplication {
        let app = launch("journal")
        openChore("Weigh flour", in: app)
        let recent = app.buttons["120 g"]
        reveal(recent, in: app)
        recent.tap()
        let save = app.buttons["save-log-button"]
        reveal(save, in: app)
        save.tap()
        XCTAssertTrue(app.staticTexts["Could not save. Please retry."].waitForExistence(timeout: 5))
        app.buttons["Cancel"].tap()
        XCTAssertTrue(app.buttons["pending-saves"].waitForExistence(timeout: 5))
        return app
    }

    private func relaunchPreservingSavedState(_ app: XCUIApplication, scenario: String) {
        app.terminate()
        app.launchArguments.removeAll { $0 == "-resetState" }
        app.launchArguments.append("-preserveTestState")
        let index = app.launchArguments.firstIndex(of: "-reviewScenario")!
        app.launchArguments[index + 1] = scenario
        app.launchForLocalTest()
        XCTAssertTrue(app.tabBars.buttons["Home"].waitForExistence(timeout: 5))
    }

    func testRejectedJournalSurvivesRelaunchAndExplicitRetry() {
        let app = rejectSavedRequest()
        relaunchPreservingSavedState(app, scenario: "journal-retry")
        let pending = app.buttons["pending-saves"]
        XCTAssertTrue(pending.waitForExistence(timeout: 5))
        pending.tap()
        XCTAssertTrue(app.navigationBars["Pending saves"].waitForExistence(timeout: 5))
        app.buttons["Retry saves"].tap()
        XCTAssertTrue(pending.waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Activity"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "120 g")).firstMatch.waitForExistence(timeout: 5))
    }

    func testDiscardRejectedJournalPersistsAcrossRelaunch() {
        let app = rejectSavedRequest()
        app.buttons["pending-saves"].tap()
        app.buttons["Discard saved request"].tap()
        app.buttons["Discard"].tap()
        XCTAssertTrue(app.buttons["pending-saves"].waitForNonExistence(timeout: 5))
        relaunchPreservingSavedState(app, scenario: "journal-retry")
        XCTAssertFalse(app.buttons["pending-saves"].exists)
    }

    func testStoppedTimerSurvivesRelaunchAndRetriesSave() {
        let app = launch("timer")
        openChore("Nap", in: app)
        let start = app.buttons["start-timer-button"]
        reveal(start, in: app)
        start.tap()
        let timer = app.buttons["timer-chip"]
        XCTAssertTrue(timer.waitForExistence(timeout: 5))
        timer.tap()
        let retry = app.buttons["Retry saving Nap"]
        XCTAssertTrue(app.staticTexts["Could not save. Please retry."].waitForExistence(timeout: 5))
        XCTAssertTrue(retry.waitForExistence(timeout: 5))
        XCTAssertTrue(retry.isEnabled)
        relaunchPreservingSavedState(app, scenario: "timer-retry")
        XCTAssertTrue(retry.waitForExistence(timeout: 5))
        retry.tap()
        XCTAssertTrue(timer.waitForNonExistence(timeout: 5))
        relaunchPreservingSavedState(app, scenario: "timer-retry")
        XCTAssertFalse(timer.exists)
        XCTAssertFalse(app.buttons["pending-saves"].exists)
    }

    func testPreviousDayVolumePrefillExcludesUnselectedType() {
        let app = launch("midnight")
        openChore("Feed Baby", in: app)
        let volumes = app.textFields.matching(identifier: "indicator-amount-input")
        XCTAssertEqual(volumes.count, 1)
        XCTAssertEqual(volumes.firstMatch.value as? String, "120")
        reveal(app.buttons["Breast"], in: app)
        app.buttons["Breast"].tap()
        XCTAssertEqual(volumes.count, 2)
        XCTAssertEqual(volumes.element(boundBy: 1).value as? String, "Amount")
        app.buttons["Cancel"].tap()
        openChore("Feed Baby", in: app)
        XCTAssertEqual(volumes.count, 1)
        XCTAssertEqual(volumes.firstMatch.value as? String, "120")
    }

    func testRecurringSchedulePersistsThroughReloadAndEditor() throws {
        let app = launch("schedule")
        app.tabBars.buttons["Schedule"].tap()
        app.buttons["empty-schedule-create"].tap()
        let frequency = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Frequency,")).firstMatch
        frequency.tap()
        app.buttons["Every day"].tap()
        let repeatThrough = app.switches["Repeat through (inclusive)"]
        repeatThrough.switches.firstMatch.tap()
        XCTAssertEqual(repeatThrough.value as? String, "1")
        XCTAssertTrue(app.staticTexts["End date"].exists)
        let end = app.datePickers.firstMatch.buttons["Date Picker"]
        XCTAssertTrue(end.exists)
        let endValue = try XCTUnwrap(end.value as? String)
        let chore = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Feed Cats")).firstMatch
        reveal(chore, in: app)
        chore.tap()
        XCTAssertTrue(app.navigationBars["Add to Schedule"].waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Home"].tap()
        app.tabBars.buttons["Schedule"].tap()
        let row = app.staticTexts["Feed Cats"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        let recurrence = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Every day")).firstMatch
        XCTAssertTrue(recurrence.waitForExistence(timeout: 5))
        XCTAssertTrue(recurrence.label.contains("until "))
        let withoutEnd = try XCTUnwrap(recurrence.label.components(separatedBy: " · until ").first)
        row.press(forDuration: 1)
        try XCTUnwrap(app.buttons.matching(identifier: "Edit").allElementsBoundByIndex.first(where: { $0.isHittable })).tap()
        XCTAssertTrue(app.navigationBars["Edit Schedule"].waitForExistence(timeout: 5))
        XCTAssertTrue(frequency.label.contains("Every day"))
        XCTAssertEqual(app.switches["Repeat through (inclusive)"].value as? String, "1")
        XCTAssertEqual(try XCTUnwrap(end.value as? String), endValue)
        app.switches["Repeat through (inclusive)"].switches.firstMatch.tap()
        app.buttons["Save"].tap()
        XCTAssertTrue(app.navigationBars["Edit Schedule"].waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Home"].tap()
        app.tabBars.buttons["Schedule"].tap()
        XCTAssertTrue(app.staticTexts[withoutEnd].firstMatch.waitForExistence(timeout: 5))
        row.press(forDuration: 1)
        try XCTUnwrap(app.buttons.matching(identifier: "Edit").allElementsBoundByIndex.first(where: { $0.isHittable })).tap()
        XCTAssertTrue(app.navigationBars["Edit Schedule"].waitForExistence(timeout: 5))
        XCTAssertEqual(app.switches["Repeat through (inclusive)"].value as? String, "0")
        XCTAssertFalse(end.exists)
        app.buttons["Cancel"].tap()
    }

    func testOwnershipDeletionFailureCanCancelAndReopenCleanly() throws {
        let app = launch("delete-account")
        app.tabBars.buttons["Settings"].tap()
        let open = app.buttons["Delete Account…"]
        reveal(open, in: app); open.tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "transfer ownership (or remove")).firstMatch.exists)
        let confirm = app.textFields["Type DELETE to confirm"]
        let remove = app.buttons["Delete My Account"]
        // The data download and opt-in sections sit above the confirmation.
        reveal(remove, in: app)
        XCTAssertFalse(remove.isEnabled)
        reveal(confirm, in: app)
        confirm.tap(); confirm.typeText("DELETE")
        remove.tap()
        let error = app.staticTexts["Transfer ownership before deleting your account."]
        XCTAssertTrue(error.waitForExistence(timeout: 5))
        reveal(error, in: app)
        try captureReviewScreen(app, named: "deletion-ownership-error")
        app.buttons["Cancel"].tap()
        XCTAssertTrue(open.waitForExistence(timeout: 5))
        open.tap()
        reveal(remove, in: app)
        XCTAssertFalse(remove.isEnabled)
        reveal(confirm, in: app)
        XCTAssertEqual(confirm.value as? String, "Type DELETE to confirm")
        XCTAssertFalse(error.exists)
        app.buttons["Cancel"].tap()
    }

    func testDeletionExplainsFormerMemberAndSendsOptIns() {
        let app = launch("delete-account-options")
        app.tabBars.buttons["Settings"].tap()
        let open = app.buttons["Delete Account…"]
        reveal(open, in: app); open.tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Former member")).firstMatch.waitForExistence(timeout: 5))
        let logs = app.switches["delete-account-logs"]
        let notes = app.switches["delete-account-notes"]
        XCTAssertEqual(logs.value as? String, "0")
        XCTAssertEqual(notes.value as? String, "0")

        let download = app.buttons["download-my-data"]
        download.tap()
        XCTAssertTrue(app.otherElements["export-share-sheet"].waitForExistence(timeout: 5)
            || app.otherElements["ActivityListView"].waitForExistence(timeout: 5))
        if app.buttons["Close"].exists { app.buttons["Close"].tap() } else { app.swipeDown() }

        reveal(logs, in: app)
        (logs.switches.firstMatch.exists ? logs.switches.firstMatch : logs).tap()
        XCTAssertEqual(logs.value as? String, "1")
        let confirm = app.textFields["Type DELETE to confirm"]
        reveal(confirm, in: app)
        confirm.tap(); confirm.typeText("DELETE")
        let remove = app.buttons["Delete My Account"]
        reveal(remove, in: app)
        remove.tap()
        let echoed = app.staticTexts["Received deleteLogs=true deleteNotes=false"]
        XCTAssertTrue(echoed.waitForExistence(timeout: 5))
        app.buttons["Cancel"].tap()
    }

    func testStatsShowsNewHouseholdAfterSwitch() {
        let app = launch("stats-switch")
        app.tabBars.buttons["Stats"].tap()
        let prior = app.staticTexts["This week you completed 13 chores."]
        reveal(prior, in: app)
        XCTAssertTrue(prior.waitForExistence(timeout: 5))
        app.tabBars.buttons["Settings"].tap()
        let switchHousehold = app.buttons["Switch"]
        reveal(switchHousehold, in: app)
        switchHousehold.tap()
        XCTAssertTrue(app.staticTexts["No chores yet"].waitForExistence(timeout: 5))
        app.tabBars.buttons["Stats"].tap()
        let current = app.staticTexts["This week you completed 84 chores."]
        reveal(current, in: app)
        XCTAssertTrue(current.waitForExistence(timeout: 5))
        XCTAssertFalse(prior.exists)
    }
}

final class NabuPasswordSetupUITests: XCTestCase {
    func testClaimedAccountCanSetPasswordAndReopenChangeForm() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI",
                               "-passwordlessAccount", "-nabuBaseURL", "http://localhost:9998"]
        app.launchForLocalTest()
        XCTAssertTrue(app.tabBars.buttons["Settings"].waitForExistence(timeout: 5))
        app.tabBars.buttons["Settings"].tap()
        app.buttons["Set Password"].tap()
        let password = app.secureTextFields["New Password"]
        XCTAssertTrue(password.waitForExistence(timeout: 5))
        password.tap(); password.typeText("owner-password-123")
        let confirmation = app.secureTextFields["Confirm New Password"]
        confirmation.tap(); confirmation.typeText("owner-password-123")
        app.buttons["Save"].tap()
        XCTAssertTrue(password.waitForNonExistence(timeout: 5))
        app.tabBars.buttons["Settings"].tap()
        XCTAssertTrue(app.buttons["Change Password"].waitForExistence(timeout: 5))
        app.buttons["Change Password"].tap()
        XCTAssertTrue(app.secureTextFields["Current Password"].waitForExistence(timeout: 5))
        app.buttons["Cancel"].tap()
    }

    func testClaimedAccountCanOpenAndCancelPasswordSetup() {
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI",
                               "-passwordlessAccount", "-nabuBaseURL", "http://localhost:8080"]
        app.launchForLocalTest()
        XCTAssertTrue(app.tabBars.buttons["Settings"].waitForExistence(timeout: 5))
        app.tabBars.buttons["Settings"].tap()
        XCTAssertTrue(app.buttons["Set Password"].waitForExistence(timeout: 5))
        app.buttons["Set Password"].tap()
        XCTAssertTrue(app.secureTextFields["New Password"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.secureTextFields["Current Password"].exists)
        app.buttons["Cancel"].tap()
        XCTAssertFalse(app.secureTextFields["New Password"].exists)
        XCTAssertTrue(app.buttons["Set Password"].exists)
    }
}

final class NabuScheduleEmptyStateUITests: XCTestCase {
    func testEmptyScheduleActionOpensPickerAndCancelReturns() throws {
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState", "-seedHomeForUITest", "-useMockAPI",
                               "-nabuBaseURL", "http://localhost:8080"]
        app.launchForLocalTest()
        XCTAssertTrue(app.tabBars.buttons["Schedule"].waitForExistence(timeout: 5))
        app.tabBars.buttons["Schedule"].tap()
        let create = app.buttons["empty-schedule-create"]
        XCTAssertTrue(create.waitForExistence(timeout: 5))
        try captureReviewScreen(app, named: "empty-schedule")
        create.tap()
        XCTAssertTrue(app.navigationBars["Add to Schedule"].waitForExistence(timeout: 5))
        app.buttons["Cancel"].tap()
        XCTAssertTrue(create.waitForExistence(timeout: 5))
    }
}

// MARK: - Auth Flow Tests

final class NabuUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState"]
        app.launchForLocalTest()
    }

    func testLoginFormAppears() throws {
        XCTAssertTrue(app.staticTexts["Nabu"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Sign In"].exists)
        XCTAssertTrue(app.buttons["Create Account"].exists)
        XCTAssertTrue(app.buttons["Sign in with magic link"].exists)
    }

    func testNavigateToRegister() throws {
        XCTAssertTrue(app.buttons["Create Account"].waitForExistence(timeout: 5))
        app.buttons["Create Account"].tap()
        XCTAssertTrue(app.staticTexts["Create Account"].exists)
        XCTAssertTrue(app.secureTextFields["Password (min 8 characters)"].exists)
        XCTAssertTrue(app.secureTextFields["Confirm Password"].exists)
    }

    // P5 exit gate: the Sign in with Apple button renders on both auth
    // screens (App Store guideline 4.8).
    func testSignInWithAppleButtonOnLogin() throws {
        XCTAssertTrue(app.buttons["siwa-button"].waitForExistence(timeout: 5))
    }

    func testSignInWithAppleButtonOnRegister() throws {
        XCTAssertTrue(app.buttons["Create Account"].waitForExistence(timeout: 5))
        app.buttons["Create Account"].tap()
        XCTAssertTrue(app.buttons["siwa-button"].waitForExistence(timeout: 5))
    }

    func testNavigateToMagicLink() throws {
        XCTAssertTrue(app.buttons["Sign in with magic link"].waitForExistence(timeout: 5))
        app.buttons["Sign in with magic link"].tap()
        XCTAssertTrue(app.staticTexts["Magic Link"].exists)
        XCTAssertTrue(app.buttons["Send Magic Link"].exists)
    }

    func testNavigateToForgotPassword() throws {
        XCTAssertTrue(app.buttons["Forgot password?"].waitForExistence(timeout: 5))
        app.buttons["Forgot password?"].tap()
        XCTAssertTrue(app.staticTexts["Forgot Password"].exists)
        XCTAssertTrue(app.buttons["Send Reset Link"].exists)
    }

    func testLoginButtonDisabledWhenEmpty() throws {
        XCTAssertTrue(app.buttons["Sign In"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["Sign In"].isEnabled)
    }

    func testRegisterPasswordMismatch() throws {
        app.buttons["Create Account"].tap()
        XCTAssertTrue(app.staticTexts["Create Account"].waitForExistence(timeout: 5))

        let emailField = app.textFields.firstMatch
        emailField.tap()
        emailField.typeText("test@test.com")

        let passwordFields = app.secureTextFields
        passwordFields.firstMatch.tap()
        passwordFields.firstMatch.typeText("password123")

        passwordFields.element(boundBy: 1).tap()
        passwordFields.element(boundBy: 1).typeText("different")

        let createButton = app.buttons["Create Account"]
        XCTAssertFalse(createButton.isEnabled)
    }
}

// MARK: - Home Grid Rendering Tests

/// Tests that verify the home grid renders correctly with seeded data.
/// Uses -seedHomeForUITest to inject 5 chores with mixed log states.
final class NabuHomeGridUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest"]
        app.launchForLocalTest()
    }

    func testGridShowsAllSeededChores() throws {
        XCTAssertTrue(cell(named: "Feed Cats").waitForExistence(timeout: 5))
        XCTAssertTrue(cell(named: "Walk Dog").exists)
        XCTAssertTrue(cell(named: "Water Plants").exists)
        XCTAssertTrue(cell(named: "Feed Baby").exists)
        XCTAssertTrue(cell(named: "Take Vitamins").exists)
    }

    func testDoneChoresShowTimeAgo() throws {
        let feedCats = cell(named: "Feed Cats")
        XCTAssertTrue(feedCats.waitForExistence(timeout: 5))
        XCTAssertTrue(feedCats.label.contains("done "), "Feed Cats: \(feedCats.label)")
        XCTAssertFalse(feedCats.label.contains("never done"), "Feed Cats: \(feedCats.label)")

        let feedBaby = cell(named: "Feed Baby")
        XCTAssertTrue(feedBaby.exists)
        XCTAssertTrue(feedBaby.label.contains("done "), "Feed Baby: \(feedBaby.label)")

        let takeVitamins = cell(named: "Take Vitamins")
        XCTAssertTrue(takeVitamins.exists)
        XCTAssertTrue(takeVitamins.label.contains("done "), "Take Vitamins: \(takeVitamins.label)")
    }

    func testUndoneChoresShowNeverDone() throws {
        let walkDog = cell(named: "Walk Dog")
        XCTAssertTrue(walkDog.waitForExistence(timeout: 5))
        XCTAssertTrue(walkDog.label.contains("never done"), "Walk Dog: \(walkDog.label)")

        let waterPlants = cell(named: "Water Plants")
        XCTAssertTrue(waterPlants.exists)
        XCTAssertTrue(waterPlants.label.contains("never done"), "Water Plants: \(waterPlants.label)")
    }

    func testChoreTapOpensLogSheet() throws {
        let choreCell = cell(named: "Walk Dog")
        XCTAssertTrue(choreCell.waitForExistence(timeout: 5))
        choreCell.tap()

        XCTAssertTrue(app.buttons["Cancel"].waitForExistence(timeout: 3), "Cancel button must appear")
        XCTAssertTrue(app.staticTexts["🐕 Walk Dog"].exists, "LogSheet title must show chore")
    }

    // MARK: - Helpers

    private func cell(named name: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
    }
}

// MARK: - Log Sheet Form Tests

/// Tests the log sheet form fields: when picker, indicators, volume, note, Cancel.
/// Uses -useMockAPI so save attempts don't require a real server.
final class NabuHomeLogSheetUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI"]
        app.launchForLocalTest()
    }

    func testCancelDismissesSheet() throws {
        let choreCell = cell(named: "Water Plants")
        XCTAssertTrue(choreCell.waitForExistence(timeout: 5))
        choreCell.tap()

        let cancelButton = app.buttons["Cancel"]
        XCTAssertTrue(cancelButton.waitForExistence(timeout: 3))
        cancelButton.tap()

        XCTAssertTrue(choreCell.waitForExistence(timeout: 3), "Grid should be visible after dismiss")
    }

    func testWhenPickerVisible() throws {
        openLogSheet(forChore: "Walk Dog")
        XCTAssertTrue(app.datePickers["when-picker"].waitForExistence(timeout: 3),
            "When picker must be visible in log sheet")
    }

    func testLogButtonEnabled() throws {
        openLogSheet(forChore: "Water Plants")
        let logButton = app.buttons["save-log-button"]
        XCTAssertTrue(logButton.waitForExistence(timeout: 3))
        XCTAssertTrue(logButton.isEnabled)
    }

    func testNoteFieldExists() throws {
        openLogSheet(forChore: "Water Plants")
        // TextField with axis: .vertical may render as a text view.
        let noteField = app.textFields["Add a note..."]
        let noteView = app.textViews["Add a note..."]
        XCTAssertTrue(noteField.waitForExistence(timeout: 3) || noteView.waitForExistence(timeout: 3),
            "Note field must be present")
    }

    // MARK: - Indicator chips

    func testIndicatorChipsAppear() throws {
        openLogSheet(forChore: "Walk Dog")
        XCTAssertTrue(app.buttons["Short"].waitForExistence(timeout: 3))
        XCTAssertTrue(app.buttons["Long"].exists)
        XCTAssertTrue(app.buttons["Park"].exists)
    }

    func testNoIndicatorsForSimpleChore() throws {
        openLogSheet(forChore: "Feed Cats")
        XCTAssertFalse(app.buttons["Short"].exists)
        XCTAssertFalse(app.buttons["Formula"].exists)
    }

    // MARK: - Volume picker

    func testVolumePickerForFeedBaby() throws {
        openLogSheet(forChore: "Feed Baby")
        XCTAssertTrue(app.textFields["indicator-amount-input"].waitForExistence(timeout: 3))
    }

    func testNoVolumePickerForSimpleChore() throws {
        openLogSheet(forChore: "Feed Cats")
        XCTAssertFalse(app.textFields["indicator-amount-input"].exists)
    }

    // MARK: - Helpers

    private func cell(named name: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
    }

    private func openLogSheet(forChore name: String) {
        let c = cell(named: name)
        XCTAssertTrue(c.waitForExistence(timeout: 5), "Chore '\(name)' not found")
        c.tap()
    }
}

// MARK: - Log Save Flow Tests

/// Tests saving a log via the log sheet and verifying the UI updates.
/// Uses -useMockAPI to intercept API calls.
final class NabuHomeLogFlowUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI"]
        app.launchForLocalTest()
    }

    /// Log a chore that had no prior log; verify time-ago updates on the grid.
    func testLogChoreUpdatesTimeAgo() throws {
        let cell = cell(named: "Water Plants")
        XCTAssertTrue(cell.waitForExistence(timeout: 5))
        XCTAssertTrue(cell.label.contains("never done"), "Precondition: Water Plants should be 'never done'")

        openLogSheet(forChore: "Water Plants")
        saveLog()

        // Sheet should dismiss; grid cell should reappear with updated label.
        XCTAssertTrue(cell.waitForExistence(timeout: 5))

        let updatedLabel = cell.label
        XCTAssertTrue(updatedLabel.contains("done "), "After logging, should show 'done': \(updatedLabel)")
        XCTAssertFalse(updatedLabel.contains("never done"), "Should not say 'never done': \(updatedLabel)")
    }

    /// Log a chore with indicator chips selected.
    func testLogWithIndicators() throws {
        openLogSheet(forChore: "Walk Dog")

        let parkButton = app.buttons["Park"]
        XCTAssertTrue(parkButton.waitForExistence(timeout: 3))
        parkButton.tap()

        saveLog()

        // Verify sheet dismissed and grid is visible.
        XCTAssertTrue(cell(named: "Walk Dog").waitForExistence(timeout: 5))
    }

    /// Log Feed Baby (which has volume picker).
    func testLogFeedBabySucceeds() throws {
        openLogSheet(forChore: "Feed Baby")

        let volumeButton = app.textFields["indicator-amount-input"]
        XCTAssertTrue(volumeButton.waitForExistence(timeout: 3))
        let recentAmount = app.buttons["120 mL"]
        for _ in 0..<8 {
            if recentAmount.exists && recentAmount.isHittable { break }
            app.swipeUp()
        }
        XCTAssertTrue(recentAmount.exists && recentAmount.isHittable)
        recentAmount.tap()
        XCTAssertEqual(volumeButton.value as? String, "120")

        saveLog()

        XCTAssertTrue(cell(named: "Feed Baby").waitForExistence(timeout: 5))
    }

    /// Every new log shows an undo toast; Undo deletes it and restores the
    /// tile's previous "time ago" (here: never done).
    func testNewLogShowsUndoToastAndUndoRestoresTile() throws {
        let tile = cell(named: "Water Plants")
        XCTAssertTrue(tile.waitForExistence(timeout: 5))
        XCTAssertTrue(tile.label.contains("never done"), "Precondition: \(tile.label)")

        openLogSheet(forChore: "Water Plants")
        saveLog()

        XCTAssertTrue(app.staticTexts["Logged Water Plants"].waitForExistence(timeout: 3), "undo toast must appear after a new log")
        XCTAssertTrue(tile.label.contains("done "), "After logging: \(tile.label)")
        app.buttons["Undo"].tap()

        let restored = NSPredicate(format: "label CONTAINS %@", "never done")
        expectation(for: restored, evaluatedWith: tile)
        waitForExpectations(timeout: 5)
        XCTAssertFalse(app.staticTexts["Logged Water Plants"].exists)
    }

    /// The Log button is pinned above the safe area: reachable without
    /// scrolling at the largest accessibility text size, even for a chore
    /// with amount and type fields.
    func testLogButtonReachableAtAccessibilityXXXL() throws {
        app.terminate()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI",
                               "-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL"]
        app.launchForLocalTest()
        let tile = cell(named: "Feed Baby")
        for _ in 0..<8 where !(tile.exists && tile.isHittable) { app.swipeUp() }
        tile.tap()

        let save = app.buttons["save-log-button"]
        XCTAssertTrue(save.waitForExistence(timeout: 3))
        XCTAssertTrue(save.isHittable, "Log must be reachable at the default detent without scrolling")
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5), "the log must save and dismiss the sheet")
    }

    // MARK: - Helpers

    private func cell(named name: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
    }

    private func openLogSheet(forChore name: String) {
        let c = cell(named: name)
        XCTAssertTrue(c.waitForExistence(timeout: 5))
        c.tap()
    }

    private func saveLog() {
        let save = app.buttons["save-log-button"]
        for _ in 0..<8 {
            if save.exists && save.isHittable { break }
            app.swipeUp()
        }
        XCTAssertTrue(save.exists && save.isHittable)
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
    }
}

// MARK: - Quick Log Sheet Tests

/// Tests the quick-log sheet (accessed via the + button in the toolbar).
final class NabuHomeQuickLogUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI"]
        app.launchForLocalTest()
    }

    func testQuickLogSheetOpens() throws {
        let quickLogButton = app.buttons["quick-log-button"]
        XCTAssertTrue(quickLogButton.waitForExistence(timeout: 5))
        quickLogButton.tap()

        XCTAssertTrue(app.staticTexts["Quick Log"].waitForExistence(timeout: 3))
        XCTAssertTrue(app.buttons["Cancel"].exists)
    }

    func testQuickLogListsAllChores() throws {
        let quickLogButton = app.buttons["quick-log-button"]
        XCTAssertTrue(quickLogButton.waitForExistence(timeout: 5))
        quickLogButton.tap()

        XCTAssertTrue(app.staticTexts["Quick Log"].waitForExistence(timeout: 3))
        // All 5 chores (none hidden) should appear as buttons.
        // Buttons contain icon + name text.
        XCTAssertTrue(app.buttons.containing(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch.waitForExistence(timeout: 3))
        XCTAssertTrue(app.buttons.containing(NSPredicate(format: "label CONTAINS %@", "Feed Cats")).firstMatch.exists)
        XCTAssertTrue(app.buttons.containing(NSPredicate(format: "label CONTAINS %@", "Walk Dog")).firstMatch.exists)
        XCTAssertTrue(app.buttons.containing(NSPredicate(format: "label CONTAINS %@", "Feed Baby")).firstMatch.exists)
        XCTAssertTrue(app.buttons.containing(NSPredicate(format: "label CONTAINS %@", "Take Vitamins")).firstMatch.exists)
    }

    func testQuickLogCancelDismisses() throws {
        let quickLogButton = app.buttons["quick-log-button"]
        XCTAssertTrue(quickLogButton.waitForExistence(timeout: 5))
        quickLogButton.tap()

        XCTAssertTrue(app.buttons["Cancel"].waitForExistence(timeout: 3))
        app.buttons["Cancel"].tap()

        XCTAssertTrue(quickLogButton.waitForExistence(timeout: 3))
    }

    func testQuickLogTappingChoreDismisses() throws {
        let quickLogButton = app.buttons["quick-log-button"]
        XCTAssertTrue(quickLogButton.waitForExistence(timeout: 5))
        quickLogButton.tap()

        // Find a chore button and tap it.
        let choreButton = app.buttons.containing(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch
        XCTAssertTrue(choreButton.waitForExistence(timeout: 3))
        choreButton.tap()

        // Quick log should dismiss after the API call.
        // The grid should be visible again.
        let cell = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Water Plants")).firstMatch
        XCTAssertTrue(cell.waitForExistence(timeout: 5), "Grid should reappear after quick log")
    }
}

// MARK: - Jiggle Mode Tests

/// Tests the jiggle mode toggle (pencil/checkmark button in toolbar).
final class NabuHomeJiggleUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest"]
        app.launchForLocalTest()
    }

    func testJiggleModeToggle() throws {
        XCTAssertTrue(cell(named: "Feed Cats").waitForExistence(timeout: 5))

        let jiggleButton = app.buttons["jiggle-button"]
        XCTAssertTrue(jiggleButton.exists)

        // Enter jiggle mode (icon changes to checkmark).
        jiggleButton.tap()
        XCTAssertTrue(jiggleButton.exists)

        // Exit jiggle mode.
        jiggleButton.tap()
        XCTAssertTrue(jiggleButton.exists)

        // Grid should still be visible.
        XCTAssertTrue(cell(named: "Feed Cats").exists)
    }

    private func cell(named name: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
    }
}

// MARK: - Manage View Tests

/// Tests the Manage tab view for chore management.
final class NabuHomeManageUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI"]
        app.launchForLocalTest()
    }

    func testManageTabShowsChores() throws {
        XCTAssertTrue(cell(named: "Feed Cats").waitForExistence(timeout: 5))

        app.buttons["Manage"].tap()

        // All 5 chores should appear as static text rows.
        XCTAssertTrue(app.staticTexts["Feed Cats"].waitForExistence(timeout: 3))
        XCTAssertTrue(app.staticTexts["Walk Dog"].exists)
        XCTAssertTrue(app.staticTexts["Water Plants"].exists)
        XCTAssertTrue(app.staticTexts["Feed Baby"].exists)
        XCTAssertTrue(app.staticTexts["Take Vitamins"].exists)

        // All seeded chores are predefined → 5 "Default" badges.
        let defaultBadges = app.staticTexts.matching(NSPredicate(format: "label == %@", "Default"))
        XCTAssertEqual(defaultBadges.count, 5)
    }

    func testSwitchBackToLogTab() throws {
        XCTAssertTrue(cell(named: "Feed Cats").waitForExistence(timeout: 5))

        app.buttons["Manage"].tap()
        XCTAssertTrue(app.staticTexts["Feed Cats"].waitForExistence(timeout: 3))

        app.buttons["Log"].tap()
        XCTAssertTrue(cell(named: "Feed Cats").waitForExistence(timeout: 3))
    }

    func testManageViewHasPillTabs() throws {
        XCTAssertTrue(cell(named: "Feed Cats").waitForExistence(timeout: 5))

        // Both "Log" and "Manage" pill tab buttons should exist.
        XCTAssertTrue(app.buttons["Log"].exists)
        XCTAssertTrue(app.buttons["Manage"].exists)
    }

    private func cell(named name: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
    }
}

// MARK: - End-to-End Real Server Tests

/// Runs against a real server at http://localhost:8080.
/// Requires `go run ./cmd/server` (in-memory) or `make local` (Postgres).
final class NabuHomeEndToEndUITests: XCTestCase {
    var app: XCUIApplication!
    var email: String = ""
    let password = "test123456"

    override func setUpWithError() throws {
        continueAfterFailure = false
        email = "e2e-ios-\(Int(Date().timeIntervalSince1970))-\(Int.random(in: 0...9999))@nabu.local"
        app = XCUIApplication()
    }

    /// Full flow: auto-register → create household → seed defaults →
    /// wait for home grid → tap chore → log → verify time-ago updates.
    func testFullRegisterToLogFlow() {
        app.launchArguments = [
            "-disableAnimations",
            "-resetState",
            "-nabuBaseURL", e2eServerURL,
            "-nabuAutoRegister", email, password,
        ]
        app.launchForLocalTest()

        // 1. Wait for home grid to load — look for chore text in the home grid.
        // The HomeGrid accessibilityLabel includes chore name.
        let waterPlantsText = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch
        XCTAssertTrue(waterPlantsText.waitForExistence(timeout: 30), "Home grid should appear with seeded chores")

        // 2. Tap the Water Plants chore button.
        waterPlantsText.tap()

        // 3. Log the chore via the sheet.
        XCTAssertTrue(app.buttons["save-log-button"].waitForExistence(timeout: 5))
        app.buttons["save-log-button"].tap()

        // 4. Verify sheet dismissed and time-ago updated.
        let stillWaterPlants = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch
        XCTAssertTrue(stillWaterPlants.waitForExistence(timeout: 5))
        let updatedLabel = stillWaterPlants.label
        XCTAssertTrue(updatedLabel.contains("done"), "After logging, should show 'done': \(updatedLabel)")
        XCTAssertFalse(updatedLabel.contains("never done"), "Should not say 'never done': \(updatedLabel)")
    }
}

// MARK: - Offline launch (real server)

/// The iOS counterpart of `tests/e2e/offline-launch.spec.js`: a cold launch
/// that cannot reach the server opens the Home saved for the signed-in
/// account, a new log waits on the device, and it syncs once a later launch
/// confirms the session. `-serverUnreachable` fails every request the way a
/// missing connection does.
final class NabuOfflineLaunchUITests: XCTestCase {
    /// `http://localhost:8080` like the other real-server tests; run with
    /// `TEST_RUNNER_NABU_E2E_BASE_URL=<url>` to use another local server.
    private let server = ProcessInfo.processInfo.environment["NABU_E2E_BASE_URL"] ?? "http://localhost:8080"

    func testOfflineLaunchOpensSavedHomeAndQueuedLogSyncs() {
        continueAfterFailure = false
        let email = "offline-ios-\(Int(Date().timeIntervalSince1970))-\(Int.random(in: 0...9999))@nabu.local"
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState", "-nabuBaseURL", server,
                               "-nabuAutoRegister", email, "test123456"]
        app.launchForLocalTest()
        let water = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch
        XCTAssertTrue(water.waitForExistence(timeout: 30), "Home should load online first")
        XCTAssertTrue(water.label.contains("never done"), water.label)
        // Leave the app as a person would, so the session cookie is stored.
        XCUIDevice.shared.press(.home)
        sleep(2)
        app.terminate()

        app.launchArguments = ["-disableAnimations", "-serverUnreachable", "-nabuBaseURL", server]
        app.launch()
        XCTAssertTrue(water.waitForExistence(timeout: 15), "Saved Home should open without the server")
        let banners = app.descendants(matching: .any).matching(identifier: "offline-banner")
        let savedHomeBanner = banners.matching(NSPredicate(format: "label CONTAINS %@", "saved on this device")).firstMatch
        XCTAssertTrue(savedHomeBanner.waitForExistence(timeout: 5))
        water.tap()
        XCTAssertTrue(app.buttons["save-log-button"].waitForExistence(timeout: 5))
        app.buttons["save-log-button"].tap()
        let pending = app.buttons["pending-saves"]
        XCTAssertTrue(pending.waitForExistence(timeout: 10), "The log should wait on the device")
        app.terminate()

        app.launchArguments = ["-disableAnimations", "-nabuBaseURL", server]
        app.launch()
        XCTAssertTrue(water.waitForExistence(timeout: 30))
        XCTAssertTrue(pending.waitForNonExistence(timeout: 30), "the queued log should sync once the server confirms the session")
        XCTAssertFalse(banners.firstMatch.exists)
        let label = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch.label
        XCTAssertTrue(label.contains("done") && !label.contains("never done"), "The synced log shows on Home: \(label)")
    }
}

// MARK: - Member Access Suspension (real server)

/// Owner disable/enable flow against a real server, the iOS counterpart of
/// `tests/e2e/settings-disable-member.spec.js`. The owner is created by the
/// app's auto-register hook; the second member is registered and joined over
/// HTTP from the test process so both accounts exist before the UI works.
final class NabuMemberSuspensionUITests: XCTestCase {
    let uiTimeout = 15.0
    var app: XCUIApplication!
    let password = "test123456"
    var ownerEmail = ""
    var memberEmail = ""
    let baseURL = URL(string: e2eServerURL)!

    override func setUpWithError() throws {
        continueAfterFailure = false
        let stamp = Int(Date().timeIntervalSince1970)
        let salt = Int.random(in: 0...99_999)
        ownerEmail = "susp-owner-\(stamp)-\(salt)@nabu.local"
        memberEmail = "susp-member-\(stamp)-\(salt)@nabu.local"
        app = XCUIApplication()
    }

    func testOwnerDisablesMemberAndMemberSeesDisabledMessage() throws {
        // 1. Owner: register + create household + seed defaults (app-side).
        app.launchArguments = [
            "-disableAnimations",
            "-nabuBaseURL", baseURL.absoluteString,
            "-nabuAutoRegister", ownerEmail, password,
        ]
        app.launchForLocalTest()
        let seededChore = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch
        XCTAssertTrue(seededChore.waitForExistence(timeout: 30), "Owner home grid should load")
        app.terminate()

        // 2. A second member joins with the household's invite code.
        let owner = NabuTestAPI(baseURL: baseURL)
        _ = try owner.json("POST", "/api/auth/login", ["email": ownerEmail, "password": password])
        let household = try owner.json("GET", "/api/household", nil)
        guard let hh = household["household"] as? [String: Any],
              let code = hh["inviteCode"] as? String, !code.isEmpty else {
            XCTFail("Owner should see the household invite code: \(household)")
            return
        }
        let joiner = NabuTestAPI(baseURL: baseURL)
        _ = try joiner.json("POST", "/api/auth/register", ["email": memberEmail, "password": password])
        let joined = try joiner.json("POST", "/api/household/join", ["inviteCode": code])
        XCTAssertTrue(joined["household"] != nil, "Second member should join the household: \(joined)")

        // 3. Owner disables the member from the Settings (household) tab.
        startAs(ownerEmail)
        openSettingsTab()
        revealMemberRow()
        XCTAssertTrue(memberRow.exists, "Member list should show the joined member")
        app.buttons["member-menu"].tap()
        tapMenuAction("Disable access")
        tapMenuAction("Disable access") // destructive confirm
        XCTAssertTrue(app.staticTexts["Disabled"].waitForExistence(timeout: uiTimeout),
                      "Disabled badge should appear after disabling the member")

        // 4. The disabled member sees the access-disabled screen, not the app.
        startAs(memberEmail)
        let message = app.staticTexts["access-suspended-message"]
        XCTAssertTrue(message.waitForExistence(timeout: uiTimeout),
                      "Disabled member should see the access-disabled screen")
        XCTAssertTrue(message.label.contains("temporarily disabled"),
                      "Message should explain that access is temporarily disabled: \(message.label)")
        // The system credential sheet can appear after the form is gone, on top
        // of the app, so clear it before judging what the member actually sees.
        dismissSavePasswordPrompt(waiting: 1)
        let evidence = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        evidence.name = "access-suspended-screen"
        evidence.lifetime = XCTAttachment.Lifetime.keepAlways
        add(evidence)
        app.terminate()

        // 5. Owner re-enables; the badge clears.
        startAs(ownerEmail)
        openSettingsTab()
        revealMemberRow()
        XCTAssertTrue(app.staticTexts["Disabled"].waitForExistence(timeout: uiTimeout),
                      "Disabled badge should survive an app restart")
        app.buttons["member-menu"].tap()
        tapMenuAction("Enable access")
        XCTAssertTrue(memberRow.waitForExistence(timeout: uiTimeout),
                      "Member row should remain after re-enabling")
        XCTAssertFalse(app.staticTexts["Disabled"].exists,
                       "Disabled badge should be gone after re-enabling the member")

        // 6. The re-enabled member is back in the household.
        startAs(memberEmail)
        let memberChore = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", "Water Plants")).firstMatch
        XCTAssertTrue(memberChore.waitForExistence(timeout: 30), "Re-enabled member should load the home grid")
    }

    // MARK: - Helpers

    /// Launches and leaves the app signed in as `email`, signing out of
    /// whatever session the previous phase left behind.
    private func startAs(_ email: String) {
        app.launchArguments = ["-disableAnimations", "-nabuBaseURL", baseURL.absoluteString]
        app.launchForLocalTest()

        let deadline = Date().addingTimeInterval(uiTimeout)
        while Date() < deadline {
            dismissSavePasswordPrompt()
            if app.staticTexts["access-suspended-message"].exists {
                signOutFromSuspendedScreen()
                break
            }
            if app.tabBars.buttons["Settings"].exists {
                signOutFromSettings()
                break
            }
            if app.textFields.firstMatch.exists { break } // already at the login form
            usleep(250_000)
        }
        signIn(email)
    }

    /// iOS offers to save the credential after every successful sign-in. The
    /// sheet is hosted by the system (not the app), covers the login form, and
    /// keeps keyboard focus, so the next sign-in cannot type into the password
    /// field until it is dismissed.
    private func dismissSavePasswordPrompt(waiting: TimeInterval = 0) {
        if waiting > 0 {
            _ = app.buttons["Not Now"].waitForExistence(timeout: waiting)
        }
        var hosts: [XCUIApplication] = [app]
        hosts.append(XCUIApplication(bundleIdentifier: "com.apple.springboard"))
        for host in hosts {
            let button = host.buttons["Not Now"]
            if button.exists {
                button.tap()
                return
            }
        }
    }

    /// Fills the password field and submits. The typed value is read back
    /// because a keystroke that never reaches the field's binding fails
    /// silently at the form: the Sign In button stays disabled.
    private func submitLogin(_ password: String) {
        let field = app.secureTextFields.firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: uiTimeout), "Password field should be shown")
        dismissSavePasswordPrompt()
        for attempt in 1...3 {
            field.tap()
            usleep(250_000)
            if attempt > 1 { field.doubleTap() } // select, so retyping replaces leftovers
            field.typeText(password)
            let value = field.value as? String ?? ""
            if !value.isEmpty && value != "Password" { break }
        }
        let signInButton = app.buttons["Sign In"]
        if signInButton.waitForExistence(timeout: 2) { signInButton.tap() }
    }

    /// The save prompt arrives seconds after the form closes (about 7 s on
    /// the iOS 27 simulator), over whatever screen the test has reached, so
    /// wait for it here rather than let it intercept a later tap.
    private func waitForLoginFormToClose() {
        let deadline = Date().addingTimeInterval(uiTimeout)
        while Date() < deadline, app.secureTextFields.firstMatch.exists {
            usleep(250_000)
        }
        dismissSavePasswordPrompt(waiting: uiTimeout)
    }

    private func signIn(_ email: String) {
        let emailField = app.textFields.firstMatch
        XCTAssertTrue(emailField.waitForExistence(timeout: uiTimeout), "Login form should be shown for \(email)")
        emailField.tap()
        emailField.typeText(email)
        submitLogin(password)
        if app.secureTextFields.firstMatch.exists {
            app.buttons["Sign In"].tap()
        }
        // Also on a first-try sign-in, which is when the save prompt used to
        // go unhandled and cover the next step.
        waitForLoginFormToClose()
        if app.secureTextFields.firstMatch.exists {
            submitLogin(password)
            waitForLoginFormToClose()
        }
        if app.secureTextFields.firstMatch.exists {
            let attachment = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
            attachment.name = "stuck-on-login-form"
            attachment.lifetime = XCTAttachment.Lifetime.keepAlways
            add(attachment)
        }
        XCTAssertFalse(app.secureTextFields.firstMatch.exists,
                       "Signing in as \(email) should leave the login form")
    }

    /// The member row shows the display name (the local part of the generated
    /// email) with the address underneath.
    private var memberRow: XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", memberName)).firstMatch
    }

    private var memberName: String {
        String(memberEmail.prefix(while: { $0 != "@" }))
    }

        /// Settings is a scrolling list, so bring an element on screen before using
    /// it. How far the list has to scroll varies with content height, so swipe
    /// until the element appears rather than assuming one swipe is enough.
    private func scrollUntilVisible(_ element: XCUIElement, maxSwipes: Int = 5) {
        if element.waitForExistence(timeout: 2) { return }
        for _ in 0..<maxSwipes {
            app.swipeUp()
            if element.waitForExistence(timeout: 1) { return }
        }
    }

    private func revealMemberRow() {
        let menu = app.buttons["member-menu"]
        scrollUntilVisible(menu)
        if !menu.waitForExistence(timeout: uiTimeout) {
            try? app.debugDescription.write(
                toFile: "/private/var/folders/pk/6j3lsvxs5qld69s1ct38qrlw0000gn/T/opencode/nabu-hierarchy.txt",
                atomically: true, encoding: .utf8)
            XCTFail("Member row should expose its menu")
            return
        }
    }

    private func openSettingsTab() {
        let settings = app.tabBars.buttons["Settings"]
        XCTAssertTrue(settings.waitForExistence(timeout: uiTimeout), "Settings tab should be visible")
        // A tap can land while the app is still finishing a sign-in refresh,
        // which leaves it on the Home tab, so confirm the Members section
        // arrived and retry the tap if it did not.
        let membersHeader = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Members (")).firstMatch
        for _ in 0..<3 {
            settings.tap()
            if membersHeader.waitForExistence(timeout: 3) { return }
        }
        XCTAssertTrue(membersHeader.waitForExistence(timeout: uiTimeout), "Settings should show the Members section")
    }

    private func signOutFromSettings() {
        openSettingsTab()
        let signOutButton = app.buttons["Sign Out"]
        scrollUntilVisible(signOutButton)
        XCTAssertTrue(signOutButton.waitForExistence(timeout: uiTimeout), "Sign Out should be reachable")
        signOutButton.tap()
        XCTAssertTrue(app.textFields.firstMatch.waitForExistence(timeout: uiTimeout),
                      "Signing out should return to the login form")
    }

    private func signOutFromSuspendedScreen() {
        let signOutButton = app.buttons["access-suspended-sign-out"]
        XCTAssertTrue(signOutButton.waitForExistence(timeout: uiTimeout),
                      "A disabled account must still be able to sign out")
        signOutButton.tap()
        XCTAssertTrue(app.textFields.firstMatch.waitForExistence(timeout: uiTimeout),
                      "Signing out should return to the login form")
    }

    /// Taps a menu item, waiting for the menu/alert to present it, then waits
    /// for it to disappear so the next action does not race the dismissal.
    private func tapMenuAction(_ label: String) {
        let item = app.buttons[label]
        XCTAssertTrue(item.waitForExistence(timeout: uiTimeout), "Expected action: \(label)")
        item.tap()
        let deadline = Date().addingTimeInterval(uiTimeout)
        while item.exists, Date() < deadline {
            usleep(100_000)
        }
    }
}

/// Minimal JSON client for server-side state the app cannot create itself
/// (a second account). Cookies and the CSRF token are handled explicitly:
/// the server's `nabu_csrf` cookie must be echoed in `X-CSRF-Token` on every
/// state-changing call, exactly like `apiFetch()` in the PWA.
private final class NabuTestAPI {
    private let baseURL: URL
    private let session: URLSession
    private let cookieLock = NSLock()
    private var cookies: [String: String] = [:]

    init(baseURL: URL) {
        self.baseURL = baseURL
        let config = URLSessionConfiguration.ephemeral
        config.httpShouldSetCookies = false // this client owns the jar
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        self.session = URLSession(configuration: config)
    }

    func json(_ method: String, _ path: String, _ body: [String: Any]?) throws -> [String: Any] {
        if method != "GET", cookieValue("nabu_csrf") == nil {
            // Pre-flight so the server hands us a CSRF cookie, as the app does.
            _ = try send(URLRequest(url: baseURL.appendingPathComponent("/api/me")))
        }
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let csrf = cookieValue("nabu_csrf"), method != "GET" {
            request.setValue(csrf, forHTTPHeaderField: "X-CSRF-Token")
        }
        if body != nil {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: body!)
        }
        return try send(request)
    }

    private func cookieValue(_ name: String) -> String? {
        cookieLock.lock()
        defer { cookieLock.unlock() }
        return cookies[name]
    }

    private func send(_ base: URLRequest) throws -> [String: Any] {
        var request = base
        cookieLock.lock()
        let jar = cookies
        cookieLock.unlock()
        if !jar.isEmpty {
            let header = jar.map { "\($0.key)=\($0.value)" }.joined(separator: "; ")
            request.setValue(header, forHTTPHeaderField: "Cookie")
        }

        var payload: [String: Any] = [:]
        var failure: Error?
        let done = DispatchSemaphore(value: 0)
        session.dataTask(with: request) { data, response, error in
            failure = error
            if let http = response as? HTTPURLResponse {
                self.storeCookies(from: http, for: request.url ?? self.baseURL)
                var decoded: [String: Any] = [:]
                if let data = data {
                    decoded = (try? JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
                }
                decoded["_status"] = http.statusCode
                payload = decoded
            }
            done.signal()
        }.resume()
        if done.wait(timeout: .now() + 30) == .timedOut {
            throw URLError(.timedOut)
        }
        if let failure = failure { throw failure }
        return payload
    }

    private func storeCookies(from response: HTTPURLResponse, for url: URL) {
        var fields: [String: String] = [:]
        for (key, value) in response.allHeaderFields {
            guard let key = key as? String else { continue }
            if let value = value as? String {
                fields[key] = value
            } else if let values = value as? [String] {
                fields[key] = values.joined(separator: ", ")
            }
        }
        let parsed = HTTPCookie.cookies(withResponseHeaderFields: fields, for: url)
        guard !parsed.isEmpty else { return }
        cookieLock.lock()
        for cookie in parsed {
            if cookie.value.isEmpty {
                cookies.removeValue(forKey: cookie.name)
            } else {
                cookies[cookie.name] = cookie.value
            }
        }
        cookieLock.unlock()
    }
}

// MARK: - Accessibility (P6/C6)

/// Runs the seeded home grid at an accessibility Dynamic Type size and puts
/// Xcode's automated accessibility audit over the main surfaces — the
/// review-blocking smoke gate from the P6 plan.
final class NabuAccessibilityUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = [
            "-disableAnimations", "-seedHomeForUITest",
            "-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityL",
        ]
        app.launchForLocalTest()
    }

    /// The grid must survive an accessibility type size: tiles visible and
    /// the tab bar reachable (no clipped/lost controls).
    func testHomeGridAtAccessibilitySize() throws {
        XCTAssertTrue(app.staticTexts["Feed Cats"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.tabBars.buttons["Home"].exists)
        XCTAssertTrue(app.tabBars.buttons["Stats"].exists)
        XCTAssertTrue(app.tabBars.buttons["Settings"].exists)
    }

    /// Xcode's automated audit (contrast, labels, hit regions, Dynamic Type)
    /// over the home tab, run from the default type size (the audit scales
    /// sizes itself; starting at an AX size makes it judge only the flattened
    /// top of the caption curve and flag by-design HIG scaling).
    /// Contrast findings on emoji-only text are ignored: the tiles' chore
    /// emoji are user content, and the contrast heuristic can't meaningfully
    /// rate multi-color emoji glyphs.
    func testAccessibilityAuditOnHome() throws {
        let auditApp = XCUIApplication()
        auditApp.launchArguments = ["-disableAnimations", "-seedHomeForUITest"]
        auditApp.launchForLocalTest()
        XCTAssertTrue(auditApp.staticTexts["Feed Cats"].waitForExistence(timeout: 5))
        try auditApp.performAccessibilityAudit { issue in
            if let element = issue.element {
                let label = element.label.trimmingCharacters(in: .whitespaces)
                // Emoji-only elements are pictographs (user-chosen chore
                // icons): the contrast heuristic can't rate multi-color
                // glyphs, and their Dynamic Type growth is deliberately
                // capped at AX1 so they don't clip inside the tiles.
                if !label.isEmpty && label.allSatisfy({ $0.unicodeScalars.contains { $0.properties.isEmojiPresentation } }) {
                    return true
                }
                // Known checker artifact: the pill tab text measures 6.18:1
                // on its white capsule and 4.79:1 on the beige track
                // (BrandPrimary light #236886, verified from rendered pixels
                // 2026-07-03), but the audit samples the capsule's
                // antialiased edge and flags it regardless.
                if issue.auditType == .contrast && (label == "Log" || label == "Manage") {
                    return true
                }
            }
            let frame = issue.element.map { "\($0.frame)" } ?? "?"
            print("AXAUDIT issue: type=\(issue.auditType) desc=\(issue.compactDescription) element=\(issue.element?.description ?? "nil") frame=\(frame)")
            return false
        }
    }
}

// MARK: - Profile

/// Settings → Account sets the name and color the household sees (PWA
/// `settings-profile.spec.js`). The fixture answers PATCH /api/me like the
/// server, including its name rules.
final class NabuProfileUITests: XCTestCase {
    func testNameAndColorSaveAndABlankNameIsRefused() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI", "-nabuBaseURL", "http://localhost:9998"]
        app.launchForLocalTest()
        app.tabBars.buttons["Settings"].tap()
        let row = app.buttons["edit-profile"]
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        XCTAssertTrue(row.label.contains("UI Tester"), row.label)

        row.tap()
        let field = app.textFields["profile-name"]
        XCTAssertTrue(field.waitForExistence(timeout: 5))
        field.tap()
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 12) + "Dana O'Connor")
        app.buttons["Purple"].tap()
        XCTAssertTrue(app.buttons["Purple"].isSelected)
        app.buttons["save-profile"].tap()
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        let saved = NSPredicate(format: "label CONTAINS %@", "Dana O'Connor")
        expectation(for: saved, evaluatedWith: row)
        waitForExpectations(timeout: 5)

        row.tap()
        XCTAssertTrue(field.waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Purple"].isSelected, "the saved color is selected again")
        field.tap()
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 20) + "   ")
        app.buttons["save-profile"].tap()
        XCTAssertTrue(app.staticTexts["profile-error"].waitForExistence(timeout: 5))
        XCTAssertEqual(app.staticTexts["profile-error"].label, "Enter your name")
        app.buttons["Cancel"].tap()
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        XCTAssertTrue(row.label.contains("Dana O'Connor"), row.label)
    }
}

/// A new account sets up its household: name and color, the household's
/// name, then who and what it looks after, which seeds only those packs (PWA
/// `onboarding-packs.spec.js`). Also Manage → Add activities.
final class NabuOnboardingUITests: XCTestCase {
    private func launch(_ scenario: String) -> XCUIApplication {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI", "-reviewScenario", scenario,
                               "-nabuBaseURL", "http://localhost:9998"]
        app.launchForLocalTest()
        return app
    }

    private func type(_ text: String, into field: XCUIElement) {
        XCTAssertTrue(field.waitForExistence(timeout: 5), "\(field)")
        field.tap()
        field.typeText(text)
    }

    /// The setup screen scrolls; bring a control on screen before tapping it.
    private func tapScrolling(_ element: XCUIElement, in app: XCUIApplication) {
        XCTAssertTrue(element.waitForExistence(timeout: 5), "\(element)")
        for _ in 0..<5 where !(element.isHittable && app.frame.contains(element.frame)) { app.swipeUp() }
        element.tap()
    }

    private func tile(_ name: String, in app: XCUIApplication) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
    }

    private func expectError(_ message: String, in app: XCUIApplication, file: StaticString = #filePath, line: UInt = #line) {
        let error = app.staticTexts["onboarding-error"]
        XCTAssertTrue(error.waitForExistence(timeout: 5), file: file, line: line)
        let shown = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label BEGINSWITH %@", message), object: error)
        XCTAssertEqual(XCTWaiter.wait(for: [shown], timeout: 5), .completed, "error: \(error.label)", file: file, line: line)
    }

    func testSetupSeedsThePickedPacksNamedForTheBabyAndPets() {
        let app = launch("onboarding")
        XCTAssertTrue(app.staticTexts["Welcome to Nabu"].waitForExistence(timeout: 10))
        type("Dana O'Connor\n", into: app.textFields["onboarding-name"])
        app.buttons["Purple"].tap()
        app.buttons["onboarding-continue"].tap()

        let household = app.textFields["onboarding-household"]
        XCTAssertTrue(household.waitForExistence(timeout: 5))
        XCTAssertEqual(household.value as? String, "The O'Connors' home")
        XCTAssertEqual(app.textFields["onboarding-initials"].value as? String, "TOH")
        app.buttons["onboarding-continue"].tap()

        let baby = app.buttons["pack-baby"]
        XCTAssertTrue(baby.waitForExistence(timeout: 5))
        baby.tap()
        XCTAssertTrue(baby.isSelected)
        type("Ada\n", into: app.textFields["pack-baby-name-0"])
        app.buttons["pack-cat"].tap()
        type("Mongo\n", into: app.textFields["pack-cat-name-0"])
        tapScrolling(app.buttons["pack-cat-add-name"], in: app)
        type("Roger\n", into: app.textFields["pack-cat-name-1"])
        tapScrolling(app.buttons["onboarding-create"], in: app)

        let link = app.staticTexts["onboarding-invite-link"]
        XCTAssertTrue(link.waitForExistence(timeout: 10))
        XCTAssertTrue(link.label.hasSuffix("/join?code=SETUP1"), link.label)
        XCTAssertTrue(app.buttons["onboarding-share"].exists)
        let qr = app.descendants(matching: .any)["invite-qr"]
        XCTAssertTrue(qr.exists, "a QR code of the link")
        XCTAssertEqual(qr.label, "QR code for the invite link")
        app.buttons["onboarding-done"].tap()

        for name in ["Feed Ada", "Change Ada", "Feed Mongo", "Feed Roger", "Clean Litter Box"] {
            XCTAssertTrue(tile(name, in: app).waitForExistence(timeout: 10), name)
        }
        XCTAssertFalse(tile("Water Plants", in: app).exists, "only the picked packs")
    }

    func testRefusalsAndGoingBackKeepWhatWasTyped() {
        let app = launch("onboarding")
        let continueButton = app.buttons["onboarding-continue"]
        XCTAssertTrue(continueButton.waitForExistence(timeout: 10))
        continueButton.tap()
        expectError("Enter your name", in: app)

        type("Sam\n", into: app.textFields["onboarding-name"])
        continueButton.tap()
        let household = app.textFields["onboarding-household"]
        XCTAssertTrue(household.waitForExistence(timeout: 5))
        XCTAssertEqual(household.value as? String, "Sam's home")
        app.buttons["Back"].tap()
        XCTAssertEqual(app.textFields["onboarding-name"].value as? String, "Sam")
        continueButton.tap()
        XCTAssertTrue(household.waitForExistence(timeout: 5))
        continueButton.tap()

        XCTAssertTrue(app.buttons["pack-baby"].waitForExistence(timeout: 5))
        tapScrolling(app.buttons["onboarding-create"], in: app)
        expectError("Pick at least one", in: app)
        app.buttons["pack-baby"].tap()
        type("Max\n", into: app.textFields["pack-baby-name-0"])
        app.buttons["pack-cat"].tap()
        type("max\n", into: app.textFields["pack-cat-name-0"])
        tapScrolling(app.buttons["onboarding-create"], in: app)
        expectError("\"max\" is used twice", in: app)
        XCTAssertEqual(app.buttons["onboarding-create"].label, "Create household", "nothing was created")
    }

    func testAFailedSeedKeepsTheHouseholdAndRetriesThePacks() {
        let app = launch("onboarding-pack-error")
        type("Lee\n", into: app.textFields["onboarding-name"])
        app.buttons["onboarding-continue"].tap()
        XCTAssertTrue(app.textFields["onboarding-household"].waitForExistence(timeout: 5))
        app.buttons["onboarding-continue"].tap()
        let plants = app.buttons["pack-plants"]
        XCTAssertTrue(plants.waitForExistence(timeout: 5))
        plants.tap()
        tapScrolling(app.buttons["onboarding-create"], in: app)

        expectError("Your household is ready, but its activities weren't added.", in: app)
        XCTAssertTrue(app.buttons["pack-plants"].isSelected, "the picks are kept")
        XCTAssertFalse(app.buttons["Back"].exists)
        XCTAssertEqual(app.buttons["onboarding-create"].label, "Add activities")
        tapScrolling(app.buttons["onboarding-create"], in: app)
        XCTAssertTrue(app.buttons["onboarding-done"].waitForExistence(timeout: 10))
        app.buttons["onboarding-done"].tap()
        XCTAssertTrue(tile("Water Plants", in: app).waitForExistence(timeout: 10))
    }

    func testManageAddsAPackNamedForANewPet() {
        let app = launch("home")
        app.buttons["Manage"].tap()
        let add = app.buttons["add-packs"]
        XCTAssertTrue(add.waitForExistence(timeout: 5))
        add.tap()
        let cats = app.buttons["pack-cat"]
        XCTAssertTrue(cats.waitForExistence(timeout: 5))
        cats.tap()
        type("Luna\n", into: app.textFields["pack-cat-name-0"])
        app.buttons["add-packs-confirm"].tap()
        XCTAssertTrue(add.waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Feed Luna"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Clean Litter Box"].exists)

        // The same pack again adds nothing and says so.
        add.tap()
        XCTAssertTrue(cats.waitForExistence(timeout: 5))
        cats.tap()
        type("Luna\n", into: app.textFields["pack-cat-name-0"])
        app.buttons["add-packs-confirm"].tap()
        let message = app.staticTexts["add-packs-message"]
        XCTAssertTrue(message.waitForExistence(timeout: 5))
        XCTAssertEqual(message.label, "You have these activities already")
        app.buttons["Cancel"].tap()
    }
}

/// Notifications are offered on Home after the first log, never at sign-up
/// (PWA `push-offer.spec.js`). `-pushUndecided` stands in for a device that
/// hasn't decided on notifications.
final class NabuPushOfferUITests: XCTestCase {
    private func launch(resetting: Bool) -> XCUIApplication {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI", "-pushUndecided",
                               "-nabuBaseURL", "http://localhost:9998"]
        if !resetting { app.launchArguments.append("-preserveTestState") }
        app.launchForLocalTest()
        return app
    }

    private func logWaterPlants(in app: XCUIApplication) {
        let tile = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Water Plants")).firstMatch
        XCTAssertTrue(tile.waitForExistence(timeout: 5))
        tile.tap()
        let save = app.buttons["save-log-button"]
        XCTAssertTrue(save.waitForExistence(timeout: 5))
        for _ in 0..<8 where !(save.exists && save.isHittable) { app.swipeUp() }
        save.tap()
        XCTAssertTrue(save.waitForNonExistence(timeout: 5))
    }

    func testTheFirstLogOffersNotificationsAndNotNowKeepsThemAway() {
        let app = launch(resetting: true)
        let offer = app.descendants(matching: .any)["push-offer"]
        XCTAssertTrue(app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Water Plants")).firstMatch.waitForExistence(timeout: 5))
        XCTAssertFalse(offer.exists, "nothing before a log")

        logWaterPlants(in: app)
        XCTAssertTrue(offer.waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Get reminders on this device?"].exists)
        app.buttons["push-offer-dismiss"].tap()
        XCTAssertTrue(offer.waitForNonExistence(timeout: 5))

        logWaterPlants(in: app)
        XCTAssertFalse(offer.waitForExistence(timeout: 2), "answered: a later log doesn't offer again")
        app.terminate()
        let relaunched = launch(resetting: false)
        XCTAssertTrue(relaunched.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Water Plants")).firstMatch.waitForExistence(timeout: 5))
        XCTAssertFalse(relaunched.descendants(matching: .any)["push-offer"].exists)
    }
}

/// Invite links (PWA `invite-landing.spec.js`): one opened while signed out
/// says who sent it and to which household, on sign-in and on sign-up, and a
/// dead one says so. Settings shows the household's invite link as a QR code.
final class NabuInviteUITests: XCTestCase {
    /// Opens a link as tapping one would, accepting iOS's "Open in "Nabu"?".
    private func open(_ link: String, in app: XCUIApplication) {
        app.open(URL(string: link)!)
        let confirm = XCUIApplication(bundleIdentifier: "com.apple.springboard").buttons["Open"]
        if confirm.waitForExistence(timeout: 3) { confirm.tap() }
    }

    func testInviteLinkOpenedSignedOutNamesTheSenderAndHousehold() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState", "-useMockAPI", "-nabuBaseURL", "http://localhost:9998"]
        app.launchForLocalTest()
        XCTAssertTrue(app.buttons["Create Account"].waitForExistence(timeout: 10))
        XCTAssertFalse(app.descendants(matching: .any)["invite-banner"].exists)

        open("nabu:///join?code=SETUP1", in: app)
        let banner = app.descendants(matching: .any)["invite-banner"]
        XCTAssertTrue(banner.waitForExistence(timeout: 10))
        XCTAssertTrue(banner.label.contains("Dana O'Connor invited you to join"), banner.label)
        XCTAssertTrue(banner.label.contains("The O'Connors' home"), banner.label)
        app.buttons["Create Account"].tap()
        XCTAssertTrue(app.secureTextFields["Confirm Password"].waitForExistence(timeout: 5))
        XCTAssertTrue(banner.waitForExistence(timeout: 5), "sign-up shows it too")

        // A dead link says so, and is dropped rather than joined after sign-up.
        open("nabu:///join?code=DEAD99", in: app)
        XCTAssertTrue(app.descendants(matching: .any)["invite-banner-invalid"].waitForExistence(timeout: 10))
        XCTAssertFalse(banner.exists)
    }

    func testSettingsShowsTheInviteLinkAsAQRCode() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI", "-reviewScenario", "invite",
                               "-nabuBaseURL", "http://localhost:9998"]
        app.launchForLocalTest()
        app.tabBars.buttons["Settings"].tap()
        let show = app.buttons["show-invite-qr"]
        XCTAssertTrue(show.waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["share-invite-link"].exists)
        XCTAssertFalse(app.staticTexts["http://localhost:9998/join?code=TESTHOME01"].exists, "no raw link under the household's name")

        show.tap()
        XCTAssertTrue(app.descendants(matching: .any)["invite-qr"].waitForExistence(timeout: 5))
        XCTAssertEqual(app.staticTexts["invite-qr-link"].label, "http://localhost:9998/join?code=TESTHOME01")
        app.buttons["Done"].tap()
        XCTAssertTrue(show.waitForExistence(timeout: 5))
    }
}
