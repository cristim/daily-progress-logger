package com.cristim.dailyprogress.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.SerializationException
import kotlinx.serialization.Serializable

// ---------------------------------------------------------------------------
// Tree DTOs — mirrors projectTreeDTO / projectDTO / taskDTO / recurringTemplateDTO
// in mobilecore/dto.go (FROZEN wire contract).
// ---------------------------------------------------------------------------

@Serializable
data class TreeDto(
    val projects: List<TreeProjectDto> = emptyList(),
    val unfiled: List<TreeTaskDto> = emptyList(),
    val recycled: List<TreeTaskDto> = emptyList(),
    val recurring: List<RecurringTemplateDto> = emptyList(),
)

@Serializable
data class TreeProjectDto(
    val id: String,
    val name: String,
    val done: Boolean,
    val tasks: List<TreeTaskDto> = emptyList(),
)

@Serializable
data class TreeTaskDto(
    /** Stable plan-file index; pass verbatim to every task-action call. */
    val index: Long,
    val depth: Int,
    /** Display text with project tag stripped. */
    val text: String,
    val state: TaskState,
    /** YYYY-MM-DD of the day this task belongs to. */
    val date: String,
    /** Rollup done state: true when all children are done. */
    val done: Boolean,
    /** Project display name; absent (not "") when task is unfiled. */
    val project: String = "",
    val children: List<TreeTaskDto> = emptyList(),
)

@Serializable
enum class TaskState {
    @SerialName("todo") TODO,
    @SerialName("done") DONE,
    @SerialName("postponed") POSTPONED,
}

/** Wire string for the three task states, matching mobilecore's stateTodoStr etc. */
fun TaskState.wireString(): String = when (this) {
    TaskState.TODO -> "todo"
    TaskState.DONE -> "done"
    TaskState.POSTPONED -> "postponed"
}

@Serializable
data class RecurringTemplateDto(
    val text: String,
    val project: String = "",
    val describe: String = "",
    val kind: Int = 0,
    val weekday: Int = 0,
    @SerialName("month_day") val monthDay: Int = 0,
    val hour: Int = 0,
    val minute: Int = 0,
    /** Raw stored line; pass to RemoveRecurring to delete. */
    val raw: String,
)

// ---------------------------------------------------------------------------
// Backlog
// ---------------------------------------------------------------------------

@Serializable
data class BacklogDto(
    val current: List<String> = emptyList(),
    @SerialName("next_week") val nextWeek: List<String> = emptyList(),
)

// ---------------------------------------------------------------------------
// Check-in DTOs
// ---------------------------------------------------------------------------

@Serializable
data class MorningCandidateDto(
    val text: String,
    @SerialName("from_backlog") val fromBacklog: Boolean,
)

/** Sent to ApplyMorning. */
@Serializable
data class MorningDecisionsDto(
    @SerialName("new_items") val newItems: List<String>,
    val adopted: List<MorningCandidateDto>,
)

/**
 * Typed representation of the evening action integer (mobilecore/checkin.go).
 * The [wire] value is what is sent to ApplyEvening; never use bare magic ints
 * in ViewModel or UI code — always go through this enum.
 *
 * Initial selection per state (EveningActionForState, matches Qt statebuttons.go):
 *   DONE state  → DONE
 *   POSTPONED   → NEXT_WEEK
 *   TODO        → TODO
 */
enum class EveningAction(val wire: Int) {
    TODO(0),
    DONE(1),
    NEXT_DAY(2),
    NEXT_WEEK(3),
    BACKLOG(4);

    companion object {
        /**
         * Converts a wire integer to [EveningAction].
         * Throws [SerializationException] on unknown values — fail loud, no silent default.
         */
        fun fromWire(n: Int): EveningAction =
            entries.firstOrNull { it.wire == n }
                ?: throw SerializationException("Unknown EveningAction wire value: $n (expected 0-4)")
    }
}

/** Wire DTO for one evening decision (serialized as-is to JSON). */
@Serializable
data class EveningDecisionDto(val text: String, val action: Int)

/** Sent to ApplyEvening. */
@Serializable
data class EveningDecisionsDto(
    val decisions: List<EveningDecisionDto>,
    @SerialName("extra_done") val extraDone: List<String>,
)

// ---------------------------------------------------------------------------
// Weekly DTOs
// ---------------------------------------------------------------------------

@Serializable
data class WeeklyGoalDto(val text: String, val done: Boolean = false)

@Serializable
data class WeeklyPlanDto(
    val week: String,
    val planned: Boolean,
    val goals: List<WeeklyGoalDto> = emptyList(),
)

@Serializable
data class WeekReviewCandidatesDto(
    val week: String,
    val candidates: List<String> = emptyList(),
)

/**
 * Typed representation of the week-review action integer (mobilecore/weekly.go).
 * The [wire] value is sent to ApplyWeekReview; never use bare magic ints in
 * ViewModel/UI code — always convert through this enum.
 *
 * 0=keep (stays in backlog Current), 1=postpone (move to Next week), 2=drop.
 */
enum class ReviewAction(val wire: Int) {
    KEEP(0),
    POSTPONE(1),
    DROP(2);

    companion object {
        /**
         * Converts a wire integer to [ReviewAction].
         * Throws [SerializationException] on unknown values — fail loud, no silent default.
         */
        fun fromWire(n: Int): ReviewAction =
            entries.firstOrNull { it.wire == n }
                ?: throw SerializationException("Unknown ReviewAction wire value: $n (expected 0-2)")
    }
}

/** 0=keep 1=postpone 2=drop (mobilecore/weekly.go). */
@Serializable
data class ReviewDecisionDto(val text: String, val action: Int)

/** Sent to ApplyWeekReview. */
@Serializable
data class ReviewDecisionsDto(
    val decisions: List<ReviewDecisionDto>,
    val rollover: Boolean,
)

@Serializable
data class DayDoneDto(
    val date: String,
    val items: List<String> = emptyList(),
)

@Serializable
data class WeeklySummaryDto(
    val week: String,
    val start: String,
    val end: String,
    val summarized: Boolean,
    val reviewed: Boolean,
    val goals: List<WeeklyGoalDto> = emptyList(),
    @SerialName("done_by_day") val doneByDay: List<DayDoneDto> = emptyList(),
)

/** Shared by WeeklySummaryPendingJSON and UnreviewedWeekJSON. */
@Serializable
data class PendingWeekDto(val pending: Boolean, val week: String = "")

// ---------------------------------------------------------------------------
// Projects / Recurring / Recycle
// ---------------------------------------------------------------------------

@Serializable
data class ProjectDto(val id: String, val name: String, val status: ProjectStatus)

@Serializable
enum class ProjectStatus {
    @SerialName("open") OPEN,
    @SerialName("closed") CLOSED,
}

@Serializable
data class RecycleEntryDto(
    val date: String,
    val text: String,
    val state: TaskState,
)

// ---------------------------------------------------------------------------
// Sync DTOs — mirrors conflictDTO / syncResultDTO in mobilecore/dto.go.
// ---------------------------------------------------------------------------

@Serializable
data class ConflictDto(
    val path: String,
    @SerialName("conflict_copy") val conflictCopy: String,
    /** RFC 3339 timestamp of conflict detection. */
    val time: String,
)

@Serializable
data class SyncResultDto(
    val conflicts: List<ConflictDto> = emptyList(),
    /**
     * Updated OAuth JSON when the access token was refreshed mid-run.
     * Empty string when the token was not refreshed (omitempty in Go).
     * IMPORTANT: when non-empty, persist immediately to EncryptedSharedPrefs.
     */
    val token: String = "",
)

@Serializable
enum class ConflictChoice {
    @SerialName("keep_local") KEEP_LOCAL,
    @SerialName("keep_remote") KEEP_REMOTE,
    @SerialName("keep_both") KEEP_BOTH,
}

// ---------------------------------------------------------------------------
// Schedule / DuePrompts
// ---------------------------------------------------------------------------

/**
 * Typed representation of prompt IDs (mobilecore/schedule.go).
 * The [wire] value matches the `id` field in [DuePromptDto].
 * Use [fromWire] to convert; unknown values throw — fail loud, no silent default.
 *
 * Wire values: 0=WEEK_REVIEW, 1=WEEKLY_PLAN, 2=MORNING, 3=EVENING, 4=WEEKLY_SUMMARY.
 * All five are handled by CheckinCoordinator and routed to their screens.
 */
enum class PromptId(val wire: Int) {
    WEEK_REVIEW(0),
    WEEKLY_PLAN(1),
    MORNING(2),
    EVENING(3),
    WEEKLY_SUMMARY(4);

    companion object {
        /**
         * Converts a wire integer to [PromptId].
         * Throws [SerializationException] on unknown values — advisory callers
         * catch and log; never let an unknown id crash the coordinator.
         */
        fun fromWire(n: Int): PromptId =
            entries.firstOrNull { it.wire == n }
                ?: throw SerializationException("Unknown PromptId wire value: $n (expected 0-4)")
    }
}

/**
 * 0=week review 1=weekly plan 2=morning 3=evening 4=weekly summary
 * (mobilecore/schedule.go).
 */
@Serializable
data class DuePromptDto(val id: Int, val name: String)

@Serializable
data class DuePromptsDto(val due: List<DuePromptDto> = emptyList())
