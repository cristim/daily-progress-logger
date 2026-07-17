import Foundation

// MARK: - Recurring (RecurringJSON / recurringTemplateDTO in dto.go)

struct RecurringTemplate: Codable, Identifiable {
    var text: String
    var project: String
    var describe: String
    var kind: Int
    var weekday: Int
    var monthDay: Int
    var hour: Int
    var minute: Int
    /// Raw stored line; pass to RemoveRecurring to delete this template.
    var raw: String

    var id: String { raw }

    enum CodingKeys: String, CodingKey {
        case text, project, describe, kind, weekday
        case monthDay = "month_day"
        case hour, minute, raw
    }
}
