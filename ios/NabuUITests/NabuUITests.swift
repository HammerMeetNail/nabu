import XCTest

// MARK: - Auth Flow Tests

final class NabuUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState"]
        app.launch()
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
        app.launch()
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
        app.launch()
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
        XCTAssertTrue(app.buttons["volume-picker"].waitForExistence(timeout: 3))
    }

    func testNoVolumePickerForSimpleChore() throws {
        openLogSheet(forChore: "Feed Cats")
        XCTAssertFalse(app.buttons["volume-picker"].exists)
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
        app.launch()
    }

    /// Log a chore that had no prior log; verify time-ago updates on the grid.
    func testLogChoreUpdatesTimeAgo() throws {
        let cell = cell(named: "Water Plants")
        XCTAssertTrue(cell.waitForExistence(timeout: 5))
        XCTAssertTrue(cell.label.contains("never done"), "Precondition: Water Plants should be 'never done'")

        openLogSheet(forChore: "Water Plants")
        XCTAssertTrue(app.buttons["save-log-button"].waitForExistence(timeout: 3))
        app.buttons["save-log-button"].tap()

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

        app.buttons["save-log-button"].tap()

        // Verify sheet dismissed and grid is visible.
        XCTAssertTrue(cell(named: "Walk Dog").waitForExistence(timeout: 5))
    }

    /// Log Feed Baby (which has volume picker).
    func testLogFeedBabySucceeds() throws {
        openLogSheet(forChore: "Feed Baby")

        let volumeButton = app.buttons["volume-picker"]
        XCTAssertTrue(volumeButton.waitForExistence(timeout: 3))

        app.buttons["save-log-button"].tap()

        XCTAssertTrue(cell(named: "Feed Baby").waitForExistence(timeout: 5))
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
}

// MARK: - Quick Log Sheet Tests

/// Tests the quick-log sheet (accessed via the + button in the toolbar).
final class NabuHomeQuickLogUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest", "-useMockAPI"]
        app.launch()
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
        app.launch()
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
        app.launch()
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
            "-nabuBaseURL", "http://localhost:8080",
            "-nabuAutoRegister", email, password,
        ]
        app.launch()

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
