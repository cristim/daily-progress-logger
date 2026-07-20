package com.cristim.dailyprogress.util

import java.time.DayOfWeek
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.temporal.IsoFields

/**
 * Returns the current time as an RFC 3339 string with the device's local UTC offset,
 * e.g. "2026-07-17T09:35:00+02:00". On UTC devices the offset is "+00:00" (never "Z").
 *
 * IMPORTANT: always use this function when calling DuePromptsJSON — never pass
 * Instant.now().toString() (which yields a "Z" suffix) because the Core uses the
 * embedded offset for hour/minute comparisons in the device's timezone.
 * Passing UTC causes morning/evening check-ins to fire at wrong local times.
 *
 * The pattern uses lowercase 'xxx' (not uppercase 'X') so UTC emits "+00:00" rather
 * than "Z". Go parses both "+00:00" and "Z" correctly.
 *
 * Regression test: nowRfc3339Local() output must always end with a [+-]HH:MM offset.
 */
private val RFC3339_FORMATTER = DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ssxxx")

fun nowRfc3339Local(): String =
    OffsetDateTime.now().format(RFC3339_FORMATTER)

/**
 * Parses an ISO-8601 week string "YYYY-Www" (e.g. "2026-W29") and returns the
 * Monday [LocalDate] of that week. Uses [IsoFields] for correct year-boundary
 * handling (e.g. "2026-W01" may start on Dec 28 of the previous calendar year).
 *
 * Regression targets (unit-tested in WeekTest):
 *   "2026-W29" → 2026-07-13
 *   "2026-W01" → 2025-12-29
 *   "2015-W53" → 2015-12-28
 *
 * @throws IllegalArgumentException if the string is not in "YYYY-Www" format.
 */
fun isoWeekToMonday(isoWeek: String): LocalDate {
    val parts = isoWeek.split("-W")
    require(parts.size == 2 && parts[0].isNotEmpty() && parts[1].isNotEmpty()) {
        "Invalid ISO week string: $isoWeek (expected YYYY-Www)"
    }
    val weekYear = parts[0].toInt()
    val weekNumber = parts[1].toInt()
    // Jan 4 is always in week 1 of the ISO week-based year — use it as an anchor.
    return LocalDate.of(weekYear, 1, 4)
        .with(IsoFields.WEEK_OF_WEEK_BASED_YEAR, weekNumber.toLong())
        .with(DayOfWeek.MONDAY)
}
