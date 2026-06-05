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
