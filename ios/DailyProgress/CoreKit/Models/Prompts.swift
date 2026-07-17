import Foundation

// MARK: - Due prompts (DuePromptsJSON)

struct DuePrompts: Codable {
    var due: [DuePrompt]

    enum CodingKeys: String, CodingKey {
        case due
    }
}

struct DuePrompt: Codable, Identifiable {
    var id: PromptID
    var name: String

    enum CodingKeys: String, CodingKey {
        case id, name
    }
}

/// Prompt IDs as returned by DuePromptsJSON.
enum PromptID: Int, Codable {
    case weekReview = 0
    case weeklyPlan = 1
    case morning = 2
    case evening = 3
    case weeklySummary = 4
}
