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
                    if let error = store.addFieldError {
                        Text(error)
                            .foregroundStyle(.red)
                            .font(.footnote)
                    }
                } footer: {
                    Text("""
                    Needs a recurrence tag: @daily, @weekday, @weekly (add a day like \
                    @fri and a time like @16:00), or @monthly @1 (day of month). Add \
                    @<project> to file it under a project.

                    Examples:
                    Standup @daily
                    Report @weekly @fri @16:00
                    Rent @monthly @1
                    """)
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
