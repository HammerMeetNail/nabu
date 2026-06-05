import XCTest

final class NabuUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-resetState"]
        app.launch()
    }

    // MARK: - Auth Flow

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

// MARK: - Home Tab Tests

/// Regression tests for the home-tab chore logging flow.
/// Uses -seedHomeForUITest to inject a logged-in state without a real server.
final class NabuHomeUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-disableAnimations", "-seedHomeForUITest"]
        app.launch()
    }

    /// Regression: tapping a chore on the home grid must show the LogSheet
    /// with its title and Cancel button visible — never a blank/empty sheet.
    ///
    /// The bug was .sheet(isPresented: $showingLogSheet) { if let chore = selectedChore }
    /// — when selectedChore was nil at closure evaluation time, SwiftUI produced EmptyView
    /// and the modal appeared blank.  The fix uses .sheet(item: $selectedChore) so the
    /// chore is always non-nil in the closure.
    func testChoreTapShowsLogSheet() throws {
        // The seeded home grid has one chore: "Feed Cats" with no previous log.
        let choreCell = app.buttons["Feed Cats, never done"]
        XCTAssertTrue(choreCell.waitForExistence(timeout: 5), "Chore cell must appear on home grid")

        choreCell.tap()

        // LogSheet must present with its navigation title and Cancel button.
        // On the old buggy code these would not exist (blank sheet = EmptyView).
        XCTAssertTrue(
            app.buttons["Cancel"].waitForExistence(timeout: 3),
            "LogSheet Cancel button must appear after tapping a chore"
        )
        XCTAssertTrue(
            app.staticTexts["🐱 Feed Cats"].exists,
            "LogSheet navigation title must show chore icon and name"
        )
    }
}
