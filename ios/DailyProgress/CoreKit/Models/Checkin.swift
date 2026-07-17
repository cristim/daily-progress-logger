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

// MorningCandidatesJSON returns a BARE array (see checkin.go MorningCandidatesJSON):
//   [{"text":"…","from_backlog":false}, …]
// Decode as [MorningCandidate] directly — there is no wrapping object.

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
/// Unknown int values throw at decode time — fail-loud to catch contract drift.
enum EveningAction: Int, Codable {
    case todo = 0
    case done = 1
    case nextDay = 2
    case nextWeek = 3
    case backlog = 4

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(Int.self)
        guard let value = EveningAction(rawValue: raw) else {
            throw DecodingError.dataCorruptedError(
                in: try decoder.singleValueContainer(),
                debugDescription: "Unknown EveningAction \(raw); expected 0-4"
            )
        }
        self = value
    }
}
