import SwiftUI

// MARK: - AddRecurringSheet

/// Add a recurring template by typing text with an @-recurrence tag.
/// Syntax (see internal/recur): @daily, @weekday, @weekly [@<weekday>] [@HH:MM],
/// @monthly @<day-of-month> [@HH:MM]; an optional @<project-slug> tag files the
/// materialized occurrences under that project. BAD_INPUT (no recurrence tag,
/// or an empty description once tags are stripped) is surfaced inline under the
/// field and the sheet stays open — never silently dropped.
struct AddRecurringSheet: View {
    let store: RecurringStore

    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var isSaving = false
    @FocusState private var fieldFocused: Bool

    private var trimmedText: String { text.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Standup @daily @9:00", text: $text)
                        .focused($fieldFocused)
                        .submitLabel(.done)
                        .onSubmit(add)
                        .onChange(of: text) { _, _ in
                            store.addFieldError = nil
                        }
                    if let error = store.addFieldError {
                        Text(error)
                            .foregroundStyle(.red)
                            .font(.footnote)
                    }
                } footer: {
                    Text("Examples: @daily  \u{00B7}  @weekday  \u{00B7}  @weekly @mon @09:00  \u{00B7}  @monthly @15  \u{00B7}  optional @project")
                }
            }
            .navigationTitle("Add Recurring Task")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add", action: add)
                        .disabled(trimmedText.isEmpty || isSaving)
                }
            }
            .onAppear {
                store.addFieldError = nil
                fieldFocused = true
            }
        }
    }

    private func add() {
        guard !trimmedText.isEmpty else { return }
        isSaving = true
        Task {
            let ok = await store.add(text: trimmedText)
            isSaving = false
            if ok {
                dismiss()
            }
            // On failure: store.addFieldError is set; sheet stays open (fail-loud, no drop).
        }
    }
}
