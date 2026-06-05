import SwiftUI

extension Color {
    init?(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        guard hex.count == 6, let int = UInt64(hex, radix: 16) else { return nil }
        let r = Double((int >> 16) & 0xFF) / 255.0
        let g = Double((int >> 8) & 0xFF) / 255.0
        let b = Double(int & 0xFF) / 255.0
        self.init(red: r, green: g, blue: b)
    }

    init(hexUnsafe hex: String) {
        self = Color(hex: hex) ?? .black
    }

    /// Creates a Color that automatically adapts to light/dark mode.
    init(lightHex: String, darkHex: String) {
        let light = UIColor(red: Color.hexChannel(lightHex, shift: 16),
                            green: Color.hexChannel(lightHex, shift: 8),
                            blue: Color.hexChannel(lightHex, shift: 0),
                            alpha: 1)
        let dark  = UIColor(red: Color.hexChannel(darkHex, shift: 16),
                            green: Color.hexChannel(darkHex, shift: 8),
                            blue: Color.hexChannel(darkHex, shift: 0),
                            alpha: 1)
        let adaptive = UIColor { traitCollection in
            traitCollection.userInterfaceStyle == .dark ? dark : light
        }
        self.init(uiColor: adaptive)
    }

    private static func hexChannel(_ hex: String, shift: Int) -> Double {
        let h = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        guard h.count == 6, let int = UInt64(h, radix: 16) else { return 0 }
        return Double((int >> shift) & 0xFF) / 255.0
    }
}

/// PWA-matching colors with automatic dark mode support.
/// Light values match CSS `:root`. Dark values match CSS `prefers-color-scheme: dark`.
enum DesignColors {
    // bg: #F4EFE7 → #0E1117
    static let pageBackground   = Color(lightHex: "F4EFE7", darkHex: "0E1117")

    // surface: #FFFFFF → #181C27
    static let surface          = Color(lightHex: "FFFFFF", darkHex: "181C27")

    // brand: #19323C → #5BBEDD
    static let brand            = Color(lightHex: "19323C", darkHex: "5BBEDD")

    // primary: #2E86AB → #4DABCE
    static let primary          = Color(lightHex: "2E86AB", darkHex: "4DABCE")

    // accent: #F18F01 → #F4A634
    static let accent           = Color(lightHex: "F18F01", darkHex: "F4A634")

    // success: #386641 → #4CAF6E
    static let success          = Color(lightHex: "386641", darkHex: "4CAF6E")

    // danger: #BC4742 → #E05252
    static let danger           = Color(lightHex: "BC4742", darkHex: "E05252")

    // text: #1A1A2E → #E6EDF3
    static let textPrimary      = Color(lightHex: "1A1A2E", darkHex: "E6EDF3")

    // text-secondary: #6B7280 → #8B949E
    static let textSecondary    = Color(lightHex: "6B7280", darkHex: "8B949E")

    // border: #D1D5DB → #2A3042
    static let border           = Color(lightHex: "D1D5DB", darkHex: "2A3042")

    // calendar-bg: #E8F4FB → #10141C
    static let calendarBg       = Color(lightHex: "E8F4FB", darkHex: "10141C")

    // surface-secondary (used for chips, pill backgrounds, segment controls)
    // PWA uses a darker bg variant: #E8E2D6 light, no exact dark match — use a tinted surface
    static let surfaceSecondary = Color(lightHex: "E8E2D6", darkHex: "252A35")
}

enum Typography {
    static let largeTitle = Font.largeTitle
    static let title = Font.title
    static let title2 = Font.title2
    static let title3 = Font.title3
    static let headline = Font.headline
    static let body = Font.body
    static let callout = Font.callout
    static let subheadline = Font.subheadline
    static let footnote = Font.footnote
    static let caption = Font.caption
}

extension View {
    func pageBackground() -> some View {
        background(DesignColors.pageBackground)
    }

    func surfaceCard() -> some View {
        background(DesignColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .shadow(color: .black.opacity(0.06), radius: 3, y: 1)
    }
}
