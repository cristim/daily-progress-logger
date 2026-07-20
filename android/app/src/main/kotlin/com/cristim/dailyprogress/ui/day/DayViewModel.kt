package com.cristim.dailyprogress.ui.day

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.TaskState
import com.cristim.dailyprogress.model.TreeDto
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

sealed interface DayUiState {
    data object Loading : DayUiState
    data class Content(val tree: TreeDto, val date: LocalDate) : DayUiState
    data class Error(val error: CoreError, val date: LocalDate) : DayUiState
}

/** One-shot snackbar message emitted after mutations. */
data class SnackbarEvent(val message: String)

// ---------------------------------------------------------------------------
// ViewModel
// ---------------------------------------------------------------------------

class DayViewModel(
    private val repository: CoreRepository,
    initialDate: LocalDate,
    private val dataVersion: MutableStateFlow<Int>,
) : ViewModel() {

    private val _uiState = MutableStateFlow<DayUiState>(DayUiState.Loading)
    val uiState: StateFlow<DayUiState> = _uiState.asStateFlow()

    private val _snackbar = Channel<SnackbarEvent>(Channel.BUFFERED)
    val snackbarEvents = _snackbar.receiveAsFlow()

    /** The currently displayed date. */
    private var currentDate: LocalDate = initialDate

    init {
        refresh()
        // Refresh when another screen mutates shared data (e.g. check-in apply).
        // Drop the initial emission (init already loaded).
        viewModelScope.launch {
            dataVersion.drop(1).collect { refresh() }
        }
    }

    // -----------------------------------------------------------------------
    // Navigation
    // -----------------------------------------------------------------------

    fun prevDay() {
        currentDate = currentDate.minusDays(1)
        refresh()
    }

    fun nextDay() {
        currentDate = currentDate.plusDays(1)
        refresh()
    }

    fun today() {
        currentDate = LocalDate.now()
        refresh()
    }

    fun setDate(date: LocalDate) {
        currentDate = date
        refresh()
    }

    // -----------------------------------------------------------------------
    // Data loading
    // -----------------------------------------------------------------------

    fun refresh() {
        viewModelScope.launch {
            val date = currentDate
            _uiState.value = DayUiState.Loading
            runCatching { repository.tree(date.toString()) }
                .onSuccess { _uiState.value = DayUiState.Content(it, date) }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _uiState.value = DayUiState.Error(err, date)
                }
        }
    }

    // -----------------------------------------------------------------------
    // Task mutations — each calls Core, then refreshes.
    // CAS_MISMATCH triggers a refresh + snackbar; no automatic retry.
    // -----------------------------------------------------------------------

    fun toggleTaskDone(index: Long, expectedText: String, date: String, currentState: TaskState) {
        val newState = if (currentState == TaskState.DONE) TaskState.TODO else TaskState.DONE
        mutate {
            repository.setTaskState(date, index, expectedText, newState)
        }
    }

    fun setTaskPostponed(index: Long, expectedText: String, date: String) {
        mutate { repository.setTaskState(date, index, expectedText, TaskState.POSTPONED) }
    }

    fun deleteTask(index: Long, expectedText: String, date: String) {
        mutate { repository.deleteTask(date, index, expectedText) }
    }

    fun editTaskText(index: Long, expectedText: String, date: String, newText: String) {
        if (newText.isBlank()) return
        mutate { repository.editTaskText(date, index, expectedText, newText) }
    }

    fun postponeToNextDay(index: Long, expectedText: String, date: String) {
        mutate { repository.postponeToNextDay(date, index, expectedText) }
    }

    fun postponeToNextWeek(index: Long, expectedText: String, date: String) {
        mutate { repository.postponeToNextWeek(date, index, expectedText) }
    }

    fun moveToBacklog(index: Long, expectedText: String, date: String) {
        mutate { repository.moveTaskToBacklog(date, index, expectedText) }
    }

    fun addTask(text: String, projectId: String = "") {
        if (text.isBlank()) return
        mutate { repository.addTask(currentDate.toString(), text, projectId) }
    }

    // -----------------------------------------------------------------------
    // Internal helper
    // -----------------------------------------------------------------------

    /**
     * Runs a Core mutation on [viewModelScope], then refreshes the tree.
     * On success: bumps [dataVersion] so sibling screens (BacklogScreen,
     * WeekScreen) refresh to reflect the change (e.g. moveToBacklog updates
     * the backlog; task state changes affect the weekly summary done-by-day).
     * On [CoreError.CasMismatch] the tree is refreshed and the user gets a
     * snackbar ("List changed - refreshed, try again") instead of an error.
     */
    private fun mutate(block: suspend () -> Unit) {
        viewModelScope.launch {
            runCatching { block() }
                .onSuccess {
                    dataVersion.value++
                    // Do NOT call refresh() here: the init collector
                    // (dataVersion.drop(1).collect { refresh() }) fires on the
                    // bump above and handles the reload -- matching WeekViewModel.
                    // Calling refresh() here too causes a double reload (flicker +
                    // double treeJSON fetch). The CAS branch below still calls
                    // refresh() directly because it does not bump dataVersion.
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    when (err) {
                        is CoreError.CasMismatch -> {
                            refresh()
                            _snackbar.trySend(SnackbarEvent("List changed, refreshed. Please try again."))
                        }
                        is CoreError.SyncAuth -> {
                            _snackbar.trySend(SnackbarEvent("Sign in to Google Drive again to re-enable sync."))
                        }
                        else -> {
                            _snackbar.trySend(SnackbarEvent("Error: ${err.message}"))
                            refresh()
                        }
                    }
                }
        }
    }

    // -----------------------------------------------------------------------
    // Factory
    // -----------------------------------------------------------------------

    class Factory(
        private val repository: CoreRepository,
        private val initialDate: LocalDate,
        private val dataVersion: MutableStateFlow<Int>,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            DayViewModel(repository, initialDate, dataVersion) as T
    }
}
