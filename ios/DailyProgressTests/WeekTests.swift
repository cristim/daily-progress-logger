import XCTest
@testable import DailyProgress

// MARK: - Weekly DTO decode tests

final class WeekTests: XCTestCase {

    // MARK: - WeeklyPlan decode

    func testDecodeWeeklyPlanPlanned() throws {
        let json = """
        {
            "week": "2026-W29",
            "planned": true,
            "goals": [
                {"text": "Ship mobile core", "done": false},
                {"text": "Write tests",       "done": true}
            ]
        }
        """
        let plan = try CoreDecoding.decode(WeeklyPlan.self, from: json)
        XCTAssertEqual(plan.week, "2026-W29")
        XCTAssertTrue(plan.planned)
        XCTAssertEqual(plan.goals.count, 2)
        XCTAssertEqual(plan.goals[0].text, "Ship mobile core")
        XCTAssertFalse(plan.goals[0].done)
        XCTAssertEqual(plan.goals[1].text, "Write tests")
        XCTAssertTrue(plan.goals[1].done)
    }

    func testDecodeWeeklyPlanNotPlanned() throws {
        let json = """
        {"week": "2026-W28", "planned": false, "goals": []}
        """
        let plan = try CoreDecoding.decode(WeeklyPlan.self, from: json)
        XCTAssertEqual(plan.week, "2026-W28")
        XCTAssertFalse(plan.planned)
        XCTAssertTrue(plan.goals.isEmpty)
    }

    // MARK: - WeeklySummary decode

    func testDecodeWeeklySummaryWithDoneDays() throws {
        let json = """
        {
            "week": "2026-W29",
            "start": "2026-07-13",
            "end":   "2026-07-19",
            "summarized": false,
            "reviewed": true,
            "goals": [{"text": "Finish report", "done": true}],
            "done_by_day": [
                {"date": "2026-07-14", "items": ["Fix bug", "Review PR"]},
                {"date": "2026-07-15", "items": ["Write docs"]}
            ]
        }
        """
        let s = try CoreDecoding.decode(WeeklySummary.self, from: json)
        XCTAssertEqual(s.week, "2026-W29")
        XCTAssertEqual(s.start, "2026-07-13")
        XCTAssertEqual(s.end, "2026-07-19")
        XCTAssertFalse(s.summarized)
        XCTAssertTrue(s.reviewed)
        XCTAssertEqual(s.goals.count, 1)
        XCTAssertTrue(s.goals[0].done)
        XCTAssertEqual(s.doneByDay.count, 2)
        XCTAssertEqual(s.doneByDay[0].date, "2026-07-14")
        XCTAssertEqual(s.doneByDay[0].items, ["Fix bug", "Review PR"])
        XCTAssertEqual(s.doneByDay[1].items, ["Write docs"])
    }

    func testDecodeWeeklySummaryEmptyDoneDays() throws {
        let json = """
        {
            "week": "2026-W28", "start": "2026-07-06", "end": "2026-07-12",
            "summarized": true, "reviewed": false, "goals": [], "done_by_day": []
        }
        """
        let s = try CoreDecoding.decode(WeeklySummary.self, from: json)
        XCTAssertTrue(s.goals.isEmpty)
        XCTAssertTrue(s.doneByDay.isEmpty)
    }

    // MARK: - PendingWeek decode (both shapes)

    func testDecodePendingWeekWithWeek() throws {
        let json = #"{"pending":true,"week":"2026-W27"}"#
        let p = try CoreDecoding.decode(PendingWeek.self, from: json)
        XCTAssertTrue(p.pending)
        XCTAssertEqual(p.week, "2026-W27")
    }

    func testDecodePendingWeekFalseNoWeekField() throws {
        // When pending=false, Go's omitempty omits the week field entirely.
        let json = #"{"pending":false}"#
        let p = try CoreDecoding.decode(PendingWeek.self, from: json)
        XCTAssertFalse(p.pending)
        XCTAssertNil(p.week)
    }

    func testDecodePendingWeekFalseWithEmptyWeek() throws {
        // Some callers may include week="" when pending=false; handle gracefully.
        let json = #"{"pending":false,"week":""}"#
        let p = try CoreDecoding.decode(PendingWeek.self, from: json)
        XCTAssertFalse(p.pending)
        // week field is present but empty — still read whatever the wire sends.
        XCTAssertEqual(p.week, "")
    }

    // MARK: - ReviewAction fail-loud decode

    func testReviewActionUnknownValueFails() throws {
        let json = #"{"text":"task","action":9}"#
        XCTAssertThrowsError(
            try CoreDecoding.decode(ReviewItemDecision.self, from: json)
        ) { error in
            XCTAssertTrue(error is CoreError,
                          "Expected CoreError.contractViolation, got \(error)")
        }
    }

    func testReviewActionAllKnownValuesDecodeOk() throws {
        for (raw, expected): (Int, ReviewAction) in [(0, .keep), (1, .postpone), (2, .drop)] {
            let json = #"{"text":"x","action":\#(raw)}"#
            let decision = try CoreDecoding.decode(ReviewItemDecision.self, from: json)
            XCTAssertEqual(decision.action, expected,
                           "Action \(raw) should decode to \(expected)")
        }
    }

    // MARK: - WeekReviewDecisions encode

    func testEncodeWeekReviewDecisionsExactJSON() throws {
        let decisions = WeekReviewDecisions(
            decisions: [
                ReviewItemDecision(text: "old task", action: .drop),
                ReviewItemDecision(text: "carry-on", action: .postpone),
            ],
            rollover: true
        )
        let json = try CoreDecoding.encode(decisions)
        let parsed = try XCTUnwrap(
            json.data(using: .utf8).flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] }
        )
        XCTAssertNotNil(parsed["decisions"],  "Expected 'decisions' key")
        XCTAssertEqual(parsed["rollover"] as? Bool, true, "Expected rollover=true")

        let items = try XCTUnwrap(parsed["decisions"] as? [[String: Any]])
        XCTAssertEqual(items.count, 2)
        XCTAssertEqual(items[0]["text"] as? String, "old task")
        XCTAssertEqual(items[0]["action"] as? Int, 2)   // .drop = 2
        XCTAssertEqual(items[1]["text"] as? String, "carry-on")
        XCTAssertEqual(items[1]["action"] as? Int, 1)   // .postpone = 1
    }

    // MARK: - ISO-week → Monday conversion

    func testISOWeekToMondayNominalWeek() throws {
        // 2026-W29: week 29 of 2026 starts on Monday 2026-07-13
        let monday = try XCTUnwrap(DateFormatting.date(fromISOWeek: "2026-W29"))
        let formatted = DateFormatting.string(from: monday)
        XCTAssertEqual(formatted, "2026-07-13",
                       "2026-W29 Monday should be 2026-07-13, got \(formatted)")
    }

    func testISOWeekToMondayWeek1() throws {
        // 2026-W01 starts on Monday 2025-12-29 (ISO 8601 year-boundary week)
        let monday = try XCTUnwrap(DateFormatting.date(fromISOWeek: "2026-W01"))
        let formatted = DateFormatting.string(from: monday)
        XCTAssertEqual(formatted, "2025-12-29",
                       "2026-W01 Monday should be 2025-12-29 (year boundary), got \(formatted)")
    }

    func testISOWeekToMondayWeek53() throws {
        // 2015 has 53 ISO weeks; W53 starts on Monday 2015-12-28
        let monday = try XCTUnwrap(DateFormatting.date(fromISOWeek: "2015-W53"))
        let formatted = DateFormatting.string(from: monday)
        XCTAssertEqual(formatted, "2015-12-28",
                       "2015-W53 Monday should be 2015-12-28, got \(formatted)")
    }

    func testISOWeekToMondayMalformedReturnsNil() {
        XCTAssertNil(DateFormatting.date(fromISOWeek: "2026-29"))
        XCTAssertNil(DateFormatting.date(fromISOWeek: "not-a-week"))
        XCTAssertNil(DateFormatting.date(fromISOWeek: ""))
    }

    // MARK: - Goals array rebuild: preserves order and states

    func testGoalsArrayRoundTrip() throws {
        // Verifies that encoding [WeeklyGoal] produces the exact JSON the core expects.
        let goals = [
            WeeklyGoal(text: "Ship it", done: false),
            WeeklyGoal(text: "Write tests", done: true),
        ]
        let json = try CoreDecoding.encode(goals)
        // Decode back to confirm round-trip fidelity.
        let decoded = try CoreDecoding.decode([WeeklyGoal].self, from: json)
        XCTAssertEqual(decoded.count, 2)
        XCTAssertEqual(decoded[0].text, "Ship it")
        XCTAssertFalse(decoded[0].done)
        XCTAssertEqual(decoded[1].text, "Write tests")
        XCTAssertTrue(decoded[1].done)
    }

    func testEmptyGoalsArrayEncodesAsEmptyArray() throws {
        // SetWeeklyPlan MUST receive "[]" not "" or "null" to clear the plan.
        let json = try CoreDecoding.encode([WeeklyGoal]())
        XCTAssertEqual(json, "[]", "Empty goals array must encode as '[]' per core contract")
    }
}
