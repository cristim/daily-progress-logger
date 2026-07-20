import SwiftUI

// MARK: - CheckinButtonsBar

/// Reusable toolbar button row shared by all prompt sheets (check-in and weekly).
/// Provides the standard OK / Remind me in 1h / Skip Today-or-Close layout.
///
/// Callers supply closures for each button. Dismiss is the caller's responsibility
/// so each sheet controls its own lifecycle (e.g. WeekReviewSheet loops before dismissing).
struct CheckinButtonsBar: ToolbarContent {
    let presentation: CheckinPresentation
    let isApplying: Bool

    /// Called when OK is tapped; the closure handles async work and calls dismiss() when ready.
    var onOK: () -> Void
    /// Called when "Remind me in 1h" is tapped; the closure records the snooze then dismisses.
    var onSnooze: () -> Void
    /// Called when Skip Today (scheduled) or Close (manual) is tapped; the closure dismisses.
    var onSkipOrClose: () -> Void

    var body: some ToolbarContent {
        // Left: Skip Today (scheduled) or Close (manual) — no bookkeeping on manual Close
        ToolbarItem(placement: .cancellationAction) {
            Button(presentation == .scheduled ? "Skip Today" : "Close") {
                onSkipOrClose()
            }
        }

        // Bottom bar: Snooze
        ToolbarItem(placement: .bottomBar) {
            Button("Remind me in 1h") {
                onSnooze()
            }
        }

        // Right: OK or in-progress spinner
        ToolbarItem(placement: .primaryAction) {
            if isApplying {
                ProgressView()
            } else {
                Button("OK") { onOK() }
            }
        }
    }
}
