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

        return await withCheckedContinuation { continuation in
            let session = ASWebAuthenticationSession(
                url: loginURL,
                callbackURLScheme: "nabu"
            ) { callbackURL, error in
                if let error = error as? ASWebAuthenticationSessionError {
                    if error.code != .canceledLogin {
                        self.errorMessage = "Google sign-in failed"
                    }
                    continuation.resume(returning: nil)
                    return
                }

                guard callbackURL != nil else {
                    self.errorMessage = "Google sign-in failed"
                    continuation.resume(returning: nil)
                    return
                }

                // Copy session cookie from ASWebAuthenticationSession's cookie store
                // to HTTPCookieStorage so URLSession can use it.
                WKWebsiteDataStore.default().httpCookieStore.getAllCookies { cookies in
                    for cookie in cookies {
                        HTTPCookieStorage.shared.setCookie(cookie)
                    }
                    Task { @MainActor in
                        let api = APIClient(baseURL: self.baseURL)
                        do {
                            let response: UserResponse = try await api.get("/api/me")
                            continuation.resume(returning: response.user)
                        } catch {
                            self.errorMessage = "Failed to load account"
                            continuation.resume(returning: nil)
                        }
                    }
                }
            }

            session.presentationContextProvider = self
            session.prefersEphemeralWebBrowserSession = false
            session.start()
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
