import Foundation

// MARK: - Due prompts (DuePromptsJSON)

struct DuePrompts: Codable {
    var due: [DuePrompt]

    enum CodingKeys: String, CodingKey {
        case due
    }
}

struct DuePrompt: Codable, Identifiable, Equatable {
    var id: PromptID
    var name: String

    enum CodingKeys: String, CodingKey {
        case id, name
    }
}

/// Prompt IDs as returned by DuePromptsJSON.
/// Unknown int values throw at decode time — fail-loud to catch contract drift.
enum PromptID: Int, Codable {
    case weekReview = 0
    case weeklyPlan = 1
    case morning = 2
    case evening = 3
    case weeklySummary = 4

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(Int.self)
        guard let value = PromptID(rawValue: raw) else {
            throw DecodingError.dataCorruptedError(
                in: try decoder.singleValueContainer(),
                debugDescription: "Unknown PromptID \(raw); expected 0-4"
            )
        }
        self = value
    }
}
