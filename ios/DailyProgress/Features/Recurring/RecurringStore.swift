import Foundation
import Observation

// MARK: - RecurringStore

/// Observable store for the Recurring templates screen.
///
/// Design decision (per plan): the list is sourced from TreeJSON(today).recurring
/// rather than RecurringJSON, because the tree shape carries `describe` (the
/// human-readable schedule) while the management endpoint's shape is only
/// {text, project, raw}. Templates are global (not tied to a viewed day), so
/// the store always reads today's tree regardless of which date any other tab
/// is viewing. `raw` (present in both shapes) round-trips to RemoveRecurring.
@Observable
@MainActor
final class RecurringStore {

    // MARK: - Loaded data

    var templates: [RecurringTemplate]?

    // MARK: - UI state

    var isLoading: Bool = false
    /// Non-nil shows an alert (when content is present) or full-screen error (when not).
    /// Used for refresh/remove failures; add() failures use addFieldError instead so
    /// they render inline in the add sheet rather than as an alert.
    var errorMessage: String?
    /// Short-lived bottom toast for recoverable notices.
    var toast: String?
    /// Inline error for the add-recurring sheet (e.g. BAD_INPUT: no recurrence tag).
    /// Kept separate from errorMessage so the sheet can show it under the text field
    /// while staying open, without also popping the list-level alert.
    var addFieldError: String?

    // MARK: - Init

    private let core: any CoreAPI
    /// Called after every successful mutation so sibling tabs refresh (Today's
    /// recurring summary materializes from the same data).
    @ObservationIgnored var onMutation: (@MainActor () -> Void)?

    init(core: any CoreAPI, onMutation: (@MainActor () -> Void)? = nil) {
        self.core = core
        self.onMutation = onMutation
    }

    // MARK: - Refresh

    func refresh() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let json = try await core.treeJSON(date: Date().coreDate)
            let tree = try CoreDecoding.decode(ProjectTree.self, from: json)
            templates = tree.recurring
        } catch {
            handleError(error)
        }
    }

    // MARK: - Mutations

    /// Adds a recurring template. Returns true on success so the sheet dismisses;
    /// false keeps it open with addFieldError surfaced under the field (BAD_INPUT
    /// is a validation failure, not a system error — never a silent drop).
    func add(text: String) async -> Bool {
        addFieldError = nil
        do {
            try await core.addRecurring(text: text)
            onMutation?()
            await refresh()
            return true
        } catch let coreError as CoreError {
            addFieldError = coreError.errorDescription
            return false
        } catch {
            addFieldError = error.localizedDescription
            return false
        }
    }

    /// Removes a template by its raw stored line (after user confirmation in the view).
    /// RemoveRecurring is a no-op when the template already vanished; refresh either way.
    func remove(raw: String) async {
        do {
            try await core.removeRecurring(rawText: raw)
            onMutation?()
            await refresh()
        } catch {
            handleError(error)
        }
    }

    // MARK: - Private helpers

    private func handleError(_ error: Error) {
        // CancellationError fires when .task(id:) cancels a stale in-flight refresh;
        // it is not a failure and must not produce a user-visible alert.
        if error is CancellationError { return }
        if let coreError = error as? CoreError {
            errorMessage = coreError.errorDescription
        } else {
            errorMessage = error.localizedDescription
        }
    }
}
