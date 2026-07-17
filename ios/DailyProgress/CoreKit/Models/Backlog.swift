import Foundation

// MARK: - Backlog (BacklogJSON)

struct Backlog: Codable {
    var current: [String]
    var nextWeek: [String]

    enum CodingKeys: String, CodingKey {
        case current
        case nextWeek = "next_week"
    }
}
