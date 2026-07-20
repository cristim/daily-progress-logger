package com.cristim.dailyprogress.util

import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter

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
