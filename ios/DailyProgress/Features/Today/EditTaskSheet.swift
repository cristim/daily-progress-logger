import SwiftUI

// MARK: - EditTaskSheet

struct EditTaskSheet: View {
    let task: TreeTask
    let store: TodayStore
    let appState: AppState

    @Environment(\.dismiss) private var dismiss
    @State private var text: String
    @FocusState private var focused: Bool

    init(task: TreeTask, store: TodayStore, appState: AppState) {
        self.task = task
        self.store = store
        self.appState = appState
        _text = State(initialValue: task.text)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Task text", text: $text, axis: .vertical)
                        .focused($focused)
                        .lineLimit(2...8)
                }
            }
            .navigationTitle("Edit Task")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { submit() }
                        .disabled(text.trimmingCharacters(in: .whitespaces).isEmpty ||
                                  text == task.text)
                }
            }
            .onAppear { focused = true }
        }
    }

    private func submit() {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, trimmed != task.text else { return }
        dismiss()
        Task {
            await store.editText(task: task, newText: trimmed, appState: appState)
        }
    }
}
