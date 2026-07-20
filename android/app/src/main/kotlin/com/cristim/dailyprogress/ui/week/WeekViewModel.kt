package com.cristim.dailyprogress.ui.week

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.WeeklyGoalDto
import com.cristim.dailyprogress.model.WeeklyPlanDto
import com.cristim.dailyprogress.model.WeeklySummaryDto
import com.cristim.dailyprogress.ui.day.SnackbarEvent
import com.cristim.dailyprogress.util.isoWeekToMonday
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import java.time.LocalDate

// ---------------------------------------------------------------------------
// UI state
// ---------------------------------------------------------------------------

sealed interface WeekUiState {
    data object Loading : WeekUiState

    data class Content(
        val referenceDate: LocalDate,
        val plan: WeeklyPlanDto,
        val summary: WeeklySummaryDto,
        /** True when at least one prior week awaits review (drives badge in WeekScreen). */
        val reviewPending: Boolean,
    ) : WeekUiState

    data class Error(val error: CoreError) : WeekUiState
}

// ---------------------------------------------------------------------------
// ViewModel
// ---------------------------------------------------------------------------

/**
 * ViewModel for the Week tab. Owns the weekly plan and summary state for the
 * currently-displayed reference week. Plan mutations do a full-array replace
 * per the contract (SetWeeklyPlan never receives partial arrays).
 *
 * Cross-screen data version: bumped after every mutation so sibling screens
 * (DayScreen etc.) pick up the changed data without polling.
 */
class WeekViewModel(
    private val repository: CoreRepository,
    private val dataVersion: MutableStateFlow<Int>,
) : ViewModel() {

    private val _uiState = MutableStateFlow<WeekUiState>(WeekUiState.Loading)
    val uiState: StateFlow<WeekUiState> = _uiState.asStateFlow()

    /** True while a plan save is in flight; disables the Add button. */
    private val _submitting = MutableStateFlow(false)
    val submitting: StateFlow<Boolean> = _submitting.asStateFlow()

    /** One-shot snackbar messages for mutation errors. */
    private val _snackbar = Channel<SnackbarEvent>(Channel.BUFFERED)
    val snackbarEvents = _snackbar.receiveAsFlow()

    private var referenceDate: LocalDate = LocalDate.now()

    init {
        refresh()
        // Refresh when any other screen bumps dataVersion (check-in apply etc.).
        viewModelScope.launch {
            dataVersion.drop(1).collect { refresh() }
        }
    }

    // -----------------------------------------------------------------------
    // Week navigation
    // -----------------------------------------------------------------------

    fun prevWeek() {
        referenceDate = referenceDate.minusWeeks(1)
        refresh()
    }

    fun nextWeek() {
        referenceDate = referenceDate.plusWeeks(1)
        refresh()
    }

    fun thisWeek() {
        referenceDate = LocalDate.now()
        refresh()
    }

    // -----------------------------------------------------------------------
    // Data loading
    // -----------------------------------------------------------------------

    fun refresh() {
        viewModelScope.launch {
            val date = referenceDate
            _uiState.value = WeekUiState.Loading
            runCatching {
                // Load plan + summary for the reference week in parallel.
                // Review-pending check uses today (not the viewed week) to detect any
                // prior unreviewed week, mirroring Qt's oldest-first loop.
                val planDeferred = async { repository.weeklyPlan(date.toString()) }
                val summaryDeferred = async { repository.weeklySummary(date.toString()) }
                val reviewPendingDeferred =
                    async { repository.unreviewedWeek(LocalDate.now().toString()) }
                Triple(
                    planDeferred.await(),
                    summaryDeferred.await(),
                    reviewPendingDeferred.await(),
                )
            }
                .onSuccess { (plan, summary, reviewPending) ->
                    _uiState.value = WeekUiState.Content(
                        referenceDate = date,
                        plan = plan,
                        summary = summary,
                        reviewPending = reviewPending.pending,
                    )
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _uiState.value = WeekUiState.Error(err)
                }
        }
    }

    // -----------------------------------------------------------------------
    // Plan mutations — full-array replace per SetWeeklyPlan contract.
    // "Plan save conflicts are last-write-wins by design." — no CAS here.
    // -----------------------------------------------------------------------

    /**
     * Toggles the done state of goal at [index], then persists the full
     * rebuilt goals array. Index-based mutation on the snapshot currently shown.
     */
    fun setGoalDone(index: Int, done: Boolean) {
        val content = _uiState.value as? WeekUiState.Content ?: return
        val goals = content.plan.goals.toMutableList()
        if (index !in goals.indices) return
        goals[index] = goals[index].copy(done = done)
        savePlan(content.referenceDate, goals)
    }

    /**
     * Appends new goals from [text] (one per line, trimmed, non-empty) to the
     * existing goals list, then persists. Empty-only input is a no-op.
     */
    fun addGoals(text: String) {
        val content = _uiState.value as? WeekUiState.Content ?: return
        val newGoals = text.lines()
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .map { WeeklyGoalDto(text = it, done = false) }
        if (newGoals.isEmpty()) return
        savePlan(content.referenceDate, content.plan.goals + newGoals)
    }

    private fun savePlan(date: LocalDate, goals: List<WeeklyGoalDto>) {
        viewModelScope.launch {
            _submitting.value = true
            runCatching { repository.setWeeklyPlan(date.toString(), goals) }
                .onSuccess { dataVersion.value++ }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _snackbar.trySend(SnackbarEvent("Error saving plan: ${err.message}"))
                    refresh()
                }
            _submitting.value = false
        }
    }

    // -----------------------------------------------------------------------
    // Loop drivers — used by the RootScaffold coordinator to feed the
    // oldest-first scheduled review/summary loops.
    // -----------------------------------------------------------------------

    /**
     * Returns the Monday [LocalDate] of the oldest unreviewed week, or null
     * when everything is reviewed. Non-fatal: returns null on error and logs.
     */
    suspend fun nextUnreviewedWeek(): LocalDate? = runCatching {
        val p = repository.unreviewedWeek(LocalDate.now().toString())
        if (p.pending && p.week.isNotEmpty()) isoWeekToMonday(p.week) else null
    }.getOrNull()

    /**
     * Returns the Monday [LocalDate] of the oldest week with a pending
     * (unsummarized) summary, or null when none. Non-fatal.
     */
    suspend fun nextPendingSummaryWeek(): LocalDate? = runCatching {
        val p = repository.weeklySummaryPending(LocalDate.now().toString())
        if (p.pending && p.week.isNotEmpty()) isoWeekToMonday(p.week) else null
    }.getOrNull()

    // -----------------------------------------------------------------------
    // Factory
    // -----------------------------------------------------------------------

    class Factory(
        private val repository: CoreRepository,
        private val dataVersion: MutableStateFlow<Int>,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            WeekViewModel(repository, dataVersion) as T
    }
}
