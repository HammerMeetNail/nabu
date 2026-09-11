import AuthenticationServices
import SwiftUI
import WebKit

@MainActor
final class GoogleOAuthCoordinator: NSObject, ObservableObject {
    @Published var isAuthenticating = false
    @Published var errorMessage: String?

    let baseURL: URL

    init(baseURL: URL) {
        self.baseURL = baseURL
    }

    func authenticate() async -> User? {
        isAuthenticating = true
        errorMessage = nil
        defer { isAuthenticating = false }

        // Build login URL with redirect to nabu:// scheme so ASWebAuthenticationSession can intercept it
        guard var components = URLComponents(url: baseURL.appendingPathComponent("api/auth/google/login"),
                                             resolvingAgainstBaseURL: false) else {
            errorMessage = "Google sign-in failed"
            return nil
        }
        var queryItems = components.queryItems ?? []
        queryItems.append(URLQueryItem(name: "redirect", value: "nabu://callback"))
        components.queryItems = queryItems
        guard let loginURL = components.url else {
            errorMessage = "Google sign-in failed"
            return nil
        }

        let api = APIClient(baseURL: baseURL, identity: ClientIdentity.shared(for: baseURL))
        let ticket: UUID
        do { ticket = try api.identity.beginTransition() }
        catch { errorMessage = "Finish the current account change first."; return nil }
        await api.identity.publish()
        let succeeded: Bool = await withCheckedContinuation { continuation in
            let session = ASWebAuthenticationSession(
                url: loginURL,
                callbackURLScheme: "nabu"
            ) { callbackURL, error in
                if let error = error as? ASWebAuthenticationSessionError {
                    if error.code != .canceledLogin {
                        self.errorMessage = "Google sign-in failed"
                    }
                    continuation.resume(returning: false)
                    return
                }

                guard callbackURL != nil else {
                    self.errorMessage = "Google sign-in failed"
                    continuation.resume(returning: false)
                    return
                }

                // Copy session cookie from ASWebAuthenticationSession's cookie store
                // to HTTPCookieStorage so URLSession can use it.
                WKWebsiteDataStore.default().httpCookieStore.getAllCookies { cookies in
                    Task { @MainActor in
                        guard api.identity.ownsTransition(ticket) else { continuation.resume(returning: false); return }
                        let host = self.baseURL.host ?? ""
                        for cookie in cookies where ["nabu_session", "nabu_csrf"].contains(cookie.name) {
                            let domain = cookie.domain.hasPrefix(".") ? String(cookie.domain.dropFirst()) : cookie.domain
                            guard host == domain || host.hasSuffix("." + domain) else { continue }
                            HTTPCookieStorage.shared.setCookie(cookie)
                        }
                        continuation.resume(returning: true)
                    }
                }
            }

            session.presentationContextProvider = self
            session.prefersEphemeralWebBrowserSession = false
            if !session.start() { continuation.resume(returning: false) }
        }
        do {
            let user = try await api.confirmTransition(ticket)
            return succeeded ? user : nil
        } catch {
            errorMessage = "Could not confirm your session. Retry when connected."
            return nil
        }
    }
}

extension GoogleOAuthCoordinator: ASWebAuthenticationPresentationContextProviding {
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        let scenes = UIApplication.shared.connectedScenes
        let windowScene = scenes.first as? UIWindowScene
        return windowScene?.windows.first(where: { $0.isKeyWindow }) ?? ASPresentationAnchor()
    }
}
