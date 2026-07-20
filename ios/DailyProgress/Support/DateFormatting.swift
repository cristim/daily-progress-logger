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

    /// Parses an ISO-8601 week string ("YYYY-WNN", e.g. "2026-W29") and returns the
    /// Monday of that week. Uses Calendar(identifier: .iso8601) so year-boundary weeks
    /// (e.g. "2025-W53", "2026-W01") are handled correctly.
    /// Returns nil if the string is malformed or the date cannot be derived.
    static func date(fromISOWeek isoWeek: String) -> Date? {
        // Split on "-W" so "2026-W29" yields yearPart="2026", weekPart="29"
        guard let dashW = isoWeek.range(of: "-W") else { return nil }
        let yearPart = String(isoWeek[isoWeek.startIndex..<dashW.lowerBound])
        let weekPart = String(isoWeek[dashW.upperBound...])
        guard let year = Int(yearPart), let weekOfYear = Int(weekPart) else { return nil }
        var components = DateComponents()
        components.weekOfYear = weekOfYear
        components.yearForWeekOfYear = year
        components.weekday = 2  // Monday in ISO-8601 calendar (Monday = first day of week)
        return iso8601Calendar.date(from: components)
    }

    /// Formats a "YYYY-MM-DD" date string as a short day header for display
    /// (e.g. "Fri 17 Jul"). Returns the raw string unchanged if it cannot be parsed.
    static func dayHeader(from dateString: String) -> String {
        guard let d = date(from: dateString) else { return dateString }
        return dayHeaderFormatter.string(from: d)
    }

    /// Returns a compact ISO-8601 week label (e.g. "2026-W29") from any date.
    /// Uses the ISO-8601 calendar so week numbers match Go's isoweek package output.
    static func isoWeekLabel(from date: Date) -> String {
        isoWeekFormatter.string(from: date)
    }

    private static let formatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        f.locale = Locale(identifier: "en_US_POSIX")
        f.calendar = Calendar.current   // local calendar, matching core's time.Local
        return f
    }()

    // ISO-8601 calendar for week-number arithmetic; locale fixed to POSIX for
    // deterministic weekday constants (weekday 2 = Monday in ISO-8601).
    static let iso8601Calendar: Calendar = {
        var cal = Calendar(identifier: .iso8601)
        cal.locale = Locale(identifier: "en_US_POSIX")
        return cal
    }()

    private static let dayHeaderFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "EEE d MMM"
        f.locale = Locale(identifier: "en_US_POSIX")
        return f
    }()

    private static let isoWeekFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "YYYY-'W'ww"
        f.calendar = Calendar(identifier: .iso8601)
        f.locale = Locale(identifier: "en_US_POSIX")
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
