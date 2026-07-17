import Foundation

/// Errors from the Go core, classified by the stable prefix codes in dto.go.
enum CoreError: Error, LocalizedError {
    case casMismatch                   // CAS_MISMATCH: tree is stale, refresh and re-present
    case notFound(String)              // NOT_FOUND:
    case badInput(String)              // BAD_INPUT: validation failure
    case syncAuth(String)              // SYNC_AUTH: re-authenticate
    case contractViolation(String)     // Swift-side decode failure (contract drift)
    case other(String)                 // unprefixed / INTERNAL

    var errorDescription: String? {
        switch self {
        case .casMismatch:
            return "The task list changed. Please retry."
        case .notFound(let msg):
            return "Not found: \(msg)"
        case .badInput(let msg):
            return "Invalid input: \(msg)"
        case .syncAuth(let msg):
            return "Authentication error: \(msg)"
        case .contractViolation(let msg):
            return "Data format error: \(msg)"
        case .other(let msg):
            return msg
        }
    }

    /// Classify an error from the gomobile boundary by prefix-matching
    /// the localizedDescription against the stable code tokens from dto.go.
    static func classify(_ error: Error) -> CoreError {
        let msg = error.localizedDescription
        if msg.hasPrefix("CAS_MISMATCH:") {
            return .casMismatch
        } else if msg.hasPrefix("NOT_FOUND:") {
            return .notFound(after(prefix: "NOT_FOUND: ", in: msg))
        } else if msg.hasPrefix("BAD_INPUT:") {
            return .badInput(after(prefix: "BAD_INPUT: ", in: msg))
        } else if msg.hasPrefix("SYNC_AUTH:") {
            return .syncAuth(after(prefix: "SYNC_AUTH: ", in: msg))
        } else {
            return .other(msg)
        }
    }

    private static func after(prefix: String, in msg: String) -> String {
        guard msg.hasPrefix(prefix) else { return msg }
        return String(msg.dropFirst(prefix.count))
    }
}
