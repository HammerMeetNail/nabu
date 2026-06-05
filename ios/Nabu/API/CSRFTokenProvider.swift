import Foundation

struct CSRFTokenProvider {
    let cookieStore: CookieStore

    var token: String? {
        cookieStore.csrfCookie?.value
    }
}
