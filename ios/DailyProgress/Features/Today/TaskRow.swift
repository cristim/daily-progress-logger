import SwiftUI

// MARK: - TaskRow

/// A single row in the today tree. Renders checkbox + text with indentation,
/// and provides swipe + context-menu actions.
struct TaskRow: View {
    let task: TreeTask
    let store: TodayStore
    let appState: AppState

    @State private var showEditSheet = false
    @State private var showAddSubtaskSheet = false

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            // Indentation for subtasks
            if task.depth > 0 {
                Spacer().frame(width: CGFloat(task.depth) * 20)
            }

            // Checkbox
            Button {
                Task { await store.toggleState(task: task, appState: appState) }
            } label: {
                Image(systemName: checkboxIcon)
                    .foregroundStyle(checkboxColor)
            }
            .buttonStyle(.plain)

            // Task text
            VStack(alignment: .leading, spacing: 2) {
                Text(task.text)
                    .strikethrough(task.done, color: .secondary)
                    .foregroundStyle(task.done ? .secondary : .primary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .contentShape(Rectangle())
        .swipeActions(edge: .leading) {
            Button {
                Task { await store.toggleState(task: task, appState: appState) }
            } label: {
                Label(task.state == .done ? "Undo" : "Done",
                      systemImage: task.state == .done ? "arrow.uturn.backward" : "checkmark")
            }
            .tint(task.state == .done ? .orange : .green)
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                Task { await store.delete(task: task, appState: appState) }
            } label: {
                Label("Delete", systemImage: "trash")
            }
            Button {
                Task { await store.postponeToNextDay(task: task, appState: appState) }
            } label: {
                Label("Tomorrow", systemImage: "arrow.right.circle")
            }
            .tint(.orange)
        }
        .contextMenu {
            Button("Edit") { showEditSheet = true }
            Button("Add Subtask") { showAddSubtaskSheet = true }
            Divider()
            Button("Postpone to Tomorrow") {
                Task { await store.postponeToNextDay(task: task, appState: appState) }
            }
            Button("Postpone to Next Week") {
                Task { await store.postponeToNextWeek(task: task, appState: appState) }
            }
            Button("Move to Backlog") {
                Task { await store.moveToBacklog(task: task, appState: appState) }
            }
            Divider()
            Button("Mark Postponed") {
                Task { await store.markPostponed(task: task, appState: appState) }
            }
            Button("Delete", role: .destructive) {
                Task { await store.delete(task: task, appState: appState) }
            }
        }
        .sheet(isPresented: $showEditSheet) {
            EditTaskSheet(task: task, store: store, appState: appState)
        }
        .sheet(isPresented: $showAddSubtaskSheet) {
            AddSubtaskSheet(parent: task, store: store, appState: appState)
        }
    }

    private var checkboxIcon: String {
        switch task.state {
        case .done: return "checkmark.circle.fill"
        case .postponed: return "arrow.right.circle.fill"
        case .todo: return task.done ? "checkmark.circle.fill" : "circle"
        }
    }

    private var checkboxColor: Color {
        switch task.state {
        case .done: return .green
        case .postponed: return .orange
        case .todo: return task.done ? .green : .primary
        }
    }
}
