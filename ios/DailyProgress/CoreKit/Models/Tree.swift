import Foundation

// MARK: - Tree (TreeJSON wire types)
// Mirrors projectTreeDTO / taskDTO / recurringTemplateDTO in mobilecore/dto.go.
// All field names use explicit CodingKeys so the contract is visible and greppable.

struct ProjectTree: Codable {
    var projects: [TreeProject]
    var unfiled: [TreeTask]
    var recycled: [TreeTask]
    var recurring: [RecurringTemplate]

    enum CodingKeys: String, CodingKey {
        case projects, unfiled, recycled, recurring
    }
}

struct TreeProject: Codable, Identifiable {
    var id: String
    var name: String
    var done: Bool
    var tasks: [TreeTask]

    enum CodingKeys: String, CodingKey {
        case id, name, done, tasks
    }
}

struct TreeTask: Codable, Identifiable {
    /// Stable plan-file index; pass back verbatim as the action address.
    var index: Int
    /// Nesting level: 0 = top-level, 1 = subtask, etc.
    var depth: Int
    /// Display text (project tag stripped). This is the CAS expectedText.
    var text: String
    /// Wire state string: "todo" | "done" | "postponed".
    var state: ItemState
    /// YYYY-MM-DD of the day this task belongs to.
    var date: String
    /// Rollup done state (true when all children are done).
    var done: Bool
    /// Project display name; omitted from JSON when absent (omitempty on Go side).
    var project: String?
    /// Direct children (always [], never null).
    var children: [TreeTask]

    /// Stable Identifiable id composed from date + index.
    var id: String { "\(date)#\(index)" }

    enum CodingKeys: String, CodingKey {
        case index, depth, text, state, date, done, project, children
    }
}

/// Wire state for tasks and goals: "todo" | "done" | "postponed".
/// Decoding an unknown string throws CoreError.contractViolation via the
/// fail-loud init below — contract drift is a visible error, never a silent default.
enum ItemState: String, Codable {
    case todo
    case done
    case postponed

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        guard let value = ItemState(rawValue: raw) else {
            throw DecodingError.dataCorruptedError(
                in: try decoder.singleValueContainer(),
                debugDescription: "Unknown ItemState '\(raw)'; expected todo/done/postponed"
            )
        }
        self = value
    }
}
