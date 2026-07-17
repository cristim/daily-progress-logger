import Foundation
import Observation

// MARK: - Supporting types

/// How the check-in sheet was triggered (affects the footer buttons label).
enum CheckinPresentation {
    case scheduled  // via due-prompts coordinator — shows "Skip Today"
    case manual     // via toolbar menu — shows "Close" (no bookkeeping)
}

// MARK: - CheckinStore

/// Shared store for both morning and evening check-in sheets.
/// One instance is created per check-in session (sheet presentation) from the
/// injected CoreAPI; it is not persisted across sessions.
@Observable
@MainActor
final class CheckinStore {

    // MARK: - Morning state

    /// Carry-over candidates from MorningCandidatesJSON; nil while loading.
    var morningCandidates: [MorningCandidate]?
    /// Per-candidate adopted flag; index-aligned with morningCandidates.
    /// Default: backlog candidates = false, same-week carry-overs = true.
    var adoptedFlags: [Bool] = []
    /// Current week's goals for the read-only goals section.
    var weeklyGoals: [WeeklyGoal] = []
    /// Count of tasks already in today's plan (all tasks in tree, projects + unfiled).
    var alreadyPlannedCount: Int = 0

    // MARK: - Evening state

    /// One row per plan item, flattened pre-order from TreeJSON.
    var eveningItems: [EveningItem] = []

    // MARK: - Shared UI state

    var isLoading = false
    /// Non-nil to show an alert inside the sheet (sheet stays open on error).
    var errorMessage: String?
    /// Non-nil for a brief toast overlay.
    var toast: String?

    // MARK: - EveningItem

    /// One flattened row in the evening check-in list.
    struct EveningItem {
        var text: String
        var depth: Int
        var action: EveningAction
    }

    // MARK: - Init

    private let core: any CoreAPI

    init(core: any CoreAPI) {
        self.core = core
    }

    // MARK: - Morning

    /// Loads candidates, weekly goals, and today's tree (for alreadyPlannedCount) in parallel.
    func loadMorning(date: Date) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        let dateStr = date.coreDate
        async let candidatesJSON = core.morningCandidatesJSON(date: dateStr)
        async let planJSON = core.weeklyPlanJSON(date: dateStr)
        async let treeJSON = core.treeJSON(date: dateStr)

        do {
            let (cJSON, pJSON, tJSON) = try await (candidatesJSON, planJSON, treeJSON)

            let candidates = try CoreDecoding.decode([MorningCandidate].self, from: cJSON)
            morningCandidates = candidates
            // Default-check rule: same-week carry-overs checked; backlog items unchecked.
            adoptedFlags = candidates.map { !$0.fromBacklog }

            let plan = try CoreDecoding.decode(WeeklyPlan.self, from: pJSON)
            weeklyGoals = plan.goals

            let tree = try CoreDecoding.decode(ProjectTree.self, from: tJSON)
            alreadyPlannedCount = countAllTasks(in: tree)
        } catch {
            handleError(error)
        }
    }

    /// Encodes and sends the morning decisions. Returns true on success so the
    /// sheet dismisses; false keeps it open (error surfaced via errorMessage).
    func applyMorning(date: Date, newItemsText: String) async -> Bool {
        isLoading = true
        defer { isLoading = false }
        do {
            let newItems = parseLines(newItemsText)
            let adopted = zip(morningCandidates ?? [], adoptedFlags)
                .filter(\.1)
                .map(\.0)
            let decisions = MorningDecisions(newItems: newItems, adopted: adopted)
            let json = try CoreDecoding.encode(decisions)
            try await core.applyMorning(date: date.coreDate, decisionsJSON: json)
            return true
        } catch {
            handleError(error)
            return false
        }
    }

    // MARK: - Evening

    /// Loads today's tree and builds the flat pre-order evening item list.
    func loadEvening(date: Date) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }
        do {
            let json = try await core.treeJSON(date: date.coreDate)
            let tree = try CoreDecoding.decode(ProjectTree.self, from: json)
            eveningItems = flattenForEvening(tree)
        } catch {
            handleError(error)
        }
    }

    /// Encodes and sends the evening decisions. Returns true on success.
    func applyEvening(date: Date, extraText: String) async -> Bool {
        isLoading = true
        defer { isLoading = false }
        do {
            let extra = parseLines(extraText)
            let decisions = EveningDecisions(
                decisions: eveningItems.map { EveningItemDecision(text: $0.text, action: $0.action) },
                extraDone: extra
            )
            let json = try CoreDecoding.encode(decisions)
            try await core.applyEvening(date: date.coreDate, decisionsJSON: json)
            return true
        } catch {
            handleError(error)
            return false
        }
    }

    // MARK: - Private helpers

    private func handleError(_ error: Error) {
        if let coreError = error as? CoreError {
            errorMessage = coreError.errorDescription
        } else {
            errorMessage = error.localizedDescription
        }
    }

    /// Trims each line, drops empty ones. Used for new-items and extra-done text.
    private func parseLines(_ text: String) -> [String] {
        text.split(separator: "\n", omittingEmptySubsequences: true)
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
    }

    /// Counts all tasks (recursively including children) across all projects and unfiled.
    private func countAllTasks(in tree: ProjectTree) -> Int {
        let projectCount = tree.projects.flatMap(\.tasks).map(countTask).reduce(0, +)
        let unfiledCount = tree.unfiled.map(countTask).reduce(0, +)
        return projectCount + unfiledCount
    }

    private func countTask(_ task: TreeTask) -> Int {
        1 + task.children.map(countTask).reduce(0, +)
    }

    /// Builds a flat pre-order list from the tree for the evening sheet.
    /// Order: each project's tasks (DFS), then unfiled tasks (DFS).
    /// Evening action seeded from task state per Qt's EveningActionForState:
    ///   todo → .todo, done → .done, postponed → .nextWeek
    private func flattenForEvening(_ tree: ProjectTree) -> [EveningItem] {
        var items: [EveningItem] = []
        for project in tree.projects {
            collectPreOrder(project.tasks, into: &items)
        }
        collectPreOrder(tree.unfiled, into: &items)
        return items
    }

    private func collectPreOrder(_ tasks: [TreeTask], into items: inout [EveningItem]) {
        for task in tasks {
            items.append(EveningItem(
                text: task.text,
                depth: task.depth,
                action: eveningAction(for: task.state)
            ))
            collectPreOrder(task.children, into: &items)
        }
    }

    /// Qt's EveningActionForState mapping.
    private func eveningAction(for state: ItemState) -> EveningAction {
        switch state {
        case .done: return .done
        case .todo: return .todo
        case .postponed: return .nextWeek  // Qt: postponed → NextWeek initial selection
        }
    }
}
