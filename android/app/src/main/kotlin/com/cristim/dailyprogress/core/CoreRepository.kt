package com.cristim.dailyprogress.core

import com.cristim.dailyprogress.model.BacklogDto
import com.cristim.dailyprogress.model.ConflictChoice
import com.cristim.dailyprogress.model.ConflictDto
import com.cristim.dailyprogress.model.DuePromptsDto
import com.cristim.dailyprogress.model.EveningDecisionsDto
import com.cristim.dailyprogress.model.MorningCandidateDto
import com.cristim.dailyprogress.model.MorningDecisionsDto
import com.cristim.dailyprogress.model.PendingWeekDto
import com.cristim.dailyprogress.model.ProjectDto
import com.cristim.dailyprogress.model.RecurringTemplateDto
import com.cristim.dailyprogress.model.RecycleEntryDto
import com.cristim.dailyprogress.model.ReviewDecisionsDto
import com.cristim.dailyprogress.model.SyncResultDto
import com.cristim.dailyprogress.model.TaskState
import com.cristim.dailyprogress.model.TreeDto
import com.cristim.dailyprogress.model.WeeklyGoalDto
import com.cristim.dailyprogress.model.WeeklyPlanDto
import com.cristim.dailyprogress.model.WeeklySummaryDto
import com.cristim.dailyprogress.model.WeekReviewCandidatesDto
import com.cristim.dailyprogress.model.wireString
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

/**
 * Typed interface to the Go Core, running all calls on the single-threaded
 * [CoreClient.dispatcher].
 *
 * Design rules:
 * - Every method uses [call] which wraps the Core invocation in
 *   withContext(client.dispatcher) and maps thrown exceptions to [CoreError].
 * - Callers (ViewModels) must not call Core methods directly.
 * - No optimistic UI: callers re-fetch the affected read endpoint after each
 *   mutation and replace state entirely.
 */
class CoreRepository(private val client: CoreClient) : WeekReviewOps, BacklogOps {

    /** JSON codec: ignoreUnknownKeys so additive core changes never crash. */
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    /**
     * Runs [block] on the Core dispatcher, mapping any thrown exception to a
     * typed [CoreError]. All public methods route through here.
     */
    private suspend fun <T> call(block: (mobilecore.Core) -> T): T =
        withContext(client.dispatcher) {
            try {
                block(client.openCore())
            } catch (e: CoreError) {
                throw e // already typed
            } catch (e: SerializationException) {
                // JSON decode failure: Core returned valid bytes but the DTO shape
                // has drifted. Classify distinctly from Core-side errors.
                throw CoreError.ContractViolation("CONTRACT_VIOLATION: ${e.message.orEmpty()}")
            } catch (e: Exception) {
                throw CoreError.parse(e)
            }
        }

    // -----------------------------------------------------------------------
    // Tree
    // -----------------------------------------------------------------------

    /** Fetches the day's project tree. Also materialises recurring tasks due that day. */
    suspend fun tree(date: String): TreeDto = call { core ->
        json.decodeFromString(core.treeJSON(date))
    }

    // -----------------------------------------------------------------------
    // Task actions — all take (date, index, expectedText) CAS triple.
    // -----------------------------------------------------------------------

    suspend fun setTaskState(date: String, index: Long, expectedText: String, state: TaskState) =
        call { core -> core.setTaskState(date, index, expectedText, state.wireString()) }

    suspend fun deleteTask(date: String, index: Long, expectedText: String) =
        call { core -> core.deleteTask(date, index, expectedText) }

    suspend fun editTaskText(date: String, index: Long, expectedText: String, newText: String) =
        call { core -> core.editTaskText(date, index, expectedText, newText) }

    suspend fun postponeToNextDay(date: String, index: Long, expectedText: String) =
        call { core -> core.postponeToNextDay(date, index, expectedText) }

    suspend fun postponeToNextWeek(date: String, index: Long, expectedText: String) =
        call { core -> core.postponeToNextWeek(date, index, expectedText) }

    suspend fun moveTaskToBacklog(date: String, index: Long, expectedText: String) =
        call { core -> core.moveTaskToBacklog(date, index, expectedText) }

    suspend fun addTask(date: String, text: String, projectId: String = "") =
        call { core -> core.addTask(date, text, projectId) }

    suspend fun addSubtask(date: String, parentIndex: Long, expectedParentText: String, text: String) =
        call { core -> core.addSubtask(date, parentIndex, expectedParentText, text) }

    suspend fun makeSubtask(
        date: String,
        childIndex: Long,
        expectedChildText: String,
        parentIndex: Long,
        expectedParentText: String,
    ) = call { core ->
        core.makeSubtask(date, childIndex, expectedChildText, parentIndex, expectedParentText)
    }

    suspend fun moveTaskToProject(date: String, index: Long, expectedText: String, projectId: String) =
        call { core -> core.moveTaskToProject(date, index, expectedText, projectId) }

    suspend fun unassignTaskProject(date: String, index: Long, expectedText: String) =
        call { core -> core.unassignTaskProject(date, index, expectedText) }

    // -----------------------------------------------------------------------
    // Backlog
    // -----------------------------------------------------------------------

    override suspend fun backlog(): BacklogDto = call { core ->
        json.decodeFromString(core.backlogJSON())
    }

    override suspend fun adoptFromBacklog(date: String, text: String) =
        call { core -> core.adoptFromBacklog(date, text) }

    override suspend fun moveBacklogItem(text: String, toNextWeek: Boolean) =
        call { core -> core.moveBacklogItem(text, toNextWeek) }

    // -----------------------------------------------------------------------
    // Check-ins
    // -----------------------------------------------------------------------

    suspend fun morningCandidates(date: String): List<MorningCandidateDto> = call { core ->
        json.decodeFromString<List<MorningCandidateDto>>(core.morningCandidatesJSON(date))
    }

    suspend fun applyMorning(date: String, decisions: MorningDecisionsDto) =
        call { core -> core.applyMorning(date, json.encodeToString(decisions)) }

    suspend fun applyEvening(date: String, decisions: EveningDecisionsDto) =
        call { core -> core.applyEvening(date, json.encodeToString(decisions)) }

    // -----------------------------------------------------------------------
    // Weekly
    // -----------------------------------------------------------------------

    suspend fun weeklyPlan(date: String): WeeklyPlanDto = call { core ->
        json.decodeFromString(core.weeklyPlanJSON(date))
    }

    suspend fun setWeeklyPlan(date: String, goals: List<WeeklyGoalDto>) = call { core ->
        core.setWeeklyPlan(date, json.encodeToString(goals))
    }

    /** date: any date in the week to check for an unreviewed week. */
    override suspend fun unreviewedWeek(date: String): PendingWeekDto = call { core ->
        json.decodeFromString(core.unreviewedWeekJSON(date))
    }

    override suspend fun weekReviewCandidates(date: String): WeekReviewCandidatesDto = call { core ->
        json.decodeFromString(core.weekReviewCandidatesJSON(date))
    }

    override suspend fun applyWeekReview(date: String, decisions: ReviewDecisionsDto) =
        call { core -> core.applyWeekReview(date, json.encodeToString(decisions)) }

    suspend fun weeklySummary(date: String): WeeklySummaryDto = call { core ->
        json.decodeFromString(core.weeklySummaryJSON(date))
    }

    /** date: any date in the week to check for a pending summary. */
    suspend fun weeklySummaryPending(date: String): PendingWeekDto = call { core ->
        json.decodeFromString(core.weeklySummaryPendingJSON(date))
    }

    suspend fun markWeekSummarized(date: String) =
        call { core -> core.markWeekSummarized(date) }

    // -----------------------------------------------------------------------
    // Projects
    // -----------------------------------------------------------------------

    suspend fun projects(): List<ProjectDto> = call { core ->
        json.decodeFromString(core.projectsJSON())
    }

    suspend fun addProject(name: String): String = call { core ->
        core.addProject(name)
    }

    suspend fun renameProject(id: String, newName: String) =
        call { core -> core.renameProject(id, newName) }

    suspend fun closeProject(id: String) =
        call { core -> core.closeProject(id) }

    suspend fun reopenProject(id: String) =
        call { core -> core.reopenProject(id) }

    // -----------------------------------------------------------------------
    // Recurring templates
    // -----------------------------------------------------------------------

    suspend fun recurring(): List<RecurringTemplateDto> = call { core ->
        json.decodeFromString(core.recurringJSON())
    }

    suspend fun addRecurring(text: String) =
        call { core -> core.addRecurring(text) }

    suspend fun removeRecurring(rawText: String) =
        call { core -> core.removeRecurring(rawText) }

    // -----------------------------------------------------------------------
    // Recycle bin
    // -----------------------------------------------------------------------

    suspend fun recycle(): List<RecycleEntryDto> = call { core ->
        json.decodeFromString(core.recycleJSON())
    }

    suspend fun restoreTask(date: String, displayText: String) =
        call { core -> core.restoreTask(date, displayText) }

    suspend fun purgeRecycled(date: String, displayText: String) =
        call { core -> core.purgeRecycled(date, displayText) }

    // -----------------------------------------------------------------------
    // Schedule / due prompts
    // -----------------------------------------------------------------------

    suspend fun duePrompts(nowRfc3339: String): DuePromptsDto = call { core ->
        json.decodeFromString(core.duePromptsJSON(nowRfc3339))
    }

    // -----------------------------------------------------------------------
    // Sync
    // -----------------------------------------------------------------------

    suspend fun syncNow(tokenJson: String): SyncResultDto = call { core ->
        json.decodeFromString(core.syncNow(tokenJson))
    }

    suspend fun conflicts(tokenJson: String): List<ConflictDto> = call { core ->
        json.decodeFromString(core.conflictsJSON(tokenJson))
    }

    suspend fun resolveConflict(tokenJson: String, path: String, choice: ConflictChoice) =
        call { core -> core.resolveConflict(tokenJson, path, choice.name.lowercase()) }
}
