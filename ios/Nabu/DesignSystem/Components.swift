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
                .font(.system(size: 16, weight: .bold, design: .rounded))
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
                Button {
                    withAnimation(.easeInOut(duration: 0.15)) { selection = tab }
                } label: {
                    Text(labelFor(tab))
                        .font(.subheadline)
                        .fontWeight(selection == tab ? .semibold : .regular)
                        .padding(.horizontal, 20)
                        .padding(.vertical, 8)
                        .frame(maxWidth: .infinity)
                        .background(
                            selection == tab
                                ? DesignColors.surface
                                : Color.clear
                        )
                        .clipShape(Capsule())
                        .foregroundColor(
                            selection == tab ? DesignColors.primary : DesignColors.textSecondary
                        )
                }
                .buttonStyle(.plain)
            }
        }
        .padding(4)
        .background(DesignColors.surfaceSecondary)
        .clipShape(Capsule())
    }
}
