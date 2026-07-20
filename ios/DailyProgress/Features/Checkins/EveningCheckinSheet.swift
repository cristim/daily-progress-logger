import SwiftUI

// MARK: - EveningCheckinSheet

/// "How did today go?"
/// One row per plan item (flattened pre-order from TreeJSON), each with a
/// 5-way segmented selector: Done / Not done / Next day / Next week / Backlog.
/// A TextEditor at the bottom collects any extra done items.
/// Toolbar: OK / Remind me in 1h / Skip Today (scheduled) or Close (manual).
struct EveningCheckinSheet: View {
    let store: CheckinStore
    let appState: AppState
    let presentation: CheckinPresentation
    var onComplete: () -> Void
    var onSnooze: () -> Void
    var onSkipOrClose: () -> Void

    @Environment(\.dismiss) private var dismiss
    /// Captured once when the sheet appears so load and apply always use the same date,
    /// even if the sheet is held open across midnight.
    @State private var date = Date()
    @State private var extraText = ""
    @State private var isApplying = false

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Evening Check-in")
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
                .task { await store.loadEvening(date: date) }
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
        if store.isLoading && store.eveningItems.isEmpty && store.errorMessage == nil {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            Form {
                tasksSection
                extraDoneSection
            }
        }
    }

    // Per-task 5-way selector rows
    @ViewBuilder
    private var tasksSection: some View {
        if store.eveningItems.isEmpty {
            Section {
                Text("No plan was recorded for today.")
                    .foregroundStyle(.secondary)
            }
        } else {
            Section("How did today go?") {
                // Array index as id — duplicate texts are legal (plan rule §6)
                ForEach(Array(store.eveningItems.enumerated()), id: \.offset) { index, item in
                    EveningItemRow(
                        item: item,
                        onActionChange: { newAction in
                            store.eveningItems[index].action = newAction
                        }
                    )
                }
            }
        }
    }

    private var extraDoneSection: some View {
        Section("Anything else you accomplished?") {
            TextEditor(text: $extraText)
                .frame(minHeight: 80)
        }
    }

    // MARK: - Actions

    private func applyAndDismiss() {
        isApplying = true
        Task {
            let ok = await store.applyEvening(date: date, extraText: extraText)
            isApplying = false
            if ok {
                onComplete()
                dismiss()
            }
            // On failure: store.errorMessage is set; sheet stays open (rule 4)
        }
    }
}

// MARK: - EveningItemRow

/// Single row in the evening check-in list.
/// Indents text by depth, then shows a 5-icon segmented picker for the action.
private struct EveningItemRow: View {
    let item: CheckinStore.EveningItem
    let onActionChange: (EveningAction) -> Void

    // SF Symbol names per action (matches Qt's icon choices)
    private static let actionSymbols: [(EveningAction, String, String)] = [
        (.done,     "checkmark",   "Done"),
        (.todo,     "xmark",       "Not done"),
        (.nextDay,  "arrow.right", "Next day"),
        (.nextWeek, "arrow.up",    "Next week"),
        (.backlog,  "tray",        "Backlog"),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(item.text)
                .padding(.leading, CGFloat(item.depth) * 16)

            Picker("Action for \(item.text)", selection: Binding(
                get: { item.action },
                set: { onActionChange($0) }
            )) {
                ForEach(Self.actionSymbols, id: \.0.rawValue) { action, symbol, label in
                    Image(systemName: symbol)
                        .accessibilityLabel(label)
                        .tag(action)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
        }
        .padding(.vertical, 4)
    }
}
