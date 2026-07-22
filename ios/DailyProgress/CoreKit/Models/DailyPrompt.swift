import Foundation

// MARK: - Daily prompt (DailyPromptJSON / SetDailyPrompt)
// Mirrors dailyPromptDTO in mobilecore/dto.go: a single "text" field, "" when unset.

struct DailyPromptDTO: Codable {
    var text: String

    enum CodingKeys: String, CodingKey {
        case text
    }
}
