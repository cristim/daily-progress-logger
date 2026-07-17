package com.cristim.dailyprogress

import com.cristim.dailyprogress.model.PromptId
import com.cristim.dailyprogress.ui.checkin.CheckinCoordinator
import com.cristim.dailyprogress.ui.checkin.SnoozeSkipStorage
import com.cristim.dailyprogress.util.nowRfc3339Local
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId

/**
 * Pure JVM tests for the check-in coordinator (snooze/skip) and the
 * nowRfc3339Local() time helper.
 *
 * Uses an in-memory [SnoozeSkipStorage] implementation so Android context is
 * not required. Coordinator DuePrompts calls require a real CoreRepository so
 * they are tested separately in integration; here we test only persistence logic.
 */
class CheckinTest {

    // -----------------------------------------------------------------------
    // In-memory SnoozeSkipStorage for unit tests
    // -----------------------------------------------------------------------

    private class InMemoryStorage : SnoozeSkipStorage {
        val snoozeMap = mutableMapOf<Int, Long>()
        val skipMap = mutableMapOf<Int, String>()

        override fun snoozeUntil(promptId: Int): Long = snoozeMap[promptId] ?: 0L
        override fun setSnoozeUntil(promptId: Int, epochMillis: Long) {
            snoozeMap[promptId] = epochMillis
        }
        override fun skippedOn(promptId: Int): String = skipMap[promptId] ?: ""
        override fun setSkippedOn(promptId: Int, date: String) {
            skipMap[promptId] = date
        }
    }

    // -----------------------------------------------------------------------
    // Snooze tests
    // -----------------------------------------------------------------------

    @Test
    fun `snooze sets snooze deadline approximately one hour from now`() {
        val storage = InMemoryStorage()
        val coordinator = CheckinCoordinator(
            repository = null, // snooze() does not call the repository
            storage = storage,
        )
        val before = System.currentTimeMillis()
        coordinator.snooze(PromptId.MORNING.wire)
        val after = System.currentTimeMillis()

        val snoozeUntil = storage.snoozeUntil(PromptId.MORNING.wire)
        // Should be between (before + 1h) and (after + 1h), capped at end of day
        val endOfDay = LocalDateTime.of(LocalDate.now(), LocalTime.MAX)
            .atZone(ZoneId.systemDefault()).toInstant().toEpochMilli()

        val expectedMin = minOf(before + 3_600_000L, endOfDay)
        val expectedMax = minOf(after + 3_600_000L, endOfDay)
        assertTrue(
            "snooze deadline $snoozeUntil must be >= $expectedMin",
            snoozeUntil >= expectedMin,
        )
        assertTrue(
            "snooze deadline $snoozeUntil must be <= $expectedMax",
            snoozeUntil <= expectedMax,
        )
    }

    @Test
    fun `snooze cap - deadline does not exceed end of day`() {
        val storage = InMemoryStorage()
        val coordinator = CheckinCoordinator(
            repository = null,
            storage = storage,
        )
        coordinator.snooze(PromptId.EVENING.wire)
        val snoozeUntil = storage.snoozeUntil(PromptId.EVENING.wire)
        val endOfDay = LocalDateTime.of(LocalDate.now(), LocalTime.MAX)
            .atZone(ZoneId.systemDefault()).toInstant().toEpochMilli()
        assertTrue("snooze must not exceed end of day", snoozeUntil <= endOfDay)
    }

    @Test
    fun `snooze is per prompt id and does not bleed across prompts`() {
        val storage = InMemoryStorage()
        val coordinator = CheckinCoordinator(null, storage)
        coordinator.snooze(PromptId.MORNING.wire)
        // Evening should remain 0 (not snoozed)
        assertEquals(0L, storage.snoozeUntil(PromptId.EVENING.wire))
    }

    // -----------------------------------------------------------------------
    // Skip-today tests
    // -----------------------------------------------------------------------

    @Test
    fun `skipToday sets skippedOn to today for the given prompt`() {
        val storage = InMemoryStorage()
        val coordinator = CheckinCoordinator(null, storage)
        coordinator.skipToday(PromptId.MORNING.wire)
        assertEquals(LocalDate.now().toString(), storage.skippedOn(PromptId.MORNING.wire))
    }

    @Test
    fun `skipToday is per prompt id and does not bleed across prompts`() {
        val storage = InMemoryStorage()
        val coordinator = CheckinCoordinator(null, storage)
        coordinator.skipToday(PromptId.MORNING.wire)
        assertEquals("", storage.skippedOn(PromptId.EVENING.wire))
    }

    @Test
    fun `skipToday can be overwritten (re-skipping is idempotent)`() {
        val storage = InMemoryStorage()
        val coordinator = CheckinCoordinator(null, storage)
        coordinator.skipToday(PromptId.MORNING.wire)
        coordinator.skipToday(PromptId.MORNING.wire)
        assertEquals(LocalDate.now().toString(), storage.skippedOn(PromptId.MORNING.wire))
    }

    // -----------------------------------------------------------------------
    // nowRfc3339Local regression: must never produce a UTC "Z" suffix
    // -----------------------------------------------------------------------

    @Test
    fun `nowRfc3339Local produces a local-offset timestamp, never UTC Z suffix`() {
        val result = nowRfc3339Local()
        assertFalse(
            "nowRfc3339Local() must never end with Z (UTC). Got: $result",
            result.endsWith("Z"),
        )
        // Must end with a numeric offset like +02:00 or -05:00
        val offsetPattern = Regex(".*[+-]\\d{2}:\\d{2}$")
        assertTrue(
            "nowRfc3339Local() must end with [+-]HH:MM offset. Got: $result",
            offsetPattern.matches(result),
        )
    }

    @Test
    fun `nowRfc3339Local produces a parseable RFC3339 timestamp`() {
        val result = nowRfc3339Local()
        // java.time.OffsetDateTime.parse should succeed without exception
        val parsed = java.time.OffsetDateTime.parse(result)
        // Parsed timestamp must be close to now (within 5 seconds)
        val nowEpoch = System.currentTimeMillis()
        val parsedEpoch = parsed.toInstant().toEpochMilli()
        assertTrue(
            "Parsed timestamp must be within 5s of now",
            Math.abs(nowEpoch - parsedEpoch) < 5_000L,
        )
    }
}

