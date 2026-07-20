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
    /// Local done-state for each existing goal; synced from store on load.
    /// Toggles are deferred to OK so Snooze/Skip discards them (matches Qt/Android).
    @State private var localGoalDoneStates: [Bool] = []

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
                .task {
                    await store.refresh()
                    // Sync local done-states to whatever the store loaded.
                    localGoalDoneStates = store.plan?.goals.map(\.done) ?? []
                }
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
    // Toggles write to localGoalDoneStates only; applied to the store on OK (C3).
    @ViewBuilder
    private var currentGoalsSection: some View {
        if let goals = store.plan?.goals, !goals.isEmpty {
            Section("Current goals") {
                ForEach(Array(goals.enumerated()), id: \.offset) { index, goal in
                    let isDone = localGoalDoneStates.indices.contains(index)
                        ? localGoalDoneStates[index] : goal.done
                    Toggle(goal.text, isOn: Binding(
                        get: { isDone },
                        set: { done in
                            if localGoalDoneStates.indices.contains(index) {
                                localGoalDoneStates[index] = done
                            }
                        }
                    ))
                    .strikethrough(isDone)
                    .foregroundStyle(isDone ? .secondary : .primary)
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
            // Build new goals from the text editor (one per line).
            let trimmed = newGoalsText.trimmingCharacters(in: .whitespacesAndNewlines)
            let newGoals: [WeeklyGoal] = trimmed.isEmpty ? [] :
                trimmed.split(separator: "\n", omittingEmptySubsequences: true)
                    .map { String($0).trimmingCharacters(in: .whitespaces) }
                    .filter { !$0.isEmpty }
                    .map { WeeklyGoal(text: $0, done: false) }
            // Apply local done-states to existing goals, then append new goals.
            var goals = store.plan?.goals ?? []
            for i in goals.indices where localGoalDoneStates.indices.contains(i) {
                goals[i].done = localGoalDoneStates[i]
            }
            goals.append(contentsOf: newGoals)
            // Always call savePlan — even when goals is [] — so the week is marked
            // planned and the scheduled prompt stops firing (I1: empty-OK case).
            await store.savePlan(goals: goals)
            isApplying = false
            // If an error occurred, stay open (rule 4).
            if store.errorMessage == nil {
                onComplete()
                dismiss()
            }
        }
    }
}
