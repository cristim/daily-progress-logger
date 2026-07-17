import Foundation

// MARK: - Projects (ProjectsJSON)

struct Project: Codable, Identifiable {
    var id: String
    var name: String
    var status: ProjectStatus

    enum CodingKeys: String, CodingKey {
        case id, name, status
    }
}

enum ProjectStatus: String, Codable {
    case open
    case closed
}
