import Foundation

// MARK: - Date formatting utilities

/// Shared formatters for the YYYY-MM-DD wire format used throughout the core contract.
enum DateFormatting {
    /// Formats a Date as "YYYY-MM-DD" in device-local calendar, matching the core's
    /// `time.Local` semantics. Always use this instead of ISO8601DateFormatter so
    /// timezone handling is consistent with Qt's local-midnight logic.
    static func string(from date: Date) -> String {
        formatter.string(from: date)
    }

    /// Parses a "YYYY-MM-DD" string into a Date at midnight in device-local time.
    static func date(from string: String) -> Date? {
        formatter.date(from: string)
    }

    private static let formatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        f.locale = Locale(identifier: "en_US_POSIX")
        f.calendar = Calendar.current   // local calendar, matching core's time.Local
        return f
    }()
}

extension Date {
    /// "YYYY-MM-DD" string in device-local time.
    var coreDate: String { DateFormatting.string(from: self) }

    /// RFC 3339 timestamp suitable for DuePromptsJSON.
    var rfc3339: String {
        ISO8601DateFormatter().string(from: self)
    }
}
