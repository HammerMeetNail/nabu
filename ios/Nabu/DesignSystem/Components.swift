import CoreImage.CIFilterBuiltins
import SwiftUI

// MARK: - OrDivider
// Matches the PWA .auth-divider: ---- or ----

struct OrDivider: View {
    var body: some View {
        HStack(spacing: 16) {
            Rectangle()
                .fill(DesignColors.border)
                .frame(height: 1)
            Text("or")
                .font(.footnote)
                .foregroundColor(DesignColors.textSecondary)
            Rectangle()
                .fill(DesignColors.border)
                .frame(height: 1)
        }
    }
}

// MARK: - GoogleIcon
// Matches the PWA multicolor Google SVG "G" logo

struct GoogleIcon: View {
    var body: some View {
        ZStack {
            // Outer ring segments (blue, red, yellow, green)
            // Using a Text approximation with colored substrings via attributed string
            // Since SwiftUI Text doesn't support inline coloring easily, we build it
            // using overlaid arc shapes instead.
            Canvas { context, size in
                let cx = size.width / 2
                let cy = size.height / 2
                let r = size.width / 2

                // Blue: 270° → 0° (top → right)
                drawArc(context: context, cx: cx, cy: cy, r: r,
                        start: .degrees(-90), end: .degrees(0), color: Color(hexUnsafe: "4285F4"), width: 3)
                // Red: 0° → 120° (right → bottom-left)
                drawArc(context: context, cx: cx, cy: cy, r: r,
                        start: .degrees(0), end: .degrees(120), color: Color(hexUnsafe: "EA4335"), width: 3)
                // Yellow: 120° → 240° (bottom-left → top-left)
                drawArc(context: context, cx: cx, cy: cy, r: r,
                        start: .degrees(120), end: .degrees(240), color: Color(hexUnsafe: "FBBC05"), width: 3)
                // Green: 240° → 270° (top-left → top)
                drawArc(context: context, cx: cx, cy: cy, r: r,
                        start: .degrees(240), end: .degrees(270), color: Color(hexUnsafe: "34A853"), width: 3)
            }
            .frame(width: 18, height: 18)
        }
    }

    private func drawArc(context: GraphicsContext, cx: CGFloat, cy: CGFloat, r: CGFloat,
                         start: Angle, end: Angle, color: Color, width: CGFloat) {
        var path = Path()
        path.addArc(center: CGPoint(x: cx, y: cy), radius: r - width / 2,
                    startAngle: start, endAngle: end, clockwise: false)
        context.stroke(path, with: .color(color), lineWidth: width)
    }
}

// Simpler, cleaner Google icon using the standard four-colored "G" letterform
struct GoogleIconSimple: View {
    var body: some View {
        HStack(spacing: 0) {
            Text("G")
                .font(.system(.callout, design: .rounded).weight(.bold))
                .foregroundStyle(
                    LinearGradient(
                        colors: [
                            Color(hexUnsafe: "4285F4"),
                            Color(hexUnsafe: "EA4335"),
                            Color(hexUnsafe: "FBBC05"),
                            Color(hexUnsafe: "34A853"),
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
        }
        .frame(width: 20, height: 20)
    }
}

// MARK: - NabuTextFieldStyle
// Matches PWA: 1px solid border, 8px radius, 10px 14px padding, white bg

struct NabuTextFieldStyle: TextFieldStyle {
    func _body(configuration: TextField<Self._Label>) -> some View {
        configuration
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .background(DesignColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(DesignColors.border, lineWidth: 1)
            )
    }
}

// MARK: - LabeledField
// Matches PWA .form-group: label above the input

struct LabeledField<Field: View>: View {
    let label: String
    let field: Field

    init(_ label: String, @ViewBuilder field: () -> Field) {
        self.label = label
        self.field = field()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label)
                .font(.subheadline)
                .foregroundColor(DesignColors.textPrimary)
            field
        }
    }
}

// MARK: - AuthCard
// Matches PWA .auth-card: white card, max-width, centered, shadow

struct AuthCard<Content: View>: View {
    let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        content
            .padding(24)
            .frame(maxWidth: 400)
            .background(DesignColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .shadow(color: .black.opacity(0.07), radius: 6, x: 0, y: 4)
            .padding(.horizontal, 16)
    }
}

// MARK: - PrimaryButton style
// Matches PWA .btn-primary: teal bg, white text, 8px radius, 44px min-height

struct NabuPrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .frame(maxWidth: .infinity)
            .frame(minHeight: 44)
            .padding(.horizontal, 16)
            .background(isEnabled ? DesignColors.primary : DesignColors.primary.opacity(0.4))
            .foregroundColor(.white)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .scaleEffect(configuration.isPressed ? 0.97 : 1.0)
            .animation(.easeInOut(duration: 0.1), value: configuration.isPressed)
    }
}

// MARK: - SecondaryButton style
// Matches PWA .btn-secondary: beige bg, border, text

struct NabuSecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .frame(maxWidth: .infinity)
            .frame(minHeight: 44)
            .padding(.horizontal, 16)
            .background(DesignColors.pageBackground)
            .foregroundColor(DesignColors.textPrimary)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(DesignColors.border, lineWidth: 1)
            )
            .scaleEffect(configuration.isPressed ? 0.97 : 1.0)
            .animation(.easeInOut(duration: 0.1), value: configuration.isPressed)
    }
}

// MARK: - GoogleButton style
// Matches PWA .btn-google: white bg, #dadce0 border, centered

struct NabuGoogleButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .frame(maxWidth: .infinity)
            .frame(minHeight: 44)
            .padding(.horizontal, 16)
            .background(DesignColors.surface)
            .foregroundColor(Color(hexUnsafe: "3c4043"))
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(Color(hexUnsafe: "DADCE0"), lineWidth: 1)
            )
            .scaleEffect(configuration.isPressed ? 0.97 : 1.0)
            .animation(.easeInOut(duration: 0.1), value: configuration.isPressed)
    }
}

// MARK: - PillTabBar
// Matches PWA pill-style switcher: white active pill on a slightly gray track

struct PillTabBar<T: Hashable>: View {
    @Binding var selection: T
    let tabs: [T]
    let labelFor: (T) -> String

    var body: some View {
        HStack(spacing: 0) {
            ForEach(tabs, id: \.self) { tab in
                Text(labelFor(tab))
                    .font(.subheadline)
                    .fontWeight(selection == tab ? .semibold : .regular)
                    .padding(.horizontal, 20)
                    .padding(.vertical, 8)
                    .frame(maxWidth: .infinity)
                    .frame(minHeight: 44) // ≥44pt hit target (C6)
                    .background(
                        selection == tab
                            ? DesignColors.surface
                            : Color.clear
                    )
                    .clipShape(Capsule())
                    .foregroundColor(
                        selection == tab ? DesignColors.primary : DesignColors.textSecondary
                    )
                    .contentShape(Capsule())
                    .onTapGesture {
                        withAnimation(.easeInOut(duration: 0.15)) { selection = tab }
                    }
                    // One element per pill with the full padded frame — a
                    // plain Button here vends only the text glyph bounds,
                    // which fails the ≥44pt hit-target audit (C6).
                    .accessibilityElement()
                    .accessibilityLabel(labelFor(tab))
                    .accessibilityAddTraits(selection == tab ? [.isButton, .isSelected] : .isButton)
                    .accessibilityAction {
                        selection = tab
                    }
            }
        }
        .padding(4)
        .background(DesignColors.surfaceSecondary)
        .clipShape(Capsule())
    }
}

// MARK: - Motion

/// Standard springs for the app's motion language (P6/C3). Every accessor
/// collapses to nil (no animation) when the user has Reduce Motion on or a
/// UI test passed -disableAnimations, so callers can write
/// `withAnimation(Motion.snappy) { … }` and get the fade-or-nothing fallback
/// for free.
enum Motion {
    static var reduceMotion: Bool {
        UIAccessibility.isReduceMotionEnabled || TestHooks.disableAnimations
    }

    /// Snappy spring for small interactive elements (tile bounce, chips).
    static var snappy: Animation? {
        reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.55)
    }

    /// Softer spring for surfaces sliding in (toasts, banners).
    static var slide: Animation? {
        reduceMotion ? nil : .spring(response: 0.42, dampingFraction: 0.8)
    }

    /// Fade used as the Reduce Motion stand-in for movement.
    static var fade: Animation {
        .easeOut(duration: 0.18)
    }
}

// MARK: - State quality (C4)

/// Full-screen loading skeleton drawn on the real layout — a header bar and a
/// grid of tile-shaped placeholders — instead of a centered spinner.
struct SkeletonScreen: View {
    var body: some View {
        VStack(spacing: 16) {
            RoundedRectangle(cornerRadius: 8)
                .fill(DesignColors.surfaceSecondary)
                .frame(height: 28)
                .padding(.horizontal, 60)
                .padding(.top, 24)

            LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 12), count: 3), spacing: 12) {
                ForEach(0..<9, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: 18)
                        .fill(DesignColors.surfaceSecondary)
                        .frame(height: 90)
                }
            }
            .padding()

            Spacer()
        }
        .redacted(reason: .placeholder)
        .accessibilityLabel("Loading")
    }
}

/// Loading skeleton for a card-based tab (Stats): stacked card placeholders.
struct SkeletonCards: View {
    var count: Int = 4

    var body: some View {
        ScrollView {
            VStack(spacing: 16) {
                ForEach(0..<count, id: \.self) { i in
                    RoundedRectangle(cornerRadius: 14)
                        .fill(DesignColors.surfaceSecondary)
                        .frame(height: i == 0 ? 90 : 180)
                }
            }
            .padding()
        }
        .redacted(reason: .placeholder)
        .scrollDisabled(true)
        .accessibilityLabel("Loading")
    }
}

/// Inline error with the decoded server message and a retry action — never a
/// dead screen (C4).
struct InlineErrorView: View {
    let message: String
    let retry: () async -> Void

    var body: some View {
        ContentUnavailableView {
            Label("Couldn't load", systemImage: "wifi.exclamationmark")
        } description: {
            Text(message)
        } actions: {
            Button("Try Again") {
                Task { await retry() }
            }
            .buttonStyle(.borderedProminent)
        }
    }
}

/// Global connectivity banner. Log writes keep working offline via the
/// idempotent queue; the banner says so instead of pretending it's an error.
struct OfflineBanner: View {
    /// Home is the copy saved on this device (see `AppState.showingSavedHome`).
    var savedHome = false

    var body: some View {
        Label(savedHome ? "Offline — showing what's saved on this device. Logs you add sync when you reconnect."
                        : "Offline — logs you add will sync when you're back online",
              systemImage: "wifi.slash")
            .font(.footnote.weight(.medium))
            .foregroundColor(.white)
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(Color.black.opacity(0.82))
            .clipShape(Capsule())
            .padding(.horizontal, 16)
            .accessibilityIdentifier("offline-banner")
    }
}

// MARK: - QR code
// The PWA draws the same code with qr.js: error correction M, dark on white.

/// A QR code for a link, dark on white whatever the theme so cameras read it,
/// with the standard four-module quiet zone.
struct QRCodeView: View {
    let text: String
    var label = "QR code for the invite link"

    var body: some View {
        if let image = Self.image(for: text) {
            Image(uiImage: image)
                .interpolation(.none)
                .resizable()
                .scaledToFit()
                .frame(maxWidth: 220)
                .background(Color.white)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(DesignColors.border))
                .accessibilityElement()
                .accessibilityLabel(label)
                .accessibilityIdentifier("invite-qr")
        }
    }

    /// The code as an image, 8 pixels per module, quiet zone included.
    static func image(for text: String) -> UIImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(text.utf8)
        filter.correctionLevel = "M"
        guard let code = filter.outputImage else { return nil }
        // Core Image adds a one-module margin; pad to the standard four.
        let quiet = code.transformed(by: CGAffineTransform(translationX: 3, y: 3))
            .composited(over: CIImage(color: .white).cropped(to: code.extent.insetBy(dx: -3, dy: -3).offsetBy(dx: 3, dy: 3)))
        let scaled = quiet.transformed(by: CGAffineTransform(scaleX: 8, y: 8))
        guard let cgImage = CIContext().createCGImage(scaled, from: scaled.extent) else { return nil }
        return UIImage(cgImage: cgImage)
    }
}
