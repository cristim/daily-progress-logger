import SwiftUI

// MARK: - WeeklySummarySheet

/// Weekly summary sheet: goals (struck when done) + done-by-day breakdown + total count.
///
/// Scheduled path (presentation == .scheduled):
///   Loads the oldest unsummarized week via nextPendingSummaryWeek(), marks it
///   summarized on OK, then loops to the next pending week until none remain.
///   rollover semantics: the scheduled summary prompt always triggers after all
///   week-days have passed; marking summarized is idempotent.
///
/// Manual path (presentation == .manual):
///   Loads the summary for the store's current referenceDate week. OK dismisses
///   without calling markSummarized (matches Qt: manual view, no auto-mark).
struct WeeklySummarySheet: View {
    let store: WeekStore
    let appState: AppState
    let presentation: CheckinPresentation
    var onComplete: () -> Void
    var onSnooze: () -> Void
    var onSkipOrClose: () -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var isApplying = false

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Weekly Summary")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    CheckinButtonsBar(
                        presentation: presentation,
                        isApplying: isApplying,
                        onOK: applyAndAdvance,
                        onSnooze: { onSnooze(); dismiss() },
                        onSkipOrClose: { onSkipOrClose(); dismiss() }
                    )
                }
                .task { await loadContent() }
                .alert("Error", isPresented: Binding(
                    get: { store.errorMessage != nil },
                    set: { if !$0 { store.errorMessage = nil } }
                )) {
                    Button("OK", role: .cancel) { store.errorMessage = nil }
                } message: {
                    if let msg = store.errorMessage { Text(msg) }
                }
        }
    }

    // MARK: - Content

    @ViewBuilder
    private var content: some View {
        if store.isLoading && store.summary == nil {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let summary = store.summary {
            summaryContent(summary)
        } else {
            // Loading failed with no content — inline error already shown via alert.
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func summaryContent(_ summary: WeeklySummary) -> some View {
        let total = summary.doneByDay.reduce(0) { $0 + $1.items.count }
        return List {
            // Goals section (struck through when done)
            if !summary.goals.isEmpty {
                Section("Goals") {
                    ForEach(Array(summary.goals.enumerated()), id: \.offset) { _, goal in
                        Text(goal.text)
                            .strikethrough(goal.done)
                            .foregroundStyle(goal.done ? .secondary : .primary)
                    }
                }
            }

            // Done-by-day breakdown
            if summary.doneByDay.isEmpty {
                Section("Done this week") {
                    Text("Nothing completed yet this week.")
                        .foregroundStyle(.secondary)
                }
            } else {
                ForEach(summary.doneByDay, id: \.date) { day in
                    Section(header: Text(DateFormatting.dayHeader(from: day.date))) {
                        ForEach(day.items, id: \.self) { item in
                            Label(item, systemImage: "checkmark")
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                Section(footer: Text("Total: \(total) item\(total == 1 ? "" : "s") this week")) {
                    EmptyView()
                }
            }
        }
    }

    // MARK: - Load

    private func loadContent() async {
        if presentation == .manual {
            // Manual: use the store's current referenceDate (week currently viewed).
            // If summary not yet loaded, trigger a refresh.
            if store.summary == nil {
                await store.refresh()
            }
        } else {
            // Scheduled: find the oldest week with a pending (unsummarized) summary.
            if let monday = await store.nextPendingSummaryWeek() {
                store.referenceDate = monday
                await store.refresh()
            } else {
                // Nothing pending — prompt was stale; dismiss silently.
                dismiss()
            }
        }
    }

    // MARK: - Apply

    private func applyAndAdvance() {
        isApplying = true
        Task {
            if presentation == .scheduled {
                // Mark the current week as summarized, then advance to the next pending week.
                let ok = await store.markSummarized(date: store.referenceDate.coreDate)
                if ok {
                    if let nextMonday = await store.nextPendingSummaryWeek() {
                        // Continue the loop with the next pending week.
                        store.referenceDate = nextMonday
                        await store.refresh()
                        isApplying = false
                    } else {
                        // Loop exhausted — done.
                        isApplying = false
                        onComplete()
                        dismiss()
                    }
                } else {
                    // markSummarized failed; store.errorMessage is set; sheet stays open.
                    isApplying = false
                }
            } else {
                // Manual path: no mark-summarized on OK (matches Qt manual view behaviour).
                isApplying = false
                onComplete()
                dismiss()
            }
        }
    }
}
