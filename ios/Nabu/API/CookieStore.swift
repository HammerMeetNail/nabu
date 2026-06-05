import Foundation

struct CookieStore {
    private let storage = HTTPCookieStorage.shared

    var sessionCookie: HTTPCookie? {
        cookies(for: "nabu_session")
    }

    var csrfCookie: HTTPCookie? {
        cookies(for: "nabu_csrf")
    }

    func cookies(for name: String) -> HTTPCookie? {
        storage.cookies?.first { $0.name == name }
    }

    func clearAll() {
        storage.cookies?.forEach { storage.deleteCookie($0) }
    }
}
