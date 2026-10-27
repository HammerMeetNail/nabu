import Foundation

/// One Stats read: an endpoint under /api/stats/ and its query. The dashboard
/// request names each read the same way (`read=chores?period=month`).
struct StatsRead: Hashable {
    let path: String
    var query: [URLQueryItem] = []

    /// Per-chore charts travel apart from the cheaper sections, so a slow
    /// chart never holds up the rest of the page (PWA `stats-batch.js`).
    var isChart: Bool {
        path.hasPrefix("chores/") && (path.hasSuffix("/time-series") || path.hasSuffix("/summary"))
    }

    var dashboardValue: String {
        var components = URLComponents()
        components.queryItems = query.isEmpty ? nil : query
        guard let encoded = components.percentEncodedQuery, !encoded.isEmpty else { return path }
        return path + "?" + encoded
    }
}

enum StatsReadOutcome {
    case data(Data)
    case failed(status: Int, message: String?)
    /// The dashboard request itself failed: read the endpoint instead.
    case unavailable
}

/// `GET /api/stats/dashboard`: many Stats reads in one request, each answered
/// with its endpoint's own status and body.
enum StatsDashboard {
    /// The server answers at most this many reads per request.
    static let maxReads = 64

    static func fetch(_ reads: [StatsRead], api: APIClient) async -> [StatsRead: StatsReadOutcome] {
        var outcomes: [StatsRead: StatsReadOutcome] = [:]
        for start in stride(from: 0, to: reads.count, by: maxReads) {
            let chunk = Array(reads[start..<min(start + maxReads, reads.count)])
            let query = chunk.map { URLQueryItem(name: "read", value: $0.dashboardValue) }
            guard let data = try? await api.getData("/api/stats/dashboard", query: query),
                  let results = decode(data), results.count == chunk.count else {
                for read in chunk { outcomes[read] = .unavailable }
                continue
            }
            for (read, result) in zip(chunk, results) { outcomes[read] = result }
        }
        return outcomes
    }

    static func decode(_ data: Data) -> [StatsReadOutcome]? {
        guard let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let results = object["results"] as? [[String: Any]] else { return nil }
        return results.map { result in
            let status = result["status"] as? Int ?? 0
            let body = result["data"] ?? NSNull()
            if (200..<300).contains(status),
               let data = try? JSONSerialization.data(withJSONObject: body, options: [.fragmentsAllowed]) {
                return .data(data)
            }
            return .failed(status: status, message: (body as? [String: Any])?["error"] as? String)
        }
    }
}
