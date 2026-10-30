import Foundation

/// Every failure `APIClient` throws: its requests are `throws(APIError)`. A
/// request that got no answer is `networkError`, whatever the transport threw.
enum APIError: LocalizedError {
    case invalidURL
    case invalidResponse
    case httpError(statusCode: Int)
    case serverError(statusCode: Int, message: String)
    case rateLimited(retryAfter: String?)
    case decodingError(Error)
    case encodingError(Error)
    case networkError(Error)
    case unauthorized
    case contextChanged
    case saveNotDurable
    case submissionInProgress
    /// The signed-in user's household membership is disabled by an admin.
    case accessSuspended(message: String, household: String?)

    var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "Invalid URL"
        case .invalidResponse:
            return "Invalid server response"
        case .httpError(let statusCode):
            return "HTTP error \(statusCode)"
        case .serverError(_, let message):
            return message
        case .rateLimited(let retryAfter):
            if let after = retryAfter {
                return "Too many requests. Retry after \(after)s."
            }
            return "Too many requests. Please try again later."
        case .decodingError(let error):
            return "Failed to decode response: \(error.localizedDescription)"
        case .encodingError(let error):
            return "Failed to encode request: \(error.localizedDescription)"
        case .networkError(let error):
            // As the transport says it, e.g. "The Internet connection appears
            // to be offline."
            return error.localizedDescription
        case .unauthorized:
            return "Session expired. Please sign in again."
        case .contextChanged:
            return "Your account or household changed. Confirm your session before continuing."
        case .saveNotDurable:
            return "This save could not be stored on your device. Keep this form open and retry."
        case .submissionInProgress:
            return "This save is already being sent."
        case .accessSuspended(let message, _):
            return message
        }
    }

    /// What a person sees when an action fails: the server's own message when
    /// it sent one, otherwise the action's `fallback` (the PWA's
    /// `data.error || failure`). A failure the app can explain says so; a lost
    /// connection or an unreadable answer gets the action's message.
    static func message(for error: any Error, fallback: String) -> String {
        guard let error = error as? APIError else { return fallback }
        switch error {
        case .serverError(_, let message), .accessSuspended(let message, _):
            return message.isEmpty ? fallback : message
        case .rateLimited, .unauthorized, .contextChanged, .saveNotDurable, .submissionInProgress:
            return error.errorDescription ?? fallback
        case .invalidURL, .invalidResponse, .httpError, .decodingError, .encodingError, .networkError:
            return fallback
        }
    }

    /// The failure's kind and status without its message: safe to log.
    var kind: String {
        switch self {
        case .invalidURL: "invalidURL"
        case .invalidResponse: "invalidResponse"
        case .httpError(let status): "http \(status)"
        case .serverError(let status, _): "server \(status)"
        case .rateLimited: "rateLimited"
        case .decodingError: "decoding"
        case .encodingError: "encoding"
        case .networkError(let error): "network \((error as NSError).domain) \((error as NSError).code)"
        case .unauthorized: "unauthorized"
        case .contextChanged: "contextChanged"
        case .saveNotDurable: "saveNotDurable"
        case .submissionInProgress: "submissionInProgress"
        case .accessSuspended: "accessSuspended"
        }
    }
}

struct APIErrorResponse: Codable {
    let error: String
    var status: String? = nil
    var household: String? = nil
}
