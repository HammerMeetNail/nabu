import AuthenticationServices
import Foundation
import Security

/// Bridges the native Sign in with Apple flow to the backend: generates the
/// per-request nonce, then exchanges the identity token for a session via
/// `POST /api/auth/apple/native`. The server verifies Apple echoed the nonce
/// into the token, which is what stops a harvested token being replayed.
@MainActor
final class SignInWithAppleCoordinator: ObservableObject {
    @Published var errorMessage: String?
    private(set) var currentNonce: String?
    private var requestOwner: ClientIdentity.Snapshot?

    /// Configures an authorization request; called from the button's
    /// `onRequest` closure.
    func prepare(_ request: ASAuthorizationAppleIDRequest, api: APIClient) {
        requestOwner = api.identity.snapshot
        let nonce = Self.randomNonce()
        currentNonce = nonce
        request.requestedScopes = [.email]
        request.nonce = nonce
    }

    /// Handles the button's `onCompletion` result. Returns the signed-in
    /// user, or nil (with `errorMessage` set unless the user just cancelled).
    func handle(_ result: Result<ASAuthorization, Error>, api: APIClient) async -> User? {
        errorMessage = nil
        guard let owner = requestOwner, api.identity.isCurrent(owner) else { return nil }
        requestOwner = nil
        switch result {
        case .failure(let error):
            if let authError = error as? ASAuthorizationError, authError.code == .canceled {
                return nil
            }
            errorMessage = "Sign in with Apple failed"
            return nil
        case .success(let authorization):
            guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
                  let tokenData = credential.identityToken,
                  let identityToken = String(data: tokenData, encoding: .utf8),
                  let nonce = currentNonce else {
                errorMessage = "Sign in with Apple failed"
                return nil
            }
            currentNonce = nil
            do {
                let body = AppleNativeAuthRequest(identityToken: identityToken, nonce: nonce)
                let response: UserResponse = try await api.post("/api/auth/apple/native", body: body)
                return response.user
            } catch let error as APIError {
                errorMessage = error.errorDescription ?? "Sign in with Apple failed"
                return nil
            } catch {
                errorMessage = "Sign in with Apple failed"
                return nil
            }
        }
    }

    /// 32 random bytes, hex-encoded. The server compares the token's nonce
    /// claim against this exact string.
    nonisolated static func randomNonce(byteCount: Int = 32) -> String {
        var bytes = [UInt8](repeating: 0, count: byteCount)
        let status = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        if status != errSecSuccess {
            // SecRandomCopyBytes failing is effectively unheard of; fall back
            // to SystemRandomNumberGenerator rather than crashing sign-in.
            var generator = SystemRandomNumberGenerator()
            bytes = (0..<byteCount).map { _ in UInt8.random(in: .min ... .max, using: &generator) }
        }
        return bytes.map { String(format: "%02x", $0) }.joined()
    }
}
