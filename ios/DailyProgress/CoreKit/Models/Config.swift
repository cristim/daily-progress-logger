import Foundation

// MARK: - Config (ConfigJSON / SetConfig)

struct CoreConfig: Codable {
    var morningTime: String?
    var eveningTime: String?
    var summaryDay: String?
    var summaryTime: String?
    var googleClientID: String?
    var notifyCheckins: Bool?

    enum CodingKeys: String, CodingKey {
        case morningTime = "morning_time"
        case eveningTime = "evening_time"
        case summaryDay = "summary_day"
        case summaryTime = "summary_time"
        case googleClientID = "google_client_id"
        case notifyCheckins = "notify_checkins"
    }
}
