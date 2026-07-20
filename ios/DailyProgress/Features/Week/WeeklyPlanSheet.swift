import SwiftUI

// MARK: - WeeklyPlanSheet

/// "What are you planning to work on this week?"
/// Presented when the weekly-plan prompt fires (scheduled) or manually from WeekView toolbar.
/// Shows current goals with done/not-done toggles and a text editor for adding new ones.
/// OK saves the full goals array; Snooze/Skip dismiss without saving.
struct WeeklyPlanSheet: View {
    let store: WeekStore
    let appState: AppState
    let presentation: CheckinPresentation
    var onComplete: () -> Void
    var onSnooze: () -> Void
    var onSkipOrClose: () -> Void

    @Environment(\.dismiss) private var dismiss
    /// Captured once on appear so edits always apply to the same week even if held open at midnight.
    @State private var isApplying = false
    @State private var newGoalsText = ""

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Weekly Plan")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    CheckinButtonsBar(
                        presentation: presentation,
                        isApplying: isApplying,
                        onOK: applyAndDismiss,
                        onSnooze: { onSnooze(); dismiss() },
                        onSkipOrClose: { onSkipOrClose(); dismiss() }
                    )
                }
                .task { await store.refresh() }
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
        if store.isLoading && store.plan == nil {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            Form {
                currentGoalsSection
                addGoalsSection
            }
        }
    }

    // Existing goals with done/not-done toggles (struck through when done).
    // Uses enumerated() id — duplicate goal texts are legal (plan rule).
    @ViewBuilder
    private var currentGoalsSection: some View {
        if let goals = store.plan?.goals, !goals.isEmpty {
            Section("Current goals") {
                ForEach(Array(goals.enumerated()), id: \.offset) { index, goal in
                    Toggle(goal.text, isOn: Binding(
                        get: {
                            guard let gs = store.plan?.goals, gs.indices.contains(index) else { return false }
                            return gs[index].done
                        },
                        set: { done in
                            Task { await store.setGoalDone(index: index, done: done) }
                        }
                    ))
                    .strikethrough(goal.done)
                    .foregroundStyle(goal.done ? .secondary : .primary)
                }
            }
        }
    }

    private var addGoalsSection: some View {
        Section("Add goals this week (one per line)") {
            TextEditor(text: $newGoalsText)
                .frame(minHeight: 100)
        }
    }

    // MARK: - Actions

    private func applyAndDismiss() {
        isApplying = true
        Task {
            let trimmed = newGoalsText.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.isEmpty {
                // Appends new lines to existing goals and saves the full array.
                await store.addGoals(text: trimmed)
            }
            isApplying = false
            // If an error occurred during addGoals, stay open (rule 4).
            if store.errorMessage == nil {
                onComplete()
                dismiss()
            }
        }
    }
}
