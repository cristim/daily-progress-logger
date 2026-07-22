import Foundation
import Core

// MARK: - Protocol

/// CoreAPI is the capability surface that feature stores depend on.
/// Declaring it as a protocol lets unit tests inject a mock without linking the
/// 40 MB xcframework into every test build cycle.
protocol CoreAPI: Sendable {
    func treeJSON(date: String) async throws -> String
    func addTask(date: String, text: String, projectID: String) async throws
    func setTaskState(date: String, index: Int, expectedText: String, state: String) async throws
    func deleteTask(date: String, index: Int, expectedText: String) async throws
    func editTaskText(date: String, index: Int, expectedText: String, newText: String) async throws
    func postponeToNextDay(date: String, index: Int, expectedText: String) async throws
    func postponeToNextWeek(date: String, index: Int, expectedText: String) async throws
    func moveTaskToBacklog(date: String, index: Int, expectedText: String) async throws
    func moveTaskToProject(date: String, index: Int, expectedText: String, projectID: String) async throws
    func addSubtask(date: String, parentIndex: Int, expectedParentText: String, text: String) async throws
    func backlogJSON() async throws -> String
    func adoptFromBacklog(date: String, text: String) async throws
    func moveBacklogItem(text: String, toNextWeek: Bool) async throws
    func projectsJSON() async throws -> String
    func addProject(name: String) async throws -> String
    func renameProject(id: String, newName: String) async throws
    func closeProject(id: String) async throws
    func reopenProject(id: String) async throws
    func recurringJSON() async throws -> String
    func addRecurring(text: String) async throws
    func removeRecurring(rawText: String) async throws
    func recycleJSON() async throws -> String
    func restoreTask(date: String, displayText: String) async throws
    func purgeRecycled(date: String, displayText: String) async throws
    func configJSON() async throws -> String
    func setConfig(patchJSON: String) async throws
    func duePromptsJSON(nowRFC3339: String) async throws -> String
    func morningCandidatesJSON(date: String) async throws -> String
    func applyMorning(date: String, decisionsJSON: String) async throws
    func applyEvening(date: String, decisionsJSON: String) async throws
    func dailyPromptJSON() async throws -> String
    func setDailyPrompt(text: String) async throws
    func weeklyPlanJSON(date: String) async throws -> String
    func setWeeklyPlan(date: String, goalsJSON: String) async throws
    func unreviewedWeekJSON(date: String) async throws -> String
    func weekReviewCandidatesJSON(date: String) async throws -> String
    func applyWeekReview(date: String, decisionsJSON: String) async throws
    func weeklySummaryJSON(date: String) async throws -> String
    func weeklySummaryPendingJSON(date: String) async throws -> String
    func markWeekSummarized(date: String) async throws
    func syncNow(tokenJSON: String) async throws -> String
    func conflictsJSON(tokenJSON: String) async throws -> String
    func resolveConflict(tokenJSON: String, path: String, choice: String) async throws
}

// MARK: - Actor

/// CoreClient owns the single MobilecoreCore handle and serialises all calls.
/// Being an actor means every call off the main thread is automatically sequenced
/// and the Go core's "one call at a time" contract is upheld on the host side.
///
/// Interop note (Xcode 26 / gomobile):
/// - NSString*-returning ObjC methods with NSError** are NOT bridged to throws;
///   they use the manual NSErrorPointer pattern: call with `var err: NSError? = nil`
///   and check `err` after the call.
/// - BOOL-returning methods ARE bridged to `throws` (standard Cocoa pattern),
///   but some methods with prepositions in the name are renamed by Swift
///   (e.g. `postponeToNextDay` → `postpone(toNextDay:)`).
/// - The global C function `MobilecoreOpen` uses positional parameters (no labels)
///   and requires the 4th NSErrorPointer argument explicitly.
actor CoreClient: CoreAPI {
    private let core: MobilecoreCore

    /// Opens a Core at dataDir and returns a ready CoreClient, or throws.
    /// dataDir is created if it does not exist.
    static func open(dataDir: String, clientID: String, deviceID: String) throws -> CoreClient {
        let url = URL(fileURLWithPath: dataDir)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        // MobilecoreOpen is a C function: positional args, no labels, NSErrorPointer 4th arg.
        var err: NSError? = nil
        let handle = MobilecoreOpen(dataDir, clientID, deviceID, &err)
        if let err = err { throw CoreError.classify(err) }
        guard let handle = handle else {
            throw CoreError.other("Core.Open returned nil without error")
        }
        return CoreClient(core: handle)
    }

    private init(core: MobilecoreCore) {
        self.core = core
    }

    // MARK: - Tree

    func treeJSON(date: String) async throws -> String {
        try strCall { err in core.treeJSON(date, error: err) }
    }

    // MARK: - Task actions
    // These BOOL-returning methods are bridged to throws by Swift.

    func addTask(date: String, text: String, projectID: String) async throws {
        try throwsCall { try core.addTask(date, text: text, projectID: projectID) }
    }

    func setTaskState(date: String, index: Int, expectedText: String, state: String) async throws {
        try throwsCall {
            try core.setTaskState(date, index: index, expectedText: expectedText, state: state)
        }
    }

    func deleteTask(date: String, index: Int, expectedText: String) async throws {
        try throwsCall { try core.deleteTask(date, index: index, expectedText: expectedText) }
    }

    func editTaskText(date: String, index: Int, expectedText: String, newText: String) async throws {
        try throwsCall {
            try core.editTaskText(date, index: index, expectedText: expectedText, newText: newText)
        }
    }

    // Swift renames these due to the preposition pattern:
    // postponeToNextDay → postpone(toNextDay:)
    // postponeToNextWeek → postpone(toNextWeek:)
    func postponeToNextDay(date: String, index: Int, expectedText: String) async throws {
        try throwsCall {
            try core.postpone(toNextDay: date, index: index, expectedText: expectedText)
        }
    }

    func postponeToNextWeek(date: String, index: Int, expectedText: String) async throws {
        try throwsCall {
            try core.postpone(toNextWeek: date, index: index, expectedText: expectedText)
        }
    }

    // moveTaskToBacklog → moveTask(toBacklog:)
    func moveTaskToBacklog(date: String, index: Int, expectedText: String) async throws {
        try throwsCall {
            try core.moveTask(toBacklog: date, index: index, expectedText: expectedText)
        }
    }

    // moveTaskToProject → moveTask(toProject:)
    func moveTaskToProject(date: String, index: Int, expectedText: String, projectID: String) async throws {
        try throwsCall {
            try core.moveTask(
                toProject: date,
                index: index,
                expectedText: expectedText,
                projectID: projectID
            )
        }
    }

    func addSubtask(date: String, parentIndex: Int, expectedParentText: String, text: String) async throws {
        try throwsCall {
            try core.addSubtask(
                date,
                parentIndex: parentIndex,
                expectedParentText: expectedParentText,
                text: text
            )
        }
    }

    // MARK: - Backlog

    func backlogJSON() async throws -> String {
        try strCall { err in core.backlogJSON(err) }
    }

    // adoptFromBacklog → adopt(fromBacklog:)
    func adoptFromBacklog(date: String, text: String) async throws {
        try throwsCall { try core.adopt(fromBacklog: date, text: text) }
    }

    func moveBacklogItem(text: String, toNextWeek: Bool) async throws {
        try throwsCall { try core.moveBacklogItem(text, toNextWeek: toNextWeek) }
    }

    // MARK: - Projects

    func projectsJSON() async throws -> String {
        try strCall { err in core.projectsJSON(err) }
    }

    func addProject(name: String) async throws -> String {
        try strCall { err in core.addProject(name, error: err) }
    }

    func renameProject(id: String, newName: String) async throws {
        try throwsCall { try core.renameProject(id, newName: newName) }
    }

    func closeProject(id: String) async throws {
        try throwsCall { try core.closeProject(id) }
    }

    func reopenProject(id: String) async throws {
        try throwsCall { try core.reopenProject(id) }
    }

    // MARK: - Recurring

    func recurringJSON() async throws -> String {
        try strCall { err in core.recurringJSON(err) }
    }

    func addRecurring(text: String) async throws {
        try throwsCall { try core.addRecurring(text) }
    }

    func removeRecurring(rawText: String) async throws {
        try throwsCall { try core.removeRecurring(rawText) }
    }

    // MARK: - Recycle

    func recycleJSON() async throws -> String {
        try strCall { err in core.recycleJSON(err) }
    }

    func restoreTask(date: String, displayText: String) async throws {
        try throwsCall { try core.restoreTask(date, displayText: displayText) }
    }

    func purgeRecycled(date: String, displayText: String) async throws {
        try throwsCall { try core.purgeRecycled(date, displayText: displayText) }
    }

    // MARK: - Config
    // configJSON uses NSErrorPointer (String-returning).
    // setConfig uses NSErrorPointer too (BOOL but not bridged to throws in this SDK version).

    func configJSON() async throws -> String {
        try strCall { err in core.configJSON(err) }
    }

    func setConfig(patchJSON: String) async throws {
        try throwsCall { try core.setConfig(patchJSON) }
    }

    // MARK: - Prompts

    func duePromptsJSON(nowRFC3339: String) async throws -> String {
        try strCall { err in core.duePromptsJSON(nowRFC3339, error: err) }
    }

    // MARK: - Check-ins

    func morningCandidatesJSON(date: String) async throws -> String {
        try strCall { err in core.morningCandidatesJSON(date, error: err) }
    }

    func applyMorning(date: String, decisionsJSON: String) async throws {
        try throwsCall { try core.applyMorning(date, decisionsJSON: decisionsJSON) }
    }

    func applyEvening(date: String, decisionsJSON: String) async throws {
        try throwsCall { try core.applyEvening(date, decisionsJSON: decisionsJSON) }
    }

    // MARK: - Daily prompt

    func dailyPromptJSON() async throws -> String {
        try strCall { err in core.dailyPromptJSON(err) }
    }

    func setDailyPrompt(text: String) async throws {
        try throwsCall { try core.setDailyPrompt(text) }
    }

    // MARK: - Weekly

    func weeklyPlanJSON(date: String) async throws -> String {
        try strCall { err in core.weeklyPlanJSON(date, error: err) }
    }

    func setWeeklyPlan(date: String, goalsJSON: String) async throws {
        try throwsCall { try core.setWeeklyPlan(date, goalsJSON: goalsJSON) }
    }

    func unreviewedWeekJSON(date: String) async throws -> String {
        try strCall { err in core.unreviewedWeekJSON(date, error: err) }
    }

    func weekReviewCandidatesJSON(date: String) async throws -> String {
        try strCall { err in core.weekReviewCandidatesJSON(date, error: err) }
    }

    func applyWeekReview(date: String, decisionsJSON: String) async throws {
        try throwsCall { try core.applyWeekReview(date, decisionsJSON: decisionsJSON) }
    }

    func weeklySummaryJSON(date: String) async throws -> String {
        try strCall { err in core.weeklySummaryJSON(date, error: err) }
    }

    func weeklySummaryPendingJSON(date: String) async throws -> String {
        try strCall { err in core.weeklySummaryPendingJSON(date, error: err) }
    }

    func markWeekSummarized(date: String) async throws {
        try throwsCall { try core.markWeekSummarized(date) }
    }

    // MARK: - Sync

    func syncNow(tokenJSON: String) async throws -> String {
        try strCall { err in core.syncNow(tokenJSON, error: err) }
    }

    func conflictsJSON(tokenJSON: String) async throws -> String {
        try strCall { err in core.conflictsJSON(tokenJSON, error: err) }
    }

    func resolveConflict(tokenJSON: String, path: String, choice: String) async throws {
        try throwsCall {
            try core.resolveConflict(tokenJSON, path: path, choice: choice)
        }
    }

    // MARK: - Private interop helpers

    /// Wraps an NSErrorPointer-style String call and maps NSError → CoreError.
    /// Use for all NSString-returning methods which are NOT bridged to throws.
    private func strCall(_ body: (NSErrorPointer) -> String) throws -> String {
        var err: NSError? = nil
        let result = body(&err)
        if let err = err { throw CoreError.classify(err) }
        return result
    }

    /// Wraps a bridged-throws call and maps any error → CoreError.
    /// Use for BOOL-returning methods which ARE bridged to `throws` by Swift.
    private func throwsCall(_ body: () throws -> Void) throws {
        do {
            try body()
        } catch let error as CoreError {
            throw error
        } catch {
            throw CoreError.classify(error)
        }
    }
}
