import Foundation

// MARK: - Sync (SyncNow / ConflictsJSON)

/// Root response of SyncNow. The token field is omitted when not refreshed.
struct SyncResult: Codable {
    var conflicts: [SyncConflict]
    /// Updated OAuth JSON when the access token was refreshed during the sync.
    /// When present, the host MUST write it back to the Keychain immediately.
    var token: String?

    enum CodingKeys: String, CodingKey {
        case conflicts, token
    }
}

struct SyncConflict: Codable, Identifiable {
    var path: String
    var conflictCopy: String
    /// RFC 3339 timestamp of conflict detection.
    var time: String

    var id: String { path }

    enum CodingKeys: String, CodingKey {
        case path
        case conflictCopy = "conflict_copy"
        case time
    }
}

/// Conflict resolution choice values accepted by ResolveConflict.
enum ResolveChoice: String {
    case keepLocal = "keep_local"
    case keepRemote = "keep_remote"
    case keepBoth = "keep_both"
}

/// OAuth token wire shape (oauth2.Token JSON). Round-trips between Keychain and SyncNow.
struct OAuthToken: Codable {
    var accessToken: String
    var tokenType: String?
    var refreshToken: String?
    /// RFC 3339 expiry timestamp.
    var expiry: String?

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case tokenType = "token_type"
        case refreshToken = "refresh_token"
        case expiry
    }
}
