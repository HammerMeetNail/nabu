import XCTest
@testable import Nabu

/// A failure reaches the person or the log, never nowhere: what `APIClient`
/// throws, what a failed action says, and preference saves that roll back.
@MainActor
final class ErrorReportingTests: XCTestCase {
    private func client(_ handler: @escaping (URLRequest) async throws -> (Data, URLResponse)) -> APIClient {
        var api = APIClient(baseURL: URL(string: "http://localhost:9999")!)
        api.mockAsyncHandler = handler
        return api
    }

    func testALostConnectionIsANetworkError() async {
        let api = client { _ in throw URLError(.notConnectedToInternet) }
        do {
            let _: StatusResponse = try await api.get("/api/household")
            XCTFail("The request should fail")
        } catch {
            guard case .networkError(let cause) = error else { return XCTFail("Got \(error)") }
            XCTAssertEqual((cause as? URLError)?.code, .notConnectedToInternet)
            XCTAssertTrue(LogStore.isNetworkFailure(error), "An offline log must still queue")
            XCTAssertEqual(error.localizedDescription, URLError(.notConnectedToInternet).localizedDescription)
        }
    }

    func testAnUnreadableAnswerIsADecodingError() async {
        let api = client { jsonReply($0, 200, #"{"household": 3}"#) }
        do {
            let _: HouseholdResponse = try await api.get("/api/household")
            XCTFail("The answer should not decode")
        } catch {
            guard case .decodingError = error else { return XCTFail("Got \(error)") }
            XCTAssertFalse(LogStore.isNetworkFailure(error), "The server answered: nothing to queue")
        }
    }

    func testAFailedActionSaysWhatTheServerSaid() {
        let rejected = APIError.serverError(statusCode: 409, message: "Transfer ownership first")
        XCTAssertEqual(APIError.message(for: rejected, fallback: "Failed to remove member"), "Transfer ownership first")
        let suspended = APIError.accessSuspended(message: "Your access to this household is paused.", household: nil)
        XCTAssertEqual(APIError.message(for: suspended, fallback: "Failed to remove member"), "Your access to this household is paused.")
        // Otherwise the action's own message, as the PWA's `data.error || failure`.
        let unexplained: [any Error] = [
            APIError.networkError(URLError(.timedOut)), APIError.httpError(statusCode: 502),
            APIError.decodingError(CocoaError(.coderReadCorrupt)), APIError.serverError(statusCode: 500, message: ""),
            CocoaError(.fileWriteOutOfSpace),
        ]
        for error in unexplained {
            XCTAssertEqual(APIError.message(for: error, fallback: "Failed to remove member"), "Failed to remove member", "\(error)")
        }
        // A failure the app can explain says so.
        XCTAssertEqual(APIError.message(for: APIError.rateLimited(retryAfter: "30"), fallback: "Failed to remove member"),
                       "Too many requests. Retry after 30s.")
        XCTAssertEqual(APIError.message(for: APIError.contextChanged, fallback: "Failed to remove member"),
                       APIError.contextChanged.errorDescription)
    }

    func testALoggedFailureLeavesOutWhatItSaid() {
        XCTAssertEqual(Log.kind(of: APIError.serverError(statusCode: 409, message: "Ada can't be removed")), "server 409")
        XCTAssertEqual(Log.kind(of: APIError.accessSuspended(message: "Ada's access is paused", household: "Ada's home")), "accessSuspended")
        XCTAssertEqual(Log.kind(of: APIError.networkError(URLError(.timedOut))), "network NSURLErrorDomain -1001")
        XCTAssertEqual(Log.kind(of: CocoaError(.fileWriteOutOfSpace)), "NSCocoaErrorDomain 640")
    }

    func testAFailedActionShowsOnlyToTheIdentityThatStartedIt() {
        let state = AppState()
        let owner = state.revision
        state.reportFailure("Failed to remove member", APIError.serverError(statusCode: 409, message: "Transfer ownership first"), owner: owner)
        XCTAssertEqual(state.toast?.message, "Transfer ownership first")

        // Another identity's screens are not the ones that failed.
        state.adopt(ClientIdentity(managed: false).snapshot)
        XCTAssertNil(state.toast, "A new identity starts without the old one's failure")
        state.reportFailure("Failed to remove member", APIError.networkError(URLError(.timedOut)), owner: owner)
        XCTAssertNil(state.toast)

        // A request that changes the identity reports to the identity after it.
        state.reportFailure("Could not change household. Please retry.", APIError.networkError(URLError(.timedOut)))
        XCTAssertEqual(state.toast?.message, "Could not change household. Please retry.")
    }

    func testAHomeOrderThatFailsToSaveGoesBack() async {
        let state = AppState()
        state.choreOrder = [1, 2, 3]
        let preferences = PreferencesDataLoader(api: client { _ in throw URLError(.notConnectedToInternet) }, state: state)
        let save = preferences.saveChoreOrder([3, 1, 2])
        XCTAssertEqual(state.choreOrder, [3, 1, 2], "A List's move needs the new order at once")
        await save.value
        XCTAssertEqual(state.choreOrder, [1, 2, 3], "Back to what the server has, as in the PWA")
    }

    func testASavedHomeOrderTakesTheServersAnswer() async {
        let state = AppState()
        let preferences = PreferencesDataLoader(api: client { jsonReply($0, 200, #"{"preferences":{"choreOrder":[3,1]}}"#) }, state: state)
        await preferences.saveChoreOrder([3, 1, 2]).value
        XCTAssertEqual(state.choreOrder, [3, 1])
    }

    func testAnOlderFailedSaveLeavesANewerOneAlone() async {
        let state = AppState()
        let api = client { request in
            let body = try JSONSerialization.jsonObject(with: request.httpBody ?? Data()) as? [String: Any]
            if body?["hiddenHomeChoreIds"] as? [Int] == [1] { throw URLError(.timedOut) }
            return jsonReply(request, 200, #"{"preferences":{"hiddenHomeChoreIds":[1,2]}}"#)
        }
        let preferences = PreferencesDataLoader(api: api, state: state)
        let first = preferences.saveHiddenHomeChores([1])
        let second = preferences.saveHiddenHomeChores([1, 2])
        await first.value
        await second.value
        XCTAssertEqual(state.hiddenHomeChoreIDs, [1, 2])
    }

    func testHiddenTilesThatFailToSaveComeBack() async {
        let state = AppState()
        let preferences = PreferencesDataLoader(api: client { jsonReply($0, 500, #"{"error":"Could not save preferences"}"#) }, state: state)
        let save = preferences.saveHiddenHomeChores([4])
        XCTAssertEqual(state.hiddenHomeChoreIDs, [4])
        await save.value
        XCTAssertEqual(state.hiddenHomeChoreIDs, [])
    }
}

private func jsonReply(_ request: URLRequest, _ status: Int, _ body: String) -> (Data, URLResponse) {
    (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil,
                                      headerFields: ["Content-Type": "application/json"])!)
}
