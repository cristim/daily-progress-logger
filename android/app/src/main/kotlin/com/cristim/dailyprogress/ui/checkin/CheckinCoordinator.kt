package com.cristim.dailyprogress.ui.checkin

import android.content.SharedPreferences
import android.util.Log
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.DuePromptDto
import com.cristim.dailyprogress.model.PromptId
import com.cristim.dailyprogress.util.nowRfc3339Local
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId

// ---------------------------------------------------------------------------
// Storage abstraction (allows pure-JVM unit tests without Android context)
// ---------------------------------------------------------------------------

/**
 * Persistence interface for per-prompt snooze and skip-today state.
 * Decoupled from Android [SharedPreferences] so tests can use an in-memory map.
 */
interface SnoozeSkipStorage {
    /** Epoch-millis of the snooze deadline, or 0 if not snoozed. */
    fun snoozeUntil(promptId: Int): Long
    fun setSnoozeUntil(promptId: Int, epochMillis: Long)
    /** YYYY-MM-DD date string of the day the prompt was skipped, or "" if not skipped. */
    fun skippedOn(promptId: Int): String
    fun setSkippedOn(promptId: Int, date: String)
}

/** [SnoozeSkipStorage] backed by [SharedPreferences]. */
class SharedPrefsSnoozeSkipStorage(private val prefs: SharedPreferences) : SnoozeSkipStorage {
    override fun snoozeUntil(promptId: Int): Long =
        prefs.getLong("snooze_until_$promptId", 0L)

    override fun setSnoozeUntil(promptId: Int, epochMillis: Long) {
        prefs.edit().putLong("snooze_until_$promptId", epochMillis).apply()
    }

    override fun skippedOn(promptId: Int): String =
        prefs.getString("skipped_on_$promptId", "").orEmpty()

    override fun setSkippedOn(promptId: Int, date: String) {
        prefs.edit().putString("skipped_on_$promptId", date).apply()
    }
}

// ---------------------------------------------------------------------------
// Coordinator
// ---------------------------------------------------------------------------

/**
 * Routing logic for due check-in prompts, matching Qt's CheckPrompts / runPrompt
 * (app.go).
 *
 * Ownership: instantiated once in [RootScaffold] and called on each app resume
 * via `LaunchedEffect`. Snooze/skip state is persisted so it survives process
 * death.
 *
 * Phase B: WEEK_REVIEW (0), WEEKLY_PLAN (1), WEEKLY_SUMMARY (4) are explicitly
 * ignored here — they route to phase-B sheets once those land.
 */
class CheckinCoordinator(
    private val repository: CoreRepository,
    private val storage: SnoozeSkipStorage,
) {
    /**
     * Returns the first due prompt that is neither snoozed nor skipped today,
     * or null when nothing is actionable.
     *
     * DuePrompts failure is non-fatal (advisory path): returns null and logs.
     * Unknown prompt IDs are logged and skipped — never crash the coordinator.
     */
    suspend fun nextPresentable(): DuePromptDto? {
        val prompts = try {
            repository.duePrompts(nowRfc3339Local())
        } catch (e: Exception) {
            Log.w(TAG, "Failed to fetch due prompts: ${e.message}")
            return null
        }

        val nowMillis = System.currentTimeMillis()
        val today = LocalDate.now().toString()

        return prompts.due.firstOrNull { prompt ->
            val id = try {
                PromptId.fromWire(prompt.id)
            } catch (_: Exception) {
                Log.w(TAG, "Unknown prompt id ${prompt.id}, skipping")
                return@firstOrNull false
            }
            when (id) {
                PromptId.MORNING, PromptId.EVENING -> {
                    nowMillis >= storage.snoozeUntil(prompt.id) &&
                        storage.skippedOn(prompt.id) != today
                }
                PromptId.WEEK_REVIEW, PromptId.WEEKLY_PLAN, PromptId.WEEKLY_SUMMARY -> {
                    // phase B: not routed until week screens land
                    false
                }
            }
        }
    }

    /**
     * Snoozes [promptId] for 1 hour, capped at 23:59:59 of the current day.
     * Matches Qt's snooze-capped-at-midnight rule.
     */
    fun snooze(promptId: Int) {
        val nowMillis = System.currentTimeMillis()
        val oneHourLater = nowMillis + ONE_HOUR_MS
        val endOfDayMillis = LocalDateTime.of(LocalDate.now(), LocalTime.MAX)
            .atZone(ZoneId.systemDefault())
            .toInstant()
            .toEpochMilli()
        storage.setSnoozeUntil(promptId, minOf(oneHourLater, endOfDayMillis))
    }

    /**
     * Persists skip-today for [promptId]. The prompt is suppressed until tomorrow.
     * Only called from scheduled presentation — manual "Close" does no bookkeeping
     * (matches Qt: manual context menu has no Skip-Today equivalent).
     */
    fun skipToday(promptId: Int) {
        storage.setSkippedOn(promptId, LocalDate.now().toString())
    }

    companion object {
        private const val TAG = "CheckinCoordinator"
        private const val ONE_HOUR_MS = 3_600_000L
    }
}
