package com.cristim.dailyprogress.util

import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter

/**
 * Returns the current time as an RFC 3339 string with the device's local UTC offset,
 * e.g. "2026-07-17T09:35:00+02:00".
 *
 * IMPORTANT: always use this function when calling DuePromptsJSON — never pass
 * Instant.now().toString() (which yields a "Z" suffix) because the Core uses the
 * embedded offset for hour/minute comparisons in the device's timezone.
 * Passing UTC causes morning/evening check-ins to fire at wrong local times.
 * Regression test: nowRfc3339Local() output must never end with "Z".
 */
fun nowRfc3339Local(): String =
    OffsetDateTime.now().format(DateTimeFormatter.ISO_OFFSET_DATE_TIME)
