package com.cristim.dailyprogress.model

import kotlinx.serialization.SerialName
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

/** 0=todo 1=done 2=next_day 3=next_week 4=backlog (mobilecore/checkin.go). */
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
 * 0=week review 1=weekly plan 2=morning 3=evening 4=weekly summary
 * (mobilecore/schedule.go).
 */
@Serializable
data class DuePromptDto(val id: Int, val name: String)

@Serializable
data class DuePromptsDto(val due: List<DuePromptDto> = emptyList())
