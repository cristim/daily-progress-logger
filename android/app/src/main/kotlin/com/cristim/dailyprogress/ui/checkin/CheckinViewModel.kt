package com.cristim.dailyprogress.ui.checkin

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.EveningAction
import com.cristim.dailyprogress.model.EveningDecisionDto
import com.cristim.dailyprogress.model.EveningDecisionsDto
import com.cristim.dailyprogress.model.MorningCandidateDto
import com.cristim.dailyprogress.model.MorningDecisionsDto
import com.cristim.dailyprogress.model.TaskState
import com.cristim.dailyprogress.model.WeeklyGoalDto
import com.cristim.dailyprogress.ui.day.SnackbarEvent
import com.cristim.dailyprogress.util.flattenPreOrder
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import java.time.LocalDate

// ---------------------------------------------------------------------------
// UI state
// ---------------------------------------------------------------------------

/**
 * One flattened task row in the evening check-in screen.
 * [depth] mirrors the tree depth for indentation.
 * [action] is mutable per-row — starts seeded from [eveningActionForState].
 */
data class EveningItem(
    val text: String,
    val depth: Int,
    val action: EveningAction,
)

/**
 * How the check-in was launched: from the scheduled prompt coordinator
 * (shows Snooze/Skip-Today buttons) or manually from the Day top-bar menu
 * (shows Close with no bookkeeping).
 */
enum class CheckinPresentation { SCHEDULED, MANUAL }

sealed interface CheckinUiState {
    data object Loading : CheckinUiState

    data class Morning(
        val candidates: List<MorningCandidateDto>,
        /** Parallel list of adopted flags — index matches [candidates]. */
        val adopted: List<Boolean>,
        val goals: List<WeeklyGoalDto>,
        /** Count of plan items already in today's tree (summary line). */
        val plannedCount: Int,
    ) : CheckinUiState

    data class Evening(val items: List<EveningItem>) : CheckinUiState

    data class Error(val error: CoreError) : CheckinUiState
}

// ---------------------------------------------------------------------------
// ViewModel
// ---------------------------------------------------------------------------

class CheckinViewModel(
    private val repository: CoreRepository,
    private val dataVersion: MutableStateFlow<Int>,
) : ViewModel() {

    private val _uiState = MutableStateFlow<CheckinUiState>(CheckinUiState.Loading)
    val uiState: StateFlow<CheckinUiState> = _uiState.asStateFlow()

    /** One-shot snackbar messages (apply errors — sheet stays open per rule 4). */
    private val _snackbar = Channel<SnackbarEvent>(Channel.BUFFERED)
    val snackbarEvents = _snackbar.receiveAsFlow()

    /** One-shot dismiss signal: emitted on successful apply. */
    private val _done = Channel<Unit>(Channel.BUFFERED)
    val doneEvents = _done.receiveAsFlow()

    // -----------------------------------------------------------------------
    // Morning
    // -----------------------------------------------------------------------

    /**
     * Loads morning check-in state in parallel:
     * candidates + weekly plan goals + today's already-planned item count.
     *
     * Guard: if the ViewModel already holds Morning state (e.g. after a
     * configuration change where the VM survives), skip the reload so
     * adopted-candidate toggles and in-progress text are not reset.
     */
    fun loadMorning(date: LocalDate) {
        if (_uiState.value is CheckinUiState.Morning) return
        viewModelScope.launch {
            _uiState.value = CheckinUiState.Loading
            runCatching {
                val dateStr = date.toString()
                val candidatesDeferred = async { repository.morningCandidates(dateStr) }
                val planDeferred = async { repository.weeklyPlan(dateStr) }
                val treeDeferred = async { repository.tree(dateStr) }

                val candidates = candidatesDeferred.await()
                val plan = planDeferred.await()
                val tree = treeDeferred.await()

                // Count plan items already in today's tree (projects + unfiled, flattened).
                val plannedCount =
                    tree.projects.sumOf { p -> p.tasks.sumOf { t -> t.flattenPreOrder().size } } +
                        tree.unfiled.sumOf { t -> t.flattenPreOrder().size }

                // Default-check rules (mobilecore/MorningCandidates semantics):
                //   fromBacklog == false  = same-week carry-over  → checked by default
                //   fromBacklog == true   = backlog candidate      → unchecked by default
                val adopted = candidates.map { c -> !c.fromBacklog }

                CheckinUiState.Morning(
                    candidates = candidates,
                    adopted = adopted,
                    goals = plan.goals,
                    plannedCount = plannedCount,
                )
            }
                .onSuccess { _uiState.value = it }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _uiState.value = CheckinUiState.Error(err)
                }
        }
    }

    /** Toggles the adopted flag for the candidate at [index]. */
    fun toggleCandidate(index: Int) {
        val morning = _uiState.value as? CheckinUiState.Morning ?: return
        if (index !in morning.adopted.indices) return
        val updated = morning.adopted.toMutableList().also { it[index] = !it[index] }
        _uiState.value = morning.copy(adopted = updated)
    }

    /**
     * Applies the morning check-in. [newItemsText] is the free-text editor
     * content (one task per line). Adopted candidates come from current state.
     * Emits [doneEvents] on success; snackbar on error (sheet stays open).
     */
    fun applyMorning(date: LocalDate, newItemsText: String) {
        val morning = _uiState.value as? CheckinUiState.Morning ?: return
        viewModelScope.launch {
            val newItems = newItemsText.lines().map { it.trim() }.filter { it.isNotEmpty() }
            val adopted = morning.candidates.filterIndexed { i, _ ->
                i < morning.adopted.size && morning.adopted[i]
            }
            val payload = MorningDecisionsDto(newItems = newItems, adopted = adopted)
            runCatching { repository.applyMorning(date.toString(), payload) }
                .onSuccess {
                    dataVersion.value++
                    _done.trySend(Unit)
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _snackbar.trySend(SnackbarEvent("Error: ${err.message}"))
                }
        }
    }

    // -----------------------------------------------------------------------
    // Evening
    // -----------------------------------------------------------------------

    /**
     * Loads evening check-in state by fetching today's tree and flattening it
     * into a list of [EveningItem]s with initial actions seeded from task state.
     * Order: projects' tasks (pre-order) then unfiled — matches Qt's plan-file
     * ordering well enough that ApplyEvening's text-match semantics are safe.
     *
     * Guard: skip reload when the ViewModel already holds Evening state so
     * per-row action selections survive configuration changes.
     */
    fun loadEvening(date: LocalDate) {
        if (_uiState.value is CheckinUiState.Evening) return
        viewModelScope.launch {
            _uiState.value = CheckinUiState.Loading
            runCatching { repository.tree(date.toString()) }
                .onSuccess { tree ->
                    val items = buildList {
                        tree.projects.forEach { project ->
                            project.tasks.flatMap { t -> t.flattenPreOrder() }.forEach { task ->
                                add(
                                    EveningItem(
                                        text = task.text,
                                        depth = task.depth,
                                        action = eveningActionForState(task.state),
                                    ),
                                )
                            }
                        }
                        tree.unfiled.flatMap { t -> t.flattenPreOrder() }.forEach { task ->
                            add(
                                EveningItem(
                                    text = task.text,
                                    depth = task.depth,
                                    action = eveningActionForState(task.state),
                                ),
                            )
                        }
                    }
                    _uiState.value = CheckinUiState.Evening(items)
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _uiState.value = CheckinUiState.Error(err)
                }
        }
    }

    /** Updates the action for the evening item at [index]. */
    fun setEveningAction(index: Int, action: EveningAction) {
        val evening = _uiState.value as? CheckinUiState.Evening ?: return
        if (index !in evening.items.indices) return
        val updated = evening.items.toMutableList().also { it[index] = it[index].copy(action = action) }
        _uiState.value = evening.copy(items = updated)
    }

    /**
     * Applies the evening check-in. [extraText] is the free-text editor for
     * bonus accomplished items (one per line). Emits [doneEvents] on success.
     */
    fun applyEvening(date: LocalDate, extraText: String) {
        val evening = _uiState.value as? CheckinUiState.Evening ?: return
        viewModelScope.launch {
            val decisions = evening.items.map { item ->
                EveningDecisionDto(text = item.text, action = item.action.wire)
            }
            val extraDone = extraText.lines().map { it.trim() }.filter { it.isNotEmpty() }
            val payload = EveningDecisionsDto(decisions = decisions, extraDone = extraDone)
            runCatching { repository.applyEvening(date.toString(), payload) }
                .onSuccess {
                    dataVersion.value++
                    _done.trySend(Unit)
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _snackbar.trySend(SnackbarEvent("Error: ${err.message}"))
                }
        }
    }

    // -----------------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------------

    /**
     * Maps a task's current state to its initial evening action selection.
     * Mirrors Qt's EveningActionForState (statebuttons.go):
     *   done      → Done
     *   postponed → NextWeek (the postponed state is "pushed out" to next week)
     *   todo      → Todo
     */
    private fun eveningActionForState(state: TaskState): EveningAction = when (state) {
        TaskState.DONE -> EveningAction.DONE
        TaskState.POSTPONED -> EveningAction.NEXT_WEEK
        TaskState.TODO -> EveningAction.TODO
    }

    // -----------------------------------------------------------------------
    // Factory
    // -----------------------------------------------------------------------

    class Factory(
        private val repository: CoreRepository,
        private val dataVersion: MutableStateFlow<Int>,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            CheckinViewModel(repository, dataVersion) as T
    }
}
