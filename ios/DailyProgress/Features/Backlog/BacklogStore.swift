import Foundation
import Observation

// MARK: - BacklogStore

/// Observable store for the Backlog tab.
///
/// One instance per context: BacklogView owns its own store.
///
/// All backlog endpoints work on bare strings; rows are identified by
/// (section, index) — never by the string itself because duplicate texts
/// are legal across sections.
@Observable
@MainActor
final class BacklogStore {

    // MARK: - Loaded data

    /// Backlog snapshot returned by BacklogJSON: current-week and next-week lists.
    var backlog: Backlog?

    // MARK: - UI state

    var isLoading: Bool = false
    /// Non-nil shows an alert (when content is present) or full-screen error (when not).
    var errorMessage: String?
    /// Short-lived bottom toast for recoverable notices (adopt confirmation, not-found).
    var toast: String?

    // MARK: - Init

    private let core: any CoreAPI
    /// Called after every successful mutation so sibling tabs refresh.
    /// Excluded from observation — changing this property must not trigger re-renders.
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
            let json = try await core.backlogJSON()
            backlog = try CoreDecoding.decode(Backlog.self, from: json)
        } catch {
            handleError(error)
        }
    }

    // MARK: - Mutations

    /// Adds text to today's plan and removes it from the backlog.
    /// On success: refreshes the backlog, bumps dataVersion (so Today tab
    /// picks up the new task), and shows "Planned for today" toast.
    /// Adopt always targets the real current date regardless of the Today tab's
    /// viewed date (matches Qt's time.Now() semantic).
    func adopt(text: String) async {
        do {
            try await core.adoptFromBacklog(date: Date().coreDate, text: text)
            onMutation?()           // bump dataVersion so Today tab refreshes (I2)
            await refresh()
            showToast("Planned for today: \(text)")
        } catch {
            handleError(error)
        }
    }

    /// Moves text between backlog sections (current <-> next week).
    /// CoreError.notFound → toast + refresh (item already gone; Qt's friendly path).
    /// Other errors → alert / full-screen error (rule 4).
    func move(text: String, toNextWeek: Bool) async {
        do {
            try await core.moveBacklogItem(text: text, toNextWeek: toNextWeek)
            await refresh()
        } catch CoreError.notFound {
            // Double-tap race or sync removed it — friendly notice, not an error.
            await refresh()
            showToast("This item is no longer in the backlog.")
        } catch {
            handleError(error)
        }
    }

    // MARK: - Private helpers

    private func handleError(_ error: Error) {
        // CancellationError fires when .task(id:) cancels a stale in-flight refresh;
        // it is not a failure and must not produce a user-visible alert (I4).
        if error is CancellationError { return }
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
}
