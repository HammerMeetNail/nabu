import SwiftUI

struct HomeGrid: View {
    let chores: [Chore]
    let latestLogs: [Int: ChoreLog]
    let isJiggling: Bool
    let onTap: (Chore) -> Void
    let onLongPress: (Chore) -> Void

    private let columns = [
        GridItem(.flexible(), spacing: 12),
        GridItem(.flexible(), spacing: 12),
        GridItem(.flexible(), spacing: 12)
    ]

    var body: some View {
        LazyVGrid(columns: columns, spacing: 12) {
            ForEach(chores) { chore in
                HomeChoreCell(
                    chore: chore,
                    latestLog: latestLogs[chore.id],
                    isJiggling: isJiggling,
                    onTap: { onTap(chore) },
                    onLongPress: { onLongPress(chore) }
                )
            }
        }
    }
}

struct HomeChoreCell: View {
    let chore: Chore
    let latestLog: ChoreLog?
    let isJiggling: Bool
    let onTap: () -> Void
    let onLongPress: () -> Void

    @State private var isPressing = false

    var body: some View {
        Button {
            onTap()
        } label: {
            // HStack approach: colored left stripe + content
            // clipShape rounds both together — no clipping bug
            HStack(spacing: 0) {
                Rectangle()
                    .fill(Color(hex: chore.color) ?? .accentColor)
                    .frame(width: 4)

                VStack(spacing: 4) {
                    Text(chore.icon)
                        .font(.system(size: 24))
                    Text(chore.name)
                        .font(.caption)
                        .fontWeight(.semibold)
                        .lineLimit(2)
                        .multilineTextAlignment(.center)
                    if let log = latestLog {
                        Text(formatTimeAgo(log.completedAt))
                            .font(.caption2)
                            .foregroundColor(.secondary)
                    } else {
                        Text("never")
                            .font(.caption2)
                            .foregroundColor(DesignColors.textSecondary)
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 10)
                .padding(.horizontal, 6)
            }
            .frame(minHeight: 90)
            .background(DesignColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: 18))
            .overlay(
                RoundedRectangle(cornerRadius: 18)
                    .stroke(DesignColors.border.opacity(0.3), lineWidth: 1)
            )
            .shadow(color: .black.opacity(0.08), radius: 3, x: 0, y: 1)
            .scaleEffect(isPressing ? 0.92 : 1.0)
            .opacity(isPressing ? 0.75 : 1.0)
            .animation(.easeInOut(duration: 0.1), value: isPressing)
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(chore.name), \(latestLog != nil ? "done \(formatTimeAgo(latestLog!.completedAt))" : "never done")")
        .simultaneousGesture(
            LongPressGesture(minimumDuration: 0.5)
                .onEnded { _ in
                    onLongPress()
                }
        )
        .onLongPressGesture(minimumDuration: 0.1, perform: {}) {
            isPressing = $0
        }
    }

}
