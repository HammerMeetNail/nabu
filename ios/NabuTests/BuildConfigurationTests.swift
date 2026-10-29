import XCTest
@testable import Nabu

/// Every target takes its version from ios/Config, and the app reads its
/// server from the build. App Store Connect refuses an extension whose
/// version differs from its app's.
final class BuildConfigurationTests: XCTestCase {
    func testWidgetExtensionHasTheAppsVersion() throws {
        let app = Bundle.main
        let plugins = try XCTUnwrap(app.builtInPlugInsURL)
        let widget = try XCTUnwrap(Bundle(url: plugins.appendingPathComponent("NabuWidgets.appex")))
        for key in ["CFBundleShortVersionString", "CFBundleVersion"] {
            let value = try XCTUnwrap(app.object(forInfoDictionaryKey: key) as? String, key)
            XCTAssertFalse(value.contains("$("), "\(key) was not expanded: \(value)")
            XCTAssertEqual(widget.object(forInfoDictionaryKey: key) as? String, value, key)
        }
        let version = try XCTUnwrap(app.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String)
        XCTAssertNotNil(version.range(of: #"^\d+\.\d+\.\d+$"#, options: .regularExpression),
                        "version \(version) is not major.minor.patch")
    }

    func testTheAppTalksToTheServerItWasBuiltFor() throws {
        let configured = try XCTUnwrap(Bundle.main.object(forInfoDictionaryKey: "NabuBaseURL") as? String)
        XCTAssertFalse(configured.contains("$("), "NabuBaseURL was not expanded: \(configured)")
        XCTAssertEqual(AppEnvironment.configuredBaseURL(configured).absoluteString, configured)
    }

    func testAMissingOrMalformedServerSettingMeansProduction() {
        for value in [nil, "", "not a url", "ftp://nabu-app.com", "https://"] as [String?] {
            XCTAssertEqual(AppEnvironment.configuredBaseURL(value), AppEnvironment.productionBaseURL,
                           String(describing: value))
        }
        XCTAssertEqual(AppEnvironment.configuredBaseURL("https://staging.example").absoluteString,
                       "https://staging.example")
    }
}
