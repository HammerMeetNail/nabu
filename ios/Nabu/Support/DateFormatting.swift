import Foundation

func formatTimeAgo(_ date: Date) -> String {
    let interval = abs(date.timeIntervalSinceNow)
    switch interval {
    case 0..<60: return "just now"
    case 60..<3600: return "\(Int(interval / 60))m ago"
    case 3600..<86400: return "\(Int(interval / 3600))h ago"
    case 86400..<172800: return "yesterday"
    case 172800..<604800: return "\(Int(interval / 86400))d ago"
    case 604800..<2592000: return "\(Int(interval / 604800))w ago"
    case 2592000..<31536000: return "\(Int(interval / 2592000))mo ago"
    default: return "\(Int(interval / 31536000))yr ago"
    }
}

enum DateFormatting {
    static let isoFormatter: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    static func formatRelative(_ date: Date) -> String {
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .abbreviated
        return formatter.localizedString(for: date, relativeTo: Date())
    }

    static func formatLocalDate(_ localDate: LocalDate) -> String {
        localDate.value
    }
}
