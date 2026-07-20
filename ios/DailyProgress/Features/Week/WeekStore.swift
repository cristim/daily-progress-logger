import Foundation
import Observation

// MARK: - WeekStore

/// Observable store for the Week tab and weekly prompt sheets.
///
/// One instance per context: WeekView owns its own; RootTabView creates a separate
/// instance for scheduled prompt sheets so tab state is never clobbered mid-loop.
///
/// All weekly endpoints accept any date inside the target week. `referenceDate` is
/// the current "anchor" date; navigation shifts it by ±7 days. Weekly plan edits
/// always use a full-array replace (SetWeeklyPlan contract: never "" or "null").
@Observable
@MainActor
final class WeekStore {

    // MARK: - Navigation state

    /// Any date inside the currently-viewed week.
    /// Week navigation moves this by ±7 days.
    var referenceDate: Date = Date()

    // MARK: - Loaded data

    /// Weekly plan (goals) for referenceDate's week.
    var plan: WeeklyPlan?
    /// Weekly summary (done-by-day + goals + flags) for referenceDate's week.
    var summary: WeeklySummary?

    // MARK: - Review state

    /// Candidates for the week being reviewed (loaded separately via loadReview).
    var reviewCandidates: WeekReviewCandidates?
    /// One action per candidate, index-aligned. All .keep by default.
    var reviewActions: [ReviewAction] = []

    // MARK: - Unreviewed-week badge

    /// True when UnreviewedWeekJSON reports at least one unreviewed past week.
    var reviewPending: Bool = false
    /// Week string (e.g. "2026-W29") when reviewPending is true; nil otherwise.
    var pendingReviewWeekStr: String?

    // MARK: - UI state

    var isLoading: Bool = false
    /// Non-nil shows an alert (when content is present) or full-screen error (when not).
    var errorMessage: String?
    /// Short-lived bottom toast for recoverable notices.
    var toast: String?

    // MARK: - Init

    private let core: any CoreAPI
    /// Called after every successful mutation so sibling tabs can refresh.
    /// Excluded from observation — changing this property must not trigger re-renders.
    @ObservationIgnored var onMutation: (@MainActor () -> Void)?

    init(core: any CoreAPI, onMutation: (@MainActor () -> Void)? = nil) {
        self.core = core
        self.onMutation = onMutation
    }

    // MARK: - Week navigation

    func prevWeek() {
        referenceDate = Calendar.current.date(byAdding: .day, value: -7, to: referenceDate) ?? referenceDate
    }

    func nextWeek() {
        referenceDate = Calendar.current.date(byAdding: .day, value: 7, to: referenceDate) ?? referenceDate
    }

    func thisWeek() {
        referenceDate = Date()
    }

    // MARK: - Refresh (plan + summary + unreviewed badge, parallel)

    func refresh() async {
        // Guard against concurrent refreshes; a double-tap could trigger two.
        guard !isLoading else { return }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        let dateStr = referenceDate.coreDate
        let todayStr = Date().coreDate

        // Load plan, summary, and review-badge in parallel.
        async let planJSON = core.weeklyPlanJSON(date: dateStr)
        async let summaryJSON = core.weeklySummaryJSON(date: dateStr)
        async let reviewJSON = core.unreviewedWeekJSON(date: todayStr)

        do {
            let (pJSON, sJSON, rJSON) = try await (planJSON, summaryJSON, reviewJSON)
            plan = try CoreDecoding.decode(WeeklyPlan.self, from: pJSON)
            summary = try CoreDecoding.decode(WeeklySummary.self, from: sJSON)
            let pending = try CoreDecoding.decode(PendingWeek.self, from: rJSON)
            reviewPending = pending.pending
            pendingReviewWeekStr = pending.week
        } catch {
            handleError(error)
        }
    }

    // MARK: - Plan mutations

    /// Toggles the done state of the goal at index, then saves the full goals array.
    func setGoalDone(index: Int, done: Bool) async {
        guard var goals = plan?.goals, goals.indices.contains(index) else { return }
        goals[index].done = done
        await savePlan(goals: goals)
    }

    /// Parses newText as one-goal-per-line and appends them to the current goals array.
    func addGoals(text: String) async {
        let lines = text.split(separator: "\n", omittingEmptySubsequences: true)
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
        guard !lines.isEmpty else { return }
        var goals = plan?.goals ?? []
        for line in lines {
            goals.append(WeeklyGoal(text: line, done: false))
        }
        await savePlan(goals: goals)
    }

    /// Full-array replace: encodes goals to JSON and calls SetWeeklyPlan.
    /// Always sends at least [] — never "" or "null" (core rejects those; rule 8).
    /// Last-write-wins on concurrent saves: acceptable by design (no CAS on plan).
    func savePlan(goals: [WeeklyGoal]) async {
        do {
            let goalsJSON = try CoreDecoding.encode(goals)
            try await core.setWeeklyPlan(date: referenceDate.coreDate, goalsJSON: goalsJSON)
            onMutation?()  // bump dataVersion so sibling tabs refresh (I2)
            await refresh()
        } catch {
            handleError(error)
        }
    }

    // MARK: - Review

    /// Loads the review candidates for any date inside the target week.
    /// Initialises reviewActions to all .keep (default per the plan).
    func loadReview(date: String) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let json = try await core.weekReviewCandidatesJSON(date: date)
            reviewCandidates = try CoreDecoding.decode(WeekReviewCandidates.self, from: json)
            reviewActions = Array(repeating: .keep, count: reviewCandidates?.candidates.count ?? 0)
        } catch {
            handleError(error)
        }
    }

    /// Sends review decisions for the given date. Returns true on success so the
    /// caller can advance the loop or dismiss; false keeps the sheet open.
    func applyReview(date: String, rollover: Bool) async -> Bool {
        guard let candidates = reviewCandidates else { return false }
        do {
            let decisions = zip(candidates.candidates, reviewActions).map { text, action in
                ReviewItemDecision(text: text, action: action)
            }
            let payload = WeekReviewDecisions(decisions: decisions, rollover: rollover)
            let json = try CoreDecoding.encode(payload)
            try await core.applyWeekReview(date: date, decisionsJSON: json)
            onMutation?()  // bump dataVersion per mutation even if loop continues (I2)
            return true
        } catch {
            handleError(error)
            return false
        }
    }

    // MARK: - Summary

    /// Marks the week containing date as summarized. Returns true on success.
    func markSummarized(date: String) async -> Bool {
        do {
            try await core.markWeekSummarized(date: date)
            onMutation?()  // bump dataVersion per mutation even if loop continues (I2)
            return true
        } catch {
            handleError(error)
            return false
        }
    }

    // MARK: - Loop drivers

    /// Returns the Monday of the oldest unreviewed week, or nil when all caught up.
    /// Always queries today so the loop advances past weeks already reviewed this session.
    func nextUnreviewedWeek() async -> Date? {
        do {
            let json = try await core.unreviewedWeekJSON(date: Date().coreDate)
            let pending = try CoreDecoding.decode(PendingWeek.self, from: json)
            if pending.pending, let week = pending.week {
                return DateFormatting.date(fromISOWeek: week)
            }
        } catch {
            // Non-fatal: return nil so the loop stops cleanly.
        }
        return nil
    }

    /// Returns the Monday of the oldest week with a pending (unsummarized) summary, or nil.
    func nextPendingSummaryWeek() async -> Date? {
        do {
            let json = try await core.weeklySummaryPendingJSON(date: Date().coreDate)
            let pending = try CoreDecoding.decode(PendingWeek.self, from: json)
            if pending.pending, let week = pending.week {
                return DateFormatting.date(fromISOWeek: week)
            }
        } catch { }
        return nil
    }

    // MARK: - Private helpers

    private func handleError(_ error: Error) {
        if let coreError = error as? CoreError {
            errorMessage = coreError.errorDescription
        } else {
            errorMessage = error.localizedDescription
        }
    }

    func showToast(_ message: String) {
        toast = message
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 3_000_000_000)
            if toast == message { toast = nil }
        }
    }
}
