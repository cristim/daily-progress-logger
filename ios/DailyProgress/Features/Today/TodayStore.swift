import Foundation
import Observation

// MARK: - TodayStore

@Observable
@MainActor
final class TodayStore {
    var tree: ProjectTree?
    var isLoading = false
    var errorMessage: String?
    /// Short-lived toast shown after a CAS mismatch or other recoverable error.
    var toast: String?

    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }

    // MARK: - Load

    func refresh(date: Date) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let json = try await core.treeJSON(date: date.coreDate)
            tree = try decode(ProjectTree.self, from: json)
        } catch {
            handleError(error)
        }
    }

    // MARK: - Task actions (CAS loop)

    func toggleState(task: TreeTask, appState: AppState) async {
        let newState: String = task.state == .done ? "todo" : "done"
        await mutate(task: task, appState: appState) {
            try await self.core.setTaskState(
                date: task.date,
                index: task.index,
                expectedText: task.text,
                state: newState
            )
        }
    }

    func markPostponed(task: TreeTask, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.setTaskState(
                date: task.date,
                index: task.index,
                expectedText: task.text,
                state: "postponed"
            )
        }
    }

    func delete(task: TreeTask, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.deleteTask(
                date: task.date,
                index: task.index,
                expectedText: task.text
            )
        }
    }

    func postponeToNextDay(task: TreeTask, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.postponeToNextDay(
                date: task.date,
                index: task.index,
                expectedText: task.text
            )
        }
    }

    func postponeToNextWeek(task: TreeTask, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.postponeToNextWeek(
                date: task.date,
                index: task.index,
                expectedText: task.text
            )
        }
    }

    func moveToBacklog(task: TreeTask, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.moveTaskToBacklog(
                date: task.date,
                index: task.index,
                expectedText: task.text
            )
        }
    }

    func moveToProject(task: TreeTask, projectID: String, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.moveTaskToProject(
                date: task.date,
                index: task.index,
                expectedText: task.text,
                projectID: projectID
            )
        }
    }

    func editText(task: TreeTask, newText: String, appState: AppState) async {
        await mutate(task: task, appState: appState) {
            try await self.core.editTaskText(
                date: task.date,
                index: task.index,
                expectedText: task.text,
                newText: newText
            )
        }
    }

    func addTask(date: Date, text: String, projectID: String, appState: AppState) async {
        do {
            try await core.addTask(date: date.coreDate, text: text, projectID: projectID)
            await refresh(date: date)
            appState.bumpDataVersion()
        } catch {
            handleError(error)
        }
    }

    func addSubtask(parent: TreeTask, text: String, appState: AppState) async {
        do {
            try await core.addSubtask(
                date: parent.date,
                parentIndex: parent.index,
                expectedParentText: parent.text,
                text: text
            )
            if let date = DateFormatting.date(from: parent.date) {
                await refresh(date: date)
            }
            appState.bumpDataVersion()
        } catch {
            handleError(error)
        }
    }

    // MARK: - Private

    /// Executes a mutating Core call, handles CAS_MISMATCH by refreshing + toasting,
    /// then refreshes the tree and bumps the data version on success.
    private func mutate(task: TreeTask, appState: AppState, action: @escaping () async throws -> Void) async {
        do {
            try await action()
            if let date = DateFormatting.date(from: task.date) {
                await refresh(date: date)
            }
            appState.bumpDataVersion()
        } catch CoreError.casMismatch {
            // Silently re-fetch; toast to prompt the user to retry.
            if let date = DateFormatting.date(from: task.date) {
                await refresh(date: date)
            }
            showToast("List changed — please retry.")
        } catch {
            handleError(error)
        }
    }

    private func handleError(_ error: Error) {
        if let coreError = error as? CoreError {
            errorMessage = coreError.errorDescription
        } else {
            errorMessage = error.localizedDescription
        }
    }

    private func showToast(_ message: String) {
        toast = message
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 3_000_000_000)
            if toast == message { toast = nil }
        }
    }

    private func decode<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        try CoreDecoding.decode(type, from: json)
    }
}
