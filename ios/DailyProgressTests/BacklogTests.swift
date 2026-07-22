import XCTest
@testable import DailyProgress

// MARK: - Backlog DTO decode tests

final class BacklogTests: XCTestCase {

    // MARK: - Backlog decode (populated)

    func testDecodeBacklogWithItems() throws {
        let json = """
        {
            "current":   ["Buy milk", "Fix bug"],
            "next_week": ["Write report"]
        }
        """
        let backlog = try CoreDecoding.decode(Backlog.self, from: json)
        XCTAssertEqual(backlog.current,  ["Buy milk", "Fix bug"])
        XCTAssertEqual(backlog.nextWeek, ["Write report"])
    }

    // MARK: - Backlog decode (empty arrays)

    func testDecodeBacklogEmptyArrays() throws {
        let json = #"{"current":[],"next_week":[]}"#
        let backlog = try CoreDecoding.decode(Backlog.self, from: json)
        XCTAssertTrue(backlog.current.isEmpty)
        XCTAssertTrue(backlog.nextWeek.isEmpty)
    }

    // MARK: - CodingKeys: wire uses "next_week" (snake_case)

    func testDecodeBacklogSnakeCaseKey() throws {
        // Verifies that the "next_week" wire key maps to nextWeek in the Swift model.
        let json = #"{"current":[],"next_week":["Ship it"]}"#
        let backlog = try CoreDecoding.decode(Backlog.self, from: json)
        XCTAssertEqual(backlog.nextWeek, ["Ship it"],
                       "'next_week' wire key must decode to nextWeek property")
    }
}

// MARK: - BacklogStore behavior tests

/// Store-level tests exercising the NOT_FOUND friendly path and toast semantics.
/// BacklogStore is @MainActor so all test methods must be async.
@MainActor
final class BacklogStoreTests: XCTestCase {

    // MARK: - NOT_FOUND on move -> toast, not errorMessage

    func testMoveNotFoundShowsToastNotErrorMessage() async {
        let mock = BacklogMockCoreAPI()
        // Pre-populate so backlog is loaded
        mock.backlogJSONResult = #"{"current":["Task A"],"next_week":[]}"#
        mock.moveBacklogItemError = CoreError.notFound("item not found")

        let store = BacklogStore(core: mock)

        // Load initial state
        await store.refresh()
        XCTAssertNotNil(store.backlog, "backlog should be loaded after refresh")
        XCTAssertNil(store.errorMessage, "no error after successful refresh")

        // Trigger move; core returns NOT_FOUND (double-tap race)
        await store.move(text: "Task A", toNextWeek: true)

        // NOT_FOUND must surface as toast only, never as errorMessage (Qt friendly path)
        XCTAssertNil(store.errorMessage,
                     "NOT_FOUND must not set errorMessage — use toast instead")
        XCTAssertEqual(store.toast, "This item is no longer in the backlog.",
                       "NOT_FOUND must show the friendly not-found toast")
    }

    // MARK: - Adopt success bumps onMutation

    func testAdoptCallsOnMutation() async {
        let mock = BacklogMockCoreAPI()
        mock.backlogJSONResult = #"{"current":["Task B"],"next_week":[]}"#

        var mutationCount = 0
        let store = BacklogStore(core: mock, onMutation: { mutationCount += 1 })

        await store.refresh()
        XCTAssertEqual(mutationCount, 0)

        await store.adopt(text: "Task B")

        XCTAssertEqual(mutationCount, 1, "adopt must call onMutation once on success")
        XCTAssertNil(store.errorMessage, "no error on successful adopt")
    }

    // MARK: - Adopt error surfaces as errorMessage (not toast)

    func testAdoptOtherErrorSetsErrorMessage() async {
        let mock = BacklogMockCoreAPI()
        mock.backlogJSONResult = #"{"current":["Task C"],"next_week":[]}"#
        mock.adoptFromBacklogError = CoreError.other("internal failure")

        let store = BacklogStore(core: mock)

        await store.refresh()
        await store.adopt(text: "Task C")

        XCTAssertNotNil(store.errorMessage, "non-NOT_FOUND errors must set errorMessage")
        XCTAssertNil(store.toast, "non-NOT_FOUND errors must not show toast")
    }
}

// MARK: - BacklogMockCoreAPI

/// Minimal mock of CoreAPI for BacklogStore unit tests.
/// Only backlog-related methods are configured; all others throw CoreError.other
/// to catch unexpected calls.
final class BacklogMockCoreAPI: CoreAPI, @unchecked Sendable {

    // MARK: Configurable per test

    var backlogJSONResult: String = #"{"current":[],"next_week":[]}"#
    var backlogJSONError: Error? = nil
    var adoptFromBacklogError: Error? = nil
    var moveBacklogItemError: Error? = nil

    // MARK: CoreAPI — backlog

    func backlogJSON() async throws -> String {
        if let err = backlogJSONError { throw err }
        return backlogJSONResult
    }

    func adoptFromBacklog(date: String, text: String) async throws {
        if let err = adoptFromBacklogError { throw err }
    }

    func moveBacklogItem(text: String, toNextWeek: Bool) async throws {
        if let err = moveBacklogItemError { throw err }
    }

    // MARK: CoreAPI — all other methods (not used by BacklogStore)

    func treeJSON(date: String) async throws -> String { throw CoreError.other("unexpected call: treeJSON") }
    func addTask(date: String, text: String, projectID: String) async throws { throw CoreError.other("unexpected") }
    func setTaskState(date: String, index: Int, expectedText: String, state: String) async throws { throw CoreError.other("unexpected") }
    func deleteTask(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func editTaskText(date: String, index: Int, expectedText: String, newText: String) async throws { throw CoreError.other("unexpected") }
    func postponeToNextDay(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func postponeToNextWeek(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func moveTaskToBacklog(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func moveTaskToProject(date: String, index: Int, expectedText: String, projectID: String) async throws { throw CoreError.other("unexpected") }
    func addSubtask(date: String, parentIndex: Int, expectedParentText: String, text: String) async throws { throw CoreError.other("unexpected") }
    func projectsJSON() async throws -> String { throw CoreError.other("unexpected") }
    func addProject(name: String) async throws -> String { throw CoreError.other("unexpected") }
    func renameProject(id: String, newName: String) async throws { throw CoreError.other("unexpected") }
    func closeProject(id: String) async throws { throw CoreError.other("unexpected") }
    func reopenProject(id: String) async throws { throw CoreError.other("unexpected") }
    func recurringJSON() async throws -> String { throw CoreError.other("unexpected") }
    func addRecurring(text: String) async throws { throw CoreError.other("unexpected") }
    func removeRecurring(rawText: String) async throws { throw CoreError.other("unexpected") }
    func recycleJSON() async throws -> String { throw CoreError.other("unexpected") }
    func restoreTask(date: String, displayText: String) async throws { throw CoreError.other("unexpected") }
    func purgeRecycled(date: String, displayText: String) async throws { throw CoreError.other("unexpected") }
    func configJSON() async throws -> String { throw CoreError.other("unexpected") }
    func setConfig(patchJSON: String) async throws { throw CoreError.other("unexpected") }
    func duePromptsJSON(nowRFC3339: String) async throws -> String { throw CoreError.other("unexpected") }
    func morningCandidatesJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func applyMorning(date: String, decisionsJSON: String) async throws { throw CoreError.other("unexpected") }
    func applyEvening(date: String, decisionsJSON: String) async throws { throw CoreError.other("unexpected") }
    func dailyPromptJSON() async throws -> String { throw CoreError.other("unexpected call: dailyPromptJSON") }
    func setDailyPrompt(text: String) async throws { throw CoreError.other("unexpected") }
    func weeklyPlanJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func setWeeklyPlan(date: String, goalsJSON: String) async throws { throw CoreError.other("unexpected") }
    func unreviewedWeekJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func weekReviewCandidatesJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func applyWeekReview(date: String, decisionsJSON: String) async throws { throw CoreError.other("unexpected") }
    func weeklySummaryJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func weeklySummaryPendingJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func markWeekSummarized(date: String) async throws { throw CoreError.other("unexpected") }
    func syncNow(tokenJSON: String) async throws -> String { throw CoreError.other("unexpected") }
    func conflictsJSON(tokenJSON: String) async throws -> String { throw CoreError.other("unexpected") }
    func resolveConflict(tokenJSON: String, path: String, choice: String) async throws { throw CoreError.other("unexpected") }
}
