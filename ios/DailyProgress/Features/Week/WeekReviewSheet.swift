import SwiftUI

// MARK: - WeekReviewSheet

/// Week review sheet: list of open items from a past week, each with a 3-way
/// Keep-on-backlog / Postpone-to-next-week / Drop selector.
///
/// Scheduled path (presentation == .scheduled):
///   Queries UnreviewedWeekJSON for the oldest unreviewed week, presents its
///   candidates, applies decisions with rollover=true, then loops to the next
///   unreviewed week until none remain (oldest-first, per Qt runWeekReviewLoop).
///
/// Manual path (presentation == .manual):
///   Loads candidates for reviewDate (caller provides "previous week" date).
///   Applies with rollover=false. Single pass — no loop.
struct WeekReviewSheet: View {
    let store: WeekStore
    let appState: AppState
    let presentation: CheckinPresentation
    /// For the manual path: any date inside the week to review.
    /// For the scheduled path: ignored — the sheet queries nextUnreviewedWeek() itself.
    let reviewDate: String
    /// rollover=true for scheduled (Monday review loop); false for manual "Review Last Week…".
    let rollover: Bool
    var onComplete: () -> Void
    var onSnooze: () -> Void
    var onSkipOrClose: () -> Void

    @Environment(\.dismiss) private var dismiss
    /// The date we are currently reviewing (may advance through the loop).
    @State private var currentDate: String
    @State private var isApplying = false

    init(
        store: WeekStore,
        appState: AppState,
        presentation: CheckinPresentation,
        reviewDate: String,
        rollover: Bool,
        onComplete: @escaping () -> Void,
        onSnooze: @escaping () -> Void,
        onSkipOrClose: @escaping () -> Void
    ) {
        self.store = store
        self.appState = appState
        self.presentation = presentation
        self.reviewDate = reviewDate
        self.rollover = rollover
        self.onComplete = onComplete
        self.onSnooze = onSnooze
        self.onSkipOrClose = onSkipOrClose
        _currentDate = State(initialValue: reviewDate)
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Week Review")
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
                .task { await loadCurrentWeek() }
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
        if store.isLoading && store.reviewCandidates == nil {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let candidates = store.reviewCandidates, candidates.candidates.isEmpty {
            emptyState(week: candidates.week)
        } else if store.reviewCandidates != nil {
            candidateList
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func emptyState(week: String) -> some View {
        ContentUnavailableView(
            "Nothing left open",
            systemImage: "checkmark.circle.fill",
            description: Text("Nothing left open from \(week). Great job!")
        )
    }

    private var candidateList: some View {
        let candidates = store.reviewCandidates?.candidates ?? []
        let weekLabel = store.reviewCandidates?.week ?? currentDate
        return List {
            Section("Open items from \(weekLabel)") {
                // enumerated() id — candidates list may contain duplicate texts.
                ForEach(Array(candidates.enumerated()), id: \.offset) { index, text in
                    ReviewCandidateRow(
                        text: text,
                        action: Binding(
                            get: {
                                store.reviewActions.indices.contains(index)
                                    ? store.reviewActions[index] : .keep
                            },
                            set: { newAction in
                                if store.reviewActions.indices.contains(index) {
                                    store.reviewActions[index] = newAction
                                }
                            }
                        )
                    )
                }
            }
        }
    }

    // MARK: - Load

    private func loadCurrentWeek() async {
        if presentation == .scheduled {
            // Scheduled: always start from the oldest unreviewed week.
            if let monday = await store.nextUnreviewedWeek() {
                currentDate = monday.coreDate
                await store.loadReview(date: monday.coreDate)
            } else {
                // No unreviewed weeks (prompt was stale); dismiss silently.
                dismiss()
            }
        } else {
            // Manual: load candidates for the provided reviewDate.
            await store.loadReview(date: currentDate)
        }
    }

    // MARK: - Apply and advance

    private func applyAndAdvance() {
        isApplying = true
        Task {
            let ok = await store.applyReview(date: currentDate, rollover: rollover)
            if ok {
                if presentation == .scheduled {
                    // Check for more unreviewed weeks (oldest-first loop).
                    if let nextMonday = await store.nextUnreviewedWeek() {
                        currentDate = nextMonday.coreDate
                        await store.loadReview(date: nextMonday.coreDate)
                        isApplying = false
                    } else {
                        // Loop exhausted.
                        isApplying = false
                        onComplete()
                        dismiss()
                    }
                } else {
                    // Manual: single pass.
                    isApplying = false
                    onComplete()
                    dismiss()
                }
            } else {
                // Apply failed; store.errorMessage set; sheet stays open (rule 4).
                isApplying = false
            }
        }
    }
}

// MARK: - ReviewCandidateRow

/// Single candidate row with a 3-segment Keep / Postpone / Drop picker.
private struct ReviewCandidateRow: View {
    let text: String
    @Binding var action: ReviewAction

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(text)
            Picker("Action", selection: $action) {
                Text("Keep").tag(ReviewAction.keep)
                Text("Postpone").tag(ReviewAction.postpone)
                Text("Drop").tag(ReviewAction.drop)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
        }
        .padding(.vertical, 2)
    }
}
