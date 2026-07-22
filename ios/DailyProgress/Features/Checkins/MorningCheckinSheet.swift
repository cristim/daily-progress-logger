import SwiftUI

// MARK: - MorningCheckinSheet

/// "What are you planning to work on today?"
/// Shows weekly goals (read-only, done goals struck through), an already-planned
/// summary line when the tree has items, a free-text "one task per line" editor,
/// and a candidate checklist (backlog default-unchecked, carry-overs default-checked).
/// Toolbar: OK / Remind me in 1h / Skip Today (scheduled) or Close (manual).
struct MorningCheckinSheet: View {
    let store: CheckinStore
    let appState: AppState
    let presentation: CheckinPresentation
    /// Called after a successful apply (sheet should dismiss next).
    var onComplete: () -> Void
    var onSnooze: () -> Void
    /// Called when Skip Today (scheduled) or Close (manual) is tapped.
    var onSkipOrClose: () -> Void

    @Environment(\.dismiss) private var dismiss
    /// Captured once when the sheet appears so load and apply always use the same date,
    /// even if the sheet is held open across midnight.
    @State private var date = Date()
    @State private var newText = ""
    @State private var isApplying = false

    // Daily prompt tap-to-edit state
    @State private var isEditingPrompt = false
    @State private var promptDraft = ""
    @FocusState private var promptFieldFocused: Bool

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Morning Check-in")
                .navigationBarTitleDisplayMode(.inline)
                .task { await store.loadMorning(date: date) }
                .toolbar {
                    CheckinButtonsBar(
                        presentation: presentation,
                        isApplying: isApplying,
                        onOK: applyAndDismiss,
                        onSnooze: { onSnooze(); dismiss() },
                        onSkipOrClose: { onSkipOrClose(); dismiss() }
                    )
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
        if store.isLoading && store.morningCandidates == nil {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            Form {
                dailyPromptSection
                goalsSection
                plannedSummarySection
                newTasksSection
                candidatesSection
            }
        }
    }

    // Daily prompt: display mode shows the text (or a muted placeholder when unset);
    // tapping anywhere in the row switches to an inline TextField with Save/Cancel.
    @ViewBuilder
    private var dailyPromptSection: some View {
        Section {
            if isEditingPrompt {
                HStack {
                    TextField("Daily prompt", text: $promptDraft)
                        .focused($promptFieldFocused)
                        .submitLabel(.done)
                        .onSubmit { savePrompt() }
                    Button("Save") { savePrompt() }
                        .buttonStyle(.borderless)
                    Button("Cancel", role: .cancel) { cancelPromptEdit() }
                        .buttonStyle(.borderless)
                }
            } else {
                Text(store.dailyPrompt.isEmpty ? "Set a daily prompt…" : store.dailyPrompt)
                    .foregroundStyle(store.dailyPrompt.isEmpty ? .secondary : .primary)
                    .contentShape(Rectangle())
                    .onTapGesture { beginPromptEdit() }
            }
        }
    }

    // Weekly goals (read-only; struck through when done)
    @ViewBuilder
    private var goalsSection: some View {
        if !store.weeklyGoals.isEmpty {
            Section("This week's goals") {
                ForEach(Array(store.weeklyGoals.enumerated()), id: \.offset) { _, goal in
                    Text(goal.text)
                        .strikethrough(goal.done)
                        .foregroundStyle(goal.done ? .secondary : .primary)
                }
            }
        }
    }

    // Already planned summary (hidden when count is 0)
    @ViewBuilder
    private var plannedSummarySection: some View {
        if store.alreadyPlannedCount > 0 {
            Section {
                Text("Already planned today: \(store.alreadyPlannedCount) item\(store.alreadyPlannedCount == 1 ? "" : "s")")
                    .foregroundStyle(.secondary)
            }
        }
    }

    // Free-text "one task per line" input
    private var newTasksSection: some View {
        Section("What are you planning to work on today?") {
            TextEditor(text: $newText)
                .frame(minHeight: 100)
        }
    }

    // Candidate toggle list (carry-over items)
    @ViewBuilder
    private var candidatesSection: some View {
        if let candidates = store.morningCandidates, !candidates.isEmpty {
            Section("Carry-overs") {
                // Array index as stable id — duplicate texts are legal (plan rule §6)
                ForEach(Array(candidates.enumerated()), id: \.offset) { index, candidate in
                    Toggle(
                        candidate.fromBacklog
                            ? "\(candidate.text) (backlog)"
                            : candidate.text,
                        isOn: Binding(
                            get: {
                                store.adoptedFlags.indices.contains(index)
                                    ? store.adoptedFlags[index]
                                    : false
                            },
                            set: { newVal in
                                if store.adoptedFlags.indices.contains(index) {
                                    store.adoptedFlags[index] = newVal
                                }
                            }
                        )
                    )
                }
            }
        }
    }

    // MARK: - Actions

    private func beginPromptEdit() {
        promptDraft = store.dailyPrompt
        isEditingPrompt = true
        promptFieldFocused = true
    }

    private func cancelPromptEdit() {
        isEditingPrompt = false
        promptFieldFocused = false
    }

    private func savePrompt() {
        let trimmed = promptDraft.trimmingCharacters(in: .whitespacesAndNewlines)
        isEditingPrompt = false
        promptFieldFocused = false
        Task {
            await store.saveDailyPrompt(trimmed)
        }
    }

    private func applyAndDismiss() {
        isApplying = true
        Task {
            let ok = await store.applyMorning(date: date, newItemsText: newText)
            isApplying = false
            if ok {
                onComplete()
                dismiss()
            }
            // On failure: store.errorMessage is set; sheet stays open (rule 4)
        }
    }
}
