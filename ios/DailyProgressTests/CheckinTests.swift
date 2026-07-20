import XCTest
@testable import DailyProgress

// MARK: - Check-in DTO decode/encode tests

final class CheckinTests: XCTestCase {

    // MARK: - MorningCandidate array decode

    func testDecodeBaremorningCandidateArray() throws {
        // MorningCandidatesJSON returns a bare array (no wrapping object)
        let json = """
        [
            {"text": "Write tests", "from_backlog": false},
            {"text": "Review PR",   "from_backlog": true}
        ]
        """
        let candidates = try CoreDecoding.decode([MorningCandidate].self, from: json)
        XCTAssertEqual(candidates.count, 2)
        XCTAssertEqual(candidates[0].text, "Write tests")
        XCTAssertFalse(candidates[0].fromBacklog)
        XCTAssertEqual(candidates[1].text, "Review PR")
        XCTAssertTrue(candidates[1].fromBacklog)
    }

    func testDecodeMorningCandidateArrayEmpty() throws {
        let candidates = try CoreDecoding.decode([MorningCandidate].self, from: "[]")
        XCTAssertTrue(candidates.isEmpty)
    }

    // MARK: - MorningDecisions encode

    func testEncodeMorningDecisionsSnakeCaseKeys() throws {
        let decisions = MorningDecisions(
            newItems: ["Write tests", "Review PR"],
            adopted: [MorningCandidate(text: "Standup", fromBacklog: false)]
        )
        let json = try CoreDecoding.encode(decisions)
        let parsed = try XCTUnwrap(json.data(using: .utf8).flatMap {
            try? JSONSerialization.jsonObject(with: $0) as? [String: Any]
        })
        // Must use snake_case keys matching Go's json tags
        XCTAssertNotNil(parsed["new_items"], "Expected 'new_items' key")
        XCTAssertNotNil(parsed["adopted"],   "Expected 'adopted' key")
        let newItems = try XCTUnwrap(parsed["new_items"] as? [String])
        XCTAssertEqual(newItems, ["Write tests", "Review PR"])
        let adopted = try XCTUnwrap(parsed["adopted"] as? [[String: Any]])
        XCTAssertEqual(adopted.count, 1)
        XCTAssertEqual(adopted[0]["text"] as? String, "Standup")
        XCTAssertEqual(adopted[0]["from_backlog"] as? Bool, false)
    }

    // MARK: - EveningDecisions encode

    func testEncodeEveningDecisionsSnakeCaseKeys() throws {
        let decisions = EveningDecisions(
            decisions: [
                EveningItemDecision(text: "Write tests", action: .done),
                EveningItemDecision(text: "Fix bug",     action: .nextWeek),
            ],
            extraDone: ["bonus thing"]
        )
        let json = try CoreDecoding.encode(decisions)
        let parsed = try XCTUnwrap(json.data(using: .utf8).flatMap {
            try? JSONSerialization.jsonObject(with: $0) as? [String: Any]
        })
        XCTAssertNotNil(parsed["decisions"],  "Expected 'decisions' key")
        XCTAssertNotNil(parsed["extra_done"], "Expected 'extra_done' key")

        let items = try XCTUnwrap(parsed["decisions"] as? [[String: Any]])
        XCTAssertEqual(items.count, 2)
        XCTAssertEqual(items[0]["text"] as? String, "Write tests")
        XCTAssertEqual(items[0]["action"] as? Int, 1)   // .done = 1
        XCTAssertEqual(items[1]["action"] as? Int, 3)   // .nextWeek = 3

        let extra = try XCTUnwrap(parsed["extra_done"] as? [String])
        XCTAssertEqual(extra, ["bonus thing"])
    }

    // MARK: - EveningAction fail-loud decode

    func testEveningActionUnknownValueFails() throws {
        let json = #"{"text":"x","action":9}"#
        XCTAssertThrowsError(
            try CoreDecoding.decode(EveningItemDecision.self, from: json)
        ) { error in
            // Should be a contractViolation wrapping a DecodingError
            XCTAssertTrue(
                error is CoreError,
                "Expected CoreError.contractViolation, got \(error)"
            )
            if case CoreError.contractViolation(let msg) = error {
                XCTAssertTrue(msg.contains("9") || msg.contains("EveningAction"),
                              "Error message should mention the unknown value: \(msg)")
            }
        }
    }

    func testEveningActionAllKnownValuesDecodeOk() throws {
        // All 5 wire actions must decode successfully
        for (raw, expected): (Int, EveningAction) in [
            (0, .todo), (1, .done), (2, .nextDay), (3, .nextWeek), (4, .backlog)
        ] {
            let json = #"{"text":"x","action":\#(raw)}"#
            let decision = try CoreDecoding.decode(EveningItemDecision.self, from: json)
            XCTAssertEqual(decision.action, expected,
                           "Action \(raw) should decode to \(expected)")
        }
    }

    // MARK: - PromptID fail-loud decode

    func testPromptIDUnknownValueFails() throws {
        let json = #"{"id":9,"name":"unknown"}"#
        XCTAssertThrowsError(
            try CoreDecoding.decode(DuePrompt.self, from: json)
        ) { error in
            XCTAssertTrue(error is CoreError, "Expected CoreError, got \(error)")
        }
    }

    func testPromptIDAllKnownValuesDecodeOk() throws {
        for (raw, expected): (Int, PromptID) in [
            (0, .weekReview), (1, .weeklyPlan), (2, .morning), (3, .evening), (4, .weeklySummary)
        ] {
            let json = #"{"id":\#(raw),"name":"test"}"#
            let prompt = try CoreDecoding.decode(DuePrompt.self, from: json)
            XCTAssertEqual(prompt.id, expected)
        }
    }

    // MARK: - DuePrompts wrapper decode

    func testDecodeDuePromptsWithMultipleEntries() throws {
        let json = """
        {"due":[{"id":2,"name":"morning check-in"},{"id":3,"name":"evening check-in"}]}
        """
        let result = try CoreDecoding.decode(DuePrompts.self, from: json)
        XCTAssertEqual(result.due.count, 2)
        XCTAssertEqual(result.due[0].id, .morning)
        XCTAssertEqual(result.due[1].id, .evening)
    }

    func testDecodeDuePromptsEmpty() throws {
        let json = #"{"due":[]}"#
        let result = try CoreDecoding.decode(DuePrompts.self, from: json)
        XCTAssertTrue(result.due.isEmpty)
    }
}

// MARK: - CheckinCoordinator snooze / skip tests

@MainActor
final class CheckinCoordinatorTests: XCTestCase {

    // Unique UserDefaults suite to isolate tests from real app storage
    private var defaults: UserDefaults!
    private let suiteName = "CheckinCoordinatorTests"

    override func setUp() async throws {
        defaults = UserDefaults(suiteName: suiteName)
        defaults.removePersistentDomain(forName: suiteName)
    }

    override func tearDown() async throws {
        defaults.removePersistentDomain(forName: suiteName)
    }

    // We test coordinator logic through its public API.
    // Note: coordinator uses UserDefaults.standard; these tests exercise the
    // pure logic (snooze deadline, skip-today predicate) via direct method calls.

    // MARK: - Snooze deadline calculation

    func testSnoozeDeadlineIsNowPlusOneHour() {
        // Verify the +1h case directly via the exposed internal method.
        let coordinator = CheckinCoordinator(defaults: defaults)
        let now = Date()
        let deadline = coordinator.snoozeDeadline(from: now)
        XCTAssertEqual(
            deadline.timeIntervalSince1970,
            now.addingTimeInterval(3600).timeIntervalSince1970,
            accuracy: 1.0,
            "Normal snooze deadline should be now + 1 hour"
        )
    }

    func testSnoozeDeadlineCapsAtEndOfDay() {
        // 23:30 local -> +1h would cross midnight; must cap at 23:59:59.
        let coordinator = CheckinCoordinator(defaults: defaults)
        let calendar = Calendar.current
        var lateComponents = calendar.dateComponents([.year, .month, .day], from: Date())
        lateComponents.hour = 23
        lateComponents.minute = 30
        lateComponents.second = 0
        let lateTime = calendar.date(from: lateComponents)!

        let deadline = coordinator.snoozeDeadline(from: lateTime)

        var endComponents = calendar.dateComponents([.year, .month, .day], from: Date())
        endComponents.hour = 23
        endComponents.minute = 59
        endComponents.second = 59
        let endOfDay = calendar.date(from: endComponents)!

        XCTAssertEqual(
            deadline.timeIntervalSince1970,
            endOfDay.timeIntervalSince1970,
            accuracy: 1.0,
            "23:30 snooze deadline should cap at 23:59:59"
        )
    }

    func testSnoozeSuppressesPromptWithinWindow() {
        // Behavioural check: snoozed prompt is filtered by process().
        let coordinator = CheckinCoordinator(defaults: defaults)
        let prompt = DuePrompt(id: .morning, name: "morning check-in")
        coordinator.process(duePrompts: [prompt])
        XCTAssertNotNil(coordinator.scheduledPrompt)

        coordinator.snooze(prompt: prompt)
        coordinator.dismissCurrent()
        coordinator.process(duePrompts: [prompt])
        XCTAssertNil(coordinator.scheduledPrompt,
                     "Snoozed prompt should not be scheduled within the snooze window")
    }

    // MARK: - Skip today persists per-day

    func testSkipTodaySuppressesPromptForRestOfDay() {
        let coordinator = CheckinCoordinator(defaults: defaults)
        let prompt = DuePrompt(id: .morning, name: "morning check-in")
        coordinator.process(duePrompts: [prompt])
        XCTAssertNotNil(coordinator.scheduledPrompt)

        coordinator.skipToday(prompt: prompt)
        coordinator.dismissCurrent()

        // Processing again the same day should NOT show the prompt
        coordinator.process(duePrompts: [prompt])
        XCTAssertNil(coordinator.scheduledPrompt,
                     "Skipped prompt should not be scheduled on the same day")
    }

    // MARK: - Manual presentation does not set skippedOn

    func testManualCloseDoesNotCallSkipToday() {
        // Manual "Close" calls no bookkeeping — we verify by not calling skipToday
        // and then confirming the prompt can be scheduled again immediately.
        let coordinator = CheckinCoordinator(defaults: defaults)
        let prompt = DuePrompt(id: .morning, name: "morning check-in")
        coordinator.process(duePrompts: [prompt])
        XCTAssertNotNil(coordinator.scheduledPrompt)

        // Simulate manual close: no skipToday call, just dismiss
        coordinator.dismissCurrent()

        // Prompt should be schedulable again (no skip state set)
        coordinator.process(duePrompts: [prompt])
        XCTAssertNotNil(coordinator.scheduledPrompt,
                        "After manual Close (no skip), prompt should be schedulable again")
    }

    // MARK: - Phase B prompts are filtered

    func testPhaseBPromptsAreIgnored() {
        let coordinator = CheckinCoordinator(defaults: defaults)
        let phaseBPrompts: [DuePrompt] = [
            DuePrompt(id: .weekReview,     name: "week review"),
            DuePrompt(id: .weeklyPlan,     name: "weekly plan"),
            DuePrompt(id: .weeklySummary,  name: "weekly summary"),
        ]
        coordinator.process(duePrompts: phaseBPrompts)
        XCTAssertNil(coordinator.scheduledPrompt,
                     "Phase B prompt IDs (0,1,4) must be ignored in Phase A")
    }

    // MARK: - Queue advances correctly

    func testQueueAdvancesAfterDismiss() {
        let coordinator = CheckinCoordinator(defaults: defaults)
        let morning = DuePrompt(id: .morning, name: "morning check-in")
        let evening = DuePrompt(id: .evening, name: "evening check-in")
        coordinator.process(duePrompts: [morning, evening])

        XCTAssertEqual(coordinator.scheduledPrompt?.id, .morning)

        // Dismiss morning (no snooze/skip)
        coordinator.dismissCurrent()
        XCTAssertEqual(coordinator.scheduledPrompt?.id, .evening)

        // Dismiss evening
        coordinator.dismissCurrent()
        XCTAssertNil(coordinator.scheduledPrompt)
    }

    // MARK: - No interrupt while sheet is showing

    func testProcessDoesNotInterruptActiveSheet() {
        let coordinator = CheckinCoordinator(defaults: defaults)
        let morning = DuePrompt(id: .morning, name: "morning check-in")
        let evening = DuePrompt(id: .evening, name: "evening check-in")

        coordinator.process(duePrompts: [morning])
        XCTAssertEqual(coordinator.scheduledPrompt?.id, .morning)

        // Simulate a second duePrompts refresh while morning sheet is visible
        coordinator.process(duePrompts: [morning, evening])
        // Should still be showing morning, not switching to evening
        XCTAssertEqual(coordinator.scheduledPrompt?.id, .morning,
                       "process() must not interrupt an active sheet")
    }
}
