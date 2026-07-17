import Foundation

// MARK: - Check-in types (MorningCandidatesJSON / ApplyMorning / ApplyEvening)

struct MorningCandidate: Codable, Identifiable {
    var text: String
    var fromBacklog: Bool

    var id: String { text }

    enum CodingKeys: String, CodingKey {
        case text
        case fromBacklog = "from_backlog"
    }
}

/// Root of MorningCandidatesJSON response.
struct MorningCandidates: Codable {
    var candidates: [MorningCandidate]

    enum CodingKeys: String, CodingKey {
        case candidates
    }
}

/// Payload sent to ApplyMorning.
struct MorningDecisions: Codable {
    var newItems: [String]
    var adopted: [MorningCandidate]

    enum CodingKeys: String, CodingKey {
        case newItems = "new_items"
        case adopted
    }
}

/// Payload sent to ApplyEvening.
struct EveningDecisions: Codable {
    var decisions: [EveningItemDecision]
    var extraDone: [String]

    enum CodingKeys: String, CodingKey {
        case decisions
        case extraDone = "extra_done"
    }
}

struct EveningItemDecision: Codable {
    var text: String
    var action: EveningAction

    enum CodingKeys: String, CodingKey {
        case text, action
    }
}

/// Evening triage action (int-coded on the wire).
enum EveningAction: Int, Codable {
    case todo = 0
    case done = 1
    case nextDay = 2
    case nextWeek = 3
    case backlog = 4
}
