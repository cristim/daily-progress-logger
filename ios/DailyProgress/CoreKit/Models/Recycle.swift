import Foundation

// MARK: - Recycle bin (RecycleJSON)

struct RecycleEntry: Codable, Identifiable {
    var date: String
    var text: String
    var state: ItemState

    var id: String { "\(date)#\(text)" }

    enum CodingKeys: String, CodingKey {
        case date, text, state
    }
}
