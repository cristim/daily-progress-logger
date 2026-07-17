import Foundation

// MARK: - Recurring (RecurringJSON / recurringTemplateDTO in dto.go)

// RecurringTemplate is used in two contexts with different wire shapes:
//
// 1. TreeJSON (dto.go recurringTemplateDTO) — all 9 fields present.
// 2. RecurringJSON management endpoint (recurring.go recurringTaskDTO) — only
//    {text, project, raw}; the schedule fields are absent.
//
// Fields present only in the full tree shape are optional so decoding succeeds
// in both contexts.

struct RecurringTemplate: Codable, Identifiable {
    var text: String
    var project: String
    /// Human-readable schedule description (e.g. "daily 09:00"). Absent from the
    /// RecurringJSON management response; present in the TreeJSON response.
    var describe: String?
    var kind: Int?
    var weekday: Int?
    var monthDay: Int?
    var hour: Int?
    var minute: Int?
    /// Raw stored line; pass to RemoveRecurring to delete this template.
    var raw: String

    var id: String { raw }

    enum CodingKeys: String, CodingKey {
        case text, project, describe, kind, weekday
        case monthDay = "month_day"
        case hour, minute, raw
    }
}
