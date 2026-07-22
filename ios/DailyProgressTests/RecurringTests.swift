import XCTest
@testable import DailyProgress

// MARK: - Recurring DTO decode tests

final class RecurringTests: XCTestCase {

    // MARK: - Management shape (RecurringJSON): {text, project, raw} only

    func testDecodeManagementShapeLeavesScheduleFieldsNil() throws {
        let json = """
        {"text": "Standup", "project": "work", "raw": "Standup @payments @daily @9:00"}
        """
        let template = try CoreDecoding.decode(RecurringTemplate.self, from: json)
        XCTAssertEqual(template.text, "Standup")
        XCTAssertEqual(template.project, "work")
        XCTAssertEqual(template.raw, "Standup @payments @daily @9:00")
        XCTAssertNil(template.describe, "management shape has no describe field")
        XCTAssertNil(template.kind, "management shape has no kind field")
        XCTAssertNil(template.weekday)
        XCTAssertNil(template.monthDay)
        XCTAssertNil(template.hour)
        XCTAssertNil(template.minute)
    }

    func testDecodeManagementShapeArray() throws {
        let json = """
        [
            {"text": "Standup", "project": "", "raw": "Standup @daily"},
            {"text": "Rent", "project": "", "raw": "Rent @monthly @1"}
        ]
        """
        let templates = try CoreDecoding.decode([RecurringTemplate].self, from: json)
        XCTAssertEqual(templates.count, 2)
        XCTAssertEqual(templates[0].text, "Standup")
        XCTAssertNil(templates[0].describe)
    }

    // MARK: - Full tree shape (TreeJSON.recurring): all 9 fields present

    func testDecodeTreeShapeAllFieldsPresent() throws {
        let json = """
        {
            "text": "Report",
            "project": "work",
            "describe": "weekly Fri 16:00",
            "kind": 2,
            "weekday": 5,
            "month_day": 1,
            "hour": 16,
            "minute": 0,
            "raw": "Report @work @weekly @fri @16:00"
        }
        """
        let template = try CoreDecoding.decode(RecurringTemplate.self, from: json)
        XCTAssertEqual(template.describe, "weekly Fri 16:00")
        XCTAssertEqual(template.kind, 2)
        XCTAssertEqual(template.weekday, 5)
        XCTAssertEqual(template.monthDay, 1)
        XCTAssertEqual(template.hour, 16)
        XCTAssertEqual(template.minute, 0)
        XCTAssertEqual(template.raw, "Report @work @weekly @fri @16:00")
    }

    // MARK: - month_day wire key maps to monthDay (snake_case)

    func testDecodeMonthDaySnakeCaseKey() throws {
        let json = """
        {"text": "Rent", "project": "", "describe": "monthly 1 09:00", "kind": 3,
         "weekday": 0, "month_day": 1, "hour": 9, "minute": 0, "raw": "Rent @monthly @1"}
        """
        let template = try CoreDecoding.decode(RecurringTemplate.self, from: json)
        XCTAssertEqual(template.monthDay, 1, "'month_day' wire key must decode to monthDay property")
    }

    // MARK: - ProjectTree.recurring decodes via the same DTO

    func testDecodeProjectTreeRecurringField() throws {
        let json = """
        {
            "projects": [], "unfiled": [], "recycled": [],
            "recurring": [
                {"text": "Standup", "project": "", "describe": "daily 09:00", "kind": 0,
                 "weekday": 0, "month_day": 0, "hour": 9, "minute": 0, "raw": "Standup @daily @9:00"}
            ]
        }
        """
        let tree = try CoreDecoding.decode(ProjectTree.self, from: json)
        XCTAssertEqual(tree.recurring.count, 1)
        XCTAssertEqual(tree.recurring[0].describe, "daily 09:00")
    }
}

// MARK: - RecurringStore behavior tests

/// Store-level tests exercising BAD_INPUT inline surfacing and mutation bookkeeping.
/// RecurringStore is @MainActor so all test methods must be async.
@MainActor
final class RecurringStoreTests: XCTestCase {

    // MARK: - Refresh reads TreeJSON.recurring, not RecurringJSON

    func testRefreshLoadsTemplatesFromTreeJSON() async {
        let mock = RecurringMockCoreAPI()
        mock.treeJSONResult = """
        {"projects": [], "unfiled": [], "recycled": [],
         "recurring": [{"text": "Standup", "project": "", "describe": "daily 09:00",
         "kind": 0, "weekday": 0, "month_day": 0, "hour": 9, "minute": 0, "raw": "Standup @daily"}]}
        """
        let store = RecurringStore(core: mock)

        await store.refresh()

        XCTAssertEqual(store.templates?.count, 1)
        XCTAssertEqual(store.templates?.first?.text, "Standup")
        XCTAssertNil(store.errorMessage)
    }

    // MARK: - Add with BAD_INPUT keeps state, surfaces addFieldError (not errorMessage)

    func testAddBadInputSurfacesInlineFieldError() async {
        let mock = RecurringMockCoreAPI()
        mock.treeJSONResult = #"{"projects":[],"unfiled":[],"recycled":[],"recurring":[]}"#
        mock.addRecurringError = CoreError.badInput("no recurrence tag in \"Buy milk\"")
        let store = RecurringStore(core: mock)
        await store.refresh()

        let ok = await store.add(text: "Buy milk")

        XCTAssertFalse(ok, "BAD_INPUT must keep the sheet open (return false)")
        XCTAssertNotNil(store.addFieldError, "BAD_INPUT must surface under the field")
        XCTAssertNil(store.errorMessage, "BAD_INPUT must not pop the list-level alert")
        XCTAssertEqual(mock.onMutationCallCount, 0, "a failed add must not bump dataVersion")
    }

    // MARK: - Add success bumps onMutation and refreshes

    func testAddSuccessCallsOnMutationAndRefreshes() async {
        let mock = RecurringMockCoreAPI()
        mock.treeJSONResult = #"{"projects":[],"unfiled":[],"recycled":[],"recurring":[]}"#
        var mutationCount = 0
        let store = RecurringStore(core: mock, onMutation: { mutationCount += 1 })
        await store.refresh()

        // After add succeeds, refresh() re-reads the (now updated) tree.
        mock.treeJSONResult = """
        {"projects":[],"unfiled":[],"recycled":[],
         "recurring":[{"text":"Vitamins","project":"","describe":"daily 08:30","kind":0,
         "weekday":0,"month_day":0,"hour":8,"minute":30,"raw":"Vitamins @daily @8:30"}]}
        """
        let ok = await store.add(text: "Vitamins @daily @8:30")

        XCTAssertTrue(ok)
        XCTAssertEqual(mutationCount, 1, "add must call onMutation once on success")
        XCTAssertNil(store.addFieldError)
        XCTAssertEqual(store.templates?.count, 1)
    }

    // MARK: - Remove bumps onMutation and refreshes

    func testRemoveCallsOnMutationAndRefreshes() async {
        let mock = RecurringMockCoreAPI()
        mock.treeJSONResult = """
        {"projects":[],"unfiled":[],"recycled":[],
         "recurring":[{"text":"Standup","project":"","describe":"daily 09:00","kind":0,
         "weekday":0,"month_day":0,"hour":9,"minute":0,"raw":"Standup @daily"}]}
        """
        var mutationCount = 0
        let store = RecurringStore(core: mock, onMutation: { mutationCount += 1 })
        await store.refresh()
        XCTAssertEqual(store.templates?.count, 1)

        mock.treeJSONResult = #"{"projects":[],"unfiled":[],"recycled":[],"recurring":[]}"#
        await store.remove(raw: "Standup @daily")

        XCTAssertEqual(mutationCount, 1, "remove must call onMutation once on success")
        XCTAssertEqual(store.templates?.count, 0)
        XCTAssertNil(store.errorMessage)
    }
}

// MARK: - RecurringMockCoreAPI

/// Minimal mock of CoreAPI for RecurringStore unit tests.
/// Only recurring/tree-related methods are configured; all others throw CoreError.other
/// to catch unexpected calls.
final class RecurringMockCoreAPI: CoreAPI, @unchecked Sendable {

    // MARK: Configurable per test

    var treeJSONResult: String = #"{"projects":[],"unfiled":[],"recycled":[],"recurring":[]}"#
    var addRecurringError: Error? = nil
    var removeRecurringError: Error? = nil
    private(set) var onMutationCallCount = 0

    // MARK: CoreAPI — recurring + tree

    func treeJSON(date: String) async throws -> String { treeJSONResult }

    func addRecurring(text: String) async throws {
        if let err = addRecurringError {
            onMutationCallCount = 0
            throw err
        }
        onMutationCallCount += 1
    }

    func removeRecurring(rawText: String) async throws {
        if let err = removeRecurringError { throw err }
    }

    func recurringJSON() async throws -> String { throw CoreError.other("unexpected call: recurringJSON") }

    // MARK: CoreAPI — all other methods (not used by RecurringStore)

    func addTask(date: String, text: String, projectID: String) async throws { throw CoreError.other("unexpected") }
    func setTaskState(date: String, index: Int, expectedText: String, state: String) async throws { throw CoreError.other("unexpected") }
    func deleteTask(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func editTaskText(date: String, index: Int, expectedText: String, newText: String) async throws { throw CoreError.other("unexpected") }
    func postponeToNextDay(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func postponeToNextWeek(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func moveTaskToBacklog(date: String, index: Int, expectedText: String) async throws { throw CoreError.other("unexpected") }
    func moveTaskToProject(date: String, index: Int, expectedText: String, projectID: String) async throws { throw CoreError.other("unexpected") }
    func addSubtask(date: String, parentIndex: Int, expectedParentText: String, text: String) async throws { throw CoreError.other("unexpected") }
    func backlogJSON() async throws -> String { throw CoreError.other("unexpected") }
    func adoptFromBacklog(date: String, text: String) async throws { throw CoreError.other("unexpected") }
    func moveBacklogItem(text: String, toNextWeek: Bool) async throws { throw CoreError.other("unexpected") }
    func projectsJSON() async throws -> String { throw CoreError.other("unexpected") }
    func addProject(name: String) async throws -> String { throw CoreError.other("unexpected") }
    func renameProject(id: String, newName: String) async throws { throw CoreError.other("unexpected") }
    func closeProject(id: String) async throws { throw CoreError.other("unexpected") }
    func reopenProject(id: String) async throws { throw CoreError.other("unexpected") }
    func recycleJSON() async throws -> String { throw CoreError.other("unexpected") }
    func restoreTask(date: String, displayText: String) async throws { throw CoreError.other("unexpected") }
    func purgeRecycled(date: String, displayText: String) async throws { throw CoreError.other("unexpected") }
    func configJSON() async throws -> String { throw CoreError.other("unexpected") }
    func setConfig(patchJSON: String) async throws { throw CoreError.other("unexpected") }
    func duePromptsJSON(nowRFC3339: String) async throws -> String { throw CoreError.other("unexpected") }
    func morningCandidatesJSON(date: String) async throws -> String { throw CoreError.other("unexpected") }
    func applyMorning(date: String, decisionsJSON: String) async throws { throw CoreError.other("unexpected") }
    func applyEvening(date: String, decisionsJSON: String) async throws { throw CoreError.other("unexpected") }
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
