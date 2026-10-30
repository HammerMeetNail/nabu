import OSLog

/// The app's unified log, by area. A failure is logged as what failed and the
/// error's kind, which are public, and the error's own text, which is private
/// and so redacted outside a debugger: a server message or a decoding failure
/// can carry what someone typed. Never log a token, a cookie, an invite code
/// or a URL with its query.
enum Log {
    private static let subsystem = Bundle.main.bundleIdentifier ?? "Nabu"
    /// Requests and the session.
    static let api = Logger(subsystem: subsystem, category: "api")
    /// Something a person did that failed.
    static let actions = Logger(subsystem: subsystem, category: "actions")
    /// Reads in the background. A failed one keeps the last good data.
    static let loading = Logger(subsystem: subsystem, category: "loading")
    /// Files on this device: saved Home, the offline queue and timers.
    static let storage = Logger(subsystem: subsystem, category: "storage")

    /// An error's kind and status without its text: safe to log.
    static func kind(of error: any Error) -> String {
        if let error = error as? APIError { return error.kind }
        let error = error as NSError
        return "\(error.domain) \(error.code)"
    }
}

extension Logger {
    /// Logs that `what` failed, with the error's kind; its text is private.
    func failure(_ what: String, _ error: any Error) {
        self.error("\(what, privacy: .public): \(Log.kind(of: error), privacy: .public) \(String(describing: error), privacy: .private)")
    }

    /// Logs a failed action and returns what to tell the person
    /// (`APIError.message(for:fallback:)`), for a sheet that shows its own
    /// failures.
    func failureMessage(_ fallback: String, _ error: any Error) -> String {
        failure(fallback, error)
        return APIError.message(for: error, fallback: fallback)
    }
}
