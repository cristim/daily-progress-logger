package com.cristim.dailyprogress.ui.recurring

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.RecurringOps
import com.cristim.dailyprogress.model.RecurringTemplateDto
import com.cristim.dailyprogress.ui.day.SnackbarEvent
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

sealed interface RecurringUiState {
    data object Loading : RecurringUiState
    data class Content(val templates: List<RecurringTemplateDto>) : RecurringUiState
    data class Error(val error: CoreError) : RecurringUiState
}

// ---------------------------------------------------------------------------
// ViewModel
// ---------------------------------------------------------------------------

/**
 * ViewModel for the Recurring Templates screen (More > Recurring).
 *
 * Design rules (mirrors shared conventions):
 * - No optimistic UI: every mutation re-fetches [RecurringOps.tree] and
 *   replaces state wholesale.
 * - Reads `TreeJSON(today).recurring` rather than RecurringJSON, per the
 *   plan's design decision: templates are global (date-independent), but
 *   only the tree shape carries the `describe` schedule caption used for
 *   display. `raw` (used for delete) is present in both shapes.
 * - Add: [CoreError.BadInput] (missing recurrence tag, or a recurrence tag
 *   with no description) surfaces as an inline field error in the add
 *   dialog, not a snackbar — the dialog stays open so the user can fix the
 *   text (fail-loud, no silent drop). The field error text is the core's own
 *   detail (stripped of the "BAD_INPUT: " prefix), since only the core knows
 *   which of the two causes fired. Other errors surface via snackbar; the
 *   dialog also stays open so typed text is not lost.
 * - Remove: always confirmed by the caller before invoking; NOT_FOUND races
 *   (template already removed) are treated like any other error since core
 *   already refreshes state either way.
 * - Refresh failure while content is on screen: snackbar, keep content.
 *   Refresh failure with no content: Error state with Retry.
 */
class RecurringViewModel(
    private val repository: RecurringOps,
    private val dataVersion: MutableStateFlow<Int>,
) : ViewModel() {

    private val _uiState = MutableStateFlow<RecurringUiState>(RecurringUiState.Loading)
    val uiState: StateFlow<RecurringUiState> = _uiState.asStateFlow()

    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    private val _snackbar = Channel<SnackbarEvent>(Channel.BUFFERED)
    val snackbarEvents = _snackbar.receiveAsFlow()

    /** Inline field error for the add dialog (BAD_INPUT). Null clears it. */
    private val _addFieldError = MutableStateFlow<String?>(null)
    val addFieldError: StateFlow<String?> = _addFieldError.asStateFlow()

    /** One-shot signal emitted on a successful add, so the screen closes the dialog. */
    private val _addDone = Channel<Unit>(Channel.BUFFERED)
    val addDoneEvents = _addDone.receiveAsFlow()

    /** True while an add() call is in flight; guards against a double-tap firing add() twice. */
    private val _submitting = MutableStateFlow(false)
    val submitting: StateFlow<Boolean> = _submitting.asStateFlow()

    init {
        refresh()
        // Refresh when another screen bumps the data version, e.g. a
        // materialized occurrence changing today's tree.
        viewModelScope.launch {
            dataVersion.drop(1).collect { refresh() }
        }
    }

    // -----------------------------------------------------------------------
    // Data loading
    // -----------------------------------------------------------------------

    fun refresh() {
        viewModelScope.launch {
            _isRefreshing.value = true
            if (_uiState.value !is RecurringUiState.Content) {
                _uiState.value = RecurringUiState.Loading
            }
            // Templates are global, not tied to the viewed date — always use today.
            runCatching { repository.tree(LocalDate.now().toString()).recurring }
                .onSuccess { _uiState.value = RecurringUiState.Content(it) }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    if (_uiState.value is RecurringUiState.Content) {
                        _snackbar.trySend(
                            SnackbarEvent("Could not refresh recurring templates: ${err.message}"),
                        )
                    } else {
                        _uiState.value = RecurringUiState.Error(err)
                    }
                }
            _isRefreshing.value = false
        }
    }

    // -----------------------------------------------------------------------
    // Mutations
    // -----------------------------------------------------------------------

    /** Clears the add-dialog field error, e.g. when the dialog is (re)opened. */
    fun clearAddFieldError() {
        _addFieldError.value = null
    }

    /**
     * Adds [text] as a recurring template. Text must carry a recurrence tag
     * (@daily, @weekly @mon, @monthly @1, optionally @HH:MM and #project) or
     * the core returns BAD_INPUT.
     */
    fun add(text: String) {
        if (_submitting.value) return
        viewModelScope.launch {
            _submitting.value = true
            runCatching { repository.addRecurring(text) }
                .onSuccess {
                    _addFieldError.value = null
                    _addDone.trySend(Unit)
                    // Do NOT call refresh() here: the init collector
                    // (dataVersion.drop(1).collect { refresh() }) fires on the
                    // bump below and reloads — calling refresh() here too would
                    // double-fetch (matches DayViewModel.mutate's fix).
                    dataVersion.value++
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    when (err) {
                        // Surface the core's actual error detail (stripped of the
                        // "BAD_INPUT: " code prefix) rather than a hardcoded guess:
                        // BAD_INPUT covers both "no recurrence tag" and "no
                        // description" causes, and only the core knows which one
                        // fired.
                        is CoreError.BadInput ->
                            _addFieldError.value =
                                err.message.orEmpty().removePrefix("BAD_INPUT: ")
                        else ->
                            _snackbar.trySend(SnackbarEvent("Error: ${err.message}"))
                    }
                }
            _submitting.value = false
        }
    }

    /** Removes the template whose raw stored line is [raw]. Caller confirms first. */
    fun remove(raw: String) {
        viewModelScope.launch {
            runCatching { repository.removeRecurring(raw) }
                .onSuccess {
                    // Bump-only: the init collector handles the reload (see add()).
                    dataVersion.value++
                }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _snackbar.trySend(SnackbarEvent("Error: ${err.message}"))
                    // No bump on failure, so refresh explicitly here (matches the
                    // CAS-mismatch branch pattern elsewhere).
                    refresh()
                }
        }
    }

    // -----------------------------------------------------------------------
    // Factory
    // -----------------------------------------------------------------------

    class Factory(
        private val repository: RecurringOps,
        private val dataVersion: MutableStateFlow<Int>,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            RecurringViewModel(repository, dataVersion) as T
    }
}
