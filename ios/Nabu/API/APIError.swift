import Foundation

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
            return "Network error: \(error.localizedDescription)"
        case .unauthorized:
            return "Session expired. Please sign in again."
        }
    }
}

struct APIErrorResponse: Codable {
    let error: String
}
