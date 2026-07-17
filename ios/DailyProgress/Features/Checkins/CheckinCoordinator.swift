import Foundation
import Observation

// MARK: - CheckinCoordinator

/// Routes due prompts to the appropriate check-in sheet.
/// Owns snooze/skip persistence in UserDefaults, keyed by prompt ID (Int).
/// Lifecycle: one shared instance in RootTabView; call process() on every
/// duePrompts refresh; sheets call snooze()/skipToday() then dismiss,
/// which triggers dismissCurrent() via the sheet's onDismiss callback.
@Observable
@MainActor
final class CheckinCoordinator {

    // MARK: - Published state

    /// The prompt whose sheet is currently being presented; nil when idle.
    /// RootTabView binds to this to drive .sheet(item:).
    var scheduledPrompt: DuePrompt?

    // MARK: - Private state

    /// Remaining due prompts after the current one.
    private var pendingQueue: [DuePrompt] = []

    // UserDefaults storage keys
    private static let snoozeKey = "checkin.snoozeUntil"
    private static let skipKey   = "checkin.skippedOn"

    /// [promptID: snoozeUntil date] — suppresses a prompt until this time.
    private var snoozeUntil: [Int: Date] = [:]

    /// [promptID: "YYYY-MM-DD"] — suppresses a prompt for the rest of that day.
    private var skippedOn: [Int: String] = [:]

    // MARK: - Init

    init() {
        loadPersisted()
    }

    // MARK: - Process due prompts

    /// Called whenever AppState.duePrompts is refreshed.
    /// Filters snoozed/skipped prompts, restricts to Phase-A IDs (.morning, .evening),
    /// and sets scheduledPrompt to the first presentable prompt.
    /// If a sheet is already showing, this is a no-op (don't interrupt).
    func process(duePrompts: [DuePrompt]) {
        guard scheduledPrompt == nil else { return }
        let now = Date()
        let presentable = duePrompts
            .filter { isPhaseA($0) && !isSnoozed($0, at: now) && !isSkipped($0, at: now) }
        guard !presentable.isEmpty else { return }
        pendingQueue = Array(presentable.dropFirst())
        scheduledPrompt = presentable.first
    }

    // MARK: - Sheet dismissal

    /// Called from the sheet's onDismiss callback (fires for any dismissal reason).
    /// Advances to the next prompt in the queue.
    func dismissCurrent() {
        if pendingQueue.isEmpty {
            scheduledPrompt = nil
        } else {
            scheduledPrompt = pendingQueue.removeFirst()
        }
    }

    // MARK: - Snooze (suppress until now+1h capped at 23:59:59 local)

    /// Records a snooze for the prompt. The sheet then calls dismiss(), which triggers
    /// dismissCurrent() via onDismiss. Snooze state persists across app restarts.
    func snooze(prompt: DuePrompt) {
        let now = Date()
        let wakeAt = snoozeDeadline(from: now)
        snoozeUntil[prompt.id.rawValue] = wakeAt
        persistSnoozeUntil()
    }

    // MARK: - Skip today (suppress for the rest of today)

    /// Records a skip for the prompt. The sheet then calls dismiss() -> dismissCurrent().
    /// Only called for scheduled presentations; manual "Close" does no bookkeeping.
    func skipToday(prompt: DuePrompt) {
        skippedOn[prompt.id.rawValue] = Date().coreDate
        persistSkippedOn()
    }

    // MARK: - Manual presentation helpers

    /// Returns the current now-local timestamp string for DuePromptsJSON.
    /// Exposed so manual triggers (Today toolbar) can refresh prompts before
    /// presenting; normal flow goes through AppState.refreshDuePrompts().
    func nowRFC3339() -> String {
        Date().rfc3339
    }

    // MARK: - Private predicates

    private func isPhaseA(_ prompt: DuePrompt) -> Bool {
        prompt.id == .morning || prompt.id == .evening
        // Prompt IDs 0 (weekReview), 1 (weeklyPlan), 4 (weeklySummary) are phase B.
    }

    private func isSnoozed(_ prompt: DuePrompt, at now: Date) -> Bool {
        guard let until = snoozeUntil[prompt.id.rawValue] else { return false }
        return until > now
    }

    private func isSkipped(_ prompt: DuePrompt, at now: Date) -> Bool {
        guard let day = skippedOn[prompt.id.rawValue] else { return false }
        return day == now.coreDate
    }

    // MARK: - Snooze deadline calculation

    /// now + 1 hour, capped at 23:59:59 local (end of day).
    private func snoozeDeadline(from now: Date) -> Date {
        let oneHourLater = now.addingTimeInterval(3600)
        // Compute 23:59:59 of today in local time
        let calendar = Calendar.current
        let tomorrow = calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: now))!
        let endOfDay = tomorrow.addingTimeInterval(-1)  // 23:59:59
        return min(oneHourLater, endOfDay)
    }

    // MARK: - UserDefaults persistence

    private func loadPersisted() {
        let defaults = UserDefaults.standard
        if let raw = defaults.dictionary(forKey: Self.snoozeKey) as? [String: Date] {
            snoozeUntil = Dictionary(uniqueKeysWithValues: raw.compactMap { k, v in
                Int(k).map { ($0, v) }
            })
        }
        if let raw = defaults.dictionary(forKey: Self.skipKey) as? [String: String] {
            skippedOn = Dictionary(uniqueKeysWithValues: raw.compactMap { k, v in
                Int(k).map { ($0, v) }
            })
        }
    }

    private func persistSnoozeUntil() {
        let raw = Dictionary(uniqueKeysWithValues: snoozeUntil.map { ("\($0.key)", $0.value) })
        UserDefaults.standard.set(raw, forKey: Self.snoozeKey)
    }

    private func persistSkippedOn() {
        let raw = Dictionary(uniqueKeysWithValues: skippedOn.map { ("\($0.key)", $0.value) })
        UserDefaults.standard.set(raw, forKey: Self.skipKey)
    }
}
