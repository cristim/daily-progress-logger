import SwiftUI

// MARK: - AddTaskSheet

struct AddTaskSheet: View {
    let date: Date
    let store: TodayStore
    let appState: AppState
    /// Optional project to pre-select.
    var preselectedProjectID: String? = nil

    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var selectedProjectID: String? = nil
    @State private var projects: [Project] = []
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section("Task") {
                    TextField("What needs to be done?", text: $text, axis: .vertical)
                        .focused($focused)
                        .lineLimit(3...8)
                }
                if !projects.isEmpty {
                    Section("Project (optional)") {
                        Picker("Project", selection: $selectedProjectID) {
                            Text("None (Unfiled)").tag(Optional<String>.none)
                            ForEach(projects.filter { $0.status == .open }) { project in
                                Text(project.name).tag(Optional(project.id))
                            }
                        }
                    }
                }
            }
            .navigationTitle("Add Task")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") { submit() }
                        .disabled(text.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            .task {
                await loadProjects()
                selectedProjectID = preselectedProjectID
                focused = true
            }
        }
    }

    private func submit() {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return }
        dismiss()
        Task {
            await store.addTask(
                date: date,
                text: trimmed,
                projectID: selectedProjectID ?? "",
                appState: appState
            )
        }
    }

    private func loadProjects() async {
        guard let projectsJSON = try? await (appState.core)?.projectsJSON() else { return }
        projects = (try? JSONDecoder().decode([Project].self, from: Data((projectsJSON).utf8))) ?? []
    }
}

// MARK: - AddSubtaskSheet

struct AddSubtaskSheet: View {
    let parent: TreeTask
    let store: TodayStore
    let appState: AppState

    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section("Subtask of \"\(parent.text)\"") {
                    TextField("Subtask text", text: $text, axis: .vertical)
                        .focused($focused)
                        .lineLimit(2...6)
                }
            }
            .navigationTitle("Add Subtask")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") { submit() }
                        .disabled(text.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            .onAppear { focused = true }
        }
    }

    private func submit() {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return }
        dismiss()
        Task {
            await store.addSubtask(parent: parent, text: trimmed, appState: appState)
        }
    }
}
