import Foundation

// MARK: - Weekly (WeeklyPlanJSON / WeeklySummaryJSON / WeekReviewCandidatesJSON)

struct WeeklyGoal: Codable, Identifiable {
    var text: String
    var done: Bool

    var id: String { text }

    enum CodingKeys: String, CodingKey {
        case text, done
    }
}

struct WeeklyPlan: Codable {
    var week: String
    var planned: Bool
    var goals: [WeeklyGoal]

    enum CodingKeys: String, CodingKey {
        case week, planned, goals
    }
}

struct WeekReviewCandidates: Codable {
    var week: String
    var candidates: [String]

    enum CodingKeys: String, CodingKey {
        case week, candidates
    }
}

/// Payload sent to ApplyWeekReview.
struct WeekReviewDecisions: Codable {
    var decisions: [ReviewItemDecision]
    var rollover: Bool

    enum CodingKeys: String, CodingKey {
        case decisions, rollover
    }
}

struct ReviewItemDecision: Codable {
    var text: String
    var action: ReviewAction

    enum CodingKeys: String, CodingKey {
        case text, action
    }
}

/// Week-review triage action (int-coded on the wire).
/// Unknown int values throw at decode time — fail-loud to catch contract drift.
enum ReviewAction: Int, Codable {
    case keep = 0
    case postpone = 1
    case drop = 2

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(Int.self)
        guard let value = ReviewAction(rawValue: raw) else {
            throw DecodingError.dataCorruptedError(
                in: try decoder.singleValueContainer(),
                debugDescription: "Unknown ReviewAction \(raw); expected 0=keep 1=postpone 2=drop"
            )
        }
        self = value
    }
}

struct WeeklySummary: Codable {
    var week: String
    var start: String
    var end: String
    var summarized: Bool
    var reviewed: Bool
    var goals: [WeeklyGoal]
    var doneByDay: [DayDone]

    enum CodingKeys: String, CodingKey {
        case week, start, end, summarized, reviewed, goals
        case doneByDay = "done_by_day"
    }
}

struct DayDone: Codable, Identifiable {
    var date: String
    var items: [String]

    var id: String { date }

    enum CodingKeys: String, CodingKey {
        case date, items
    }
}

/// Response from WeeklySummaryPendingJSON and UnreviewedWeekJSON.
struct PendingWeek: Codable {
    var pending: Bool
    var week: String?

    enum CodingKeys: String, CodingKey {
        case pending, week
    }
}
