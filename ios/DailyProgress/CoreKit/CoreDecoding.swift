import Foundation

// MARK: - Centralized decode/encode helper

/// Single decode/encode surface for all JSON coming from or going to the Go core.
/// Funnelling every call through here ensures:
///   - decode failures always surface as CoreError.contractViolation (never swallowed)
///   - the same JSONDecoder configuration is used everywhere (no per-call inconsistencies)
///   - encode failures surface loudly, matching the fail-loud contract
enum CoreDecoding {
    private static let decoder = JSONDecoder()

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        // Do not sort keys; preserve natural order matching Go's encoding/json output.
        return e
    }()

    /// Decodes a JSON string from the core into T.
    /// Any failure is rethrown as CoreError.contractViolation so callers can treat
    /// decode errors and network/core errors uniformly.
    static func decode<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        guard let data = json.data(using: .utf8) else {
            throw CoreError.contractViolation("JSON string is not valid UTF-8")
        }
        do {
            return try decoder.decode(type, from: data)
        } catch {
            throw CoreError.contractViolation(error.localizedDescription)
        }
    }

    /// Encodes a value to a JSON string for sending to the core.
    /// Any failure is rethrown as CoreError.contractViolation.
    static func encode<T: Encodable>(_ value: T) throws -> String {
        do {
            let data = try encoder.encode(value)
            guard let json = String(data: data, encoding: .utf8) else {
                throw CoreError.contractViolation("Encoded JSON is not valid UTF-8")
            }
            return json
        } catch let error as CoreError {
            throw error
        } catch {
            throw CoreError.contractViolation(error.localizedDescription)
        }
    }
}
