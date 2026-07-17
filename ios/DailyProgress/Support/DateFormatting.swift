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
    /// Carries the device-local UTC offset (e.g. "2026-07-17T09:35:00+02:00"), never "Z".
    /// The Go core reads this offset directly for hour/minute comparisons, so emitting
    /// UTC would break morning/evening prompt timing for any non-UTC user.
    var rfc3339: String {
        Date.rfc3339Formatter.string(from: self)
    }

    /// Shared, pre-built formatter. Setting timeZone = .current replaces the default
    /// "Z" suffix with the signed local offset ("+HH:MM" or "-HH:MM"), which is what
    /// time.Parse(time.RFC3339, …) expects and what schedule.go uses for comparisons.
    private static let rfc3339Formatter: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withColonSeparatorInTimeZone]
        f.timeZone = .current
        return f
    }()
}
