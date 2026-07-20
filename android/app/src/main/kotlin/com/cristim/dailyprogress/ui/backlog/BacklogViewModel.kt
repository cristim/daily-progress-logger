package com.cristim.dailyprogress.ui.backlog

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.cristim.dailyprogress.core.BacklogOps
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.model.BacklogDto
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

sealed interface BacklogUiState {
    data object Loading : BacklogUiState
    data class Content(val backlog: BacklogDto) : BacklogUiState
    data class Error(val error: CoreError) : BacklogUiState
}

// ---------------------------------------------------------------------------
// ViewModel
// ---------------------------------------------------------------------------

/**
 * ViewModel for the Backlog tab.
 *
 * Design rules (mirrors shared conventions):
 * - No optimistic UI: every mutation re-fetches [BacklogOps.backlog].
 * - [CoreError.NotFound] on adopt/move: friendly snackbar
 *   ("This item is no longer in the backlog.") + refresh. Qt's graceful path
 *   for the common double-tap race.
 * - Adopt success: snackbar "Planned for today" + [dataVersion] bump so
 *   DayScreen picks up the newly-planned task without polling.
 * - Refresh failure while content is on screen: snackbar, keep content
 *   (shared convention rule 4: errors surface regardless of loaded state,
 *   content must not be blanked by a refresh failure).
 * - Refresh failure with no content: Error state with Retry button.
 */
class BacklogViewModel(
    private val repository: BacklogOps,
    private val dataVersion: MutableStateFlow<Int>,
) : ViewModel() {

    private val _uiState = MutableStateFlow<BacklogUiState>(BacklogUiState.Loading)
    val uiState: StateFlow<BacklogUiState> = _uiState.asStateFlow()

    /**
     * True while a background refresh is in progress.
     * Drives [PullToRefreshBox] when content is already visible so the user
     * does not see the screen blank during a pull-to-refresh.
     */
    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    private val _snackbar = Channel<SnackbarEvent>(Channel.BUFFERED)
    val snackbarEvents = _snackbar.receiveAsFlow()

    init {
        refresh()
        // Refresh when another screen bumps the data version — e.g. DayScreen
        // MoveToBacklog, or evening check-in action 4 (Backlog).
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
            // Only blank the screen if there is no content to keep visible.
            if (_uiState.value !is BacklogUiState.Content) {
                _uiState.value = BacklogUiState.Loading
            }
            runCatching { repository.backlog() }
                .onSuccess { _uiState.value = BacklogUiState.Content(it) }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    if (_uiState.value is BacklogUiState.Content) {
                        // Content is visible: keep it and surface the error via snackbar.
                        _snackbar.trySend(SnackbarEvent("Could not refresh backlog: ${err.message}"))
                    } else {
                        _uiState.value = BacklogUiState.Error(err)
                    }
                }
            _isRefreshing.value = false
        }
    }

    // -----------------------------------------------------------------------
    // Mutations
    // -----------------------------------------------------------------------

    /**
     * Adopts [text] from the backlog into today's daily plan.
     * Always targets today's date (LocalDate.now()), not any week-navigation
     * state — matches Qt which uses time.Now() for adopt.
     */
    fun adopt(text: String) {
        viewModelScope.launch {
            runCatching {
                repository.adoptFromBacklog(LocalDate.now().toString(), text)
            }.onSuccess {
                // Bump dataVersion first so sibling screens (DayScreen, WeekScreen)
                // pick up the change before we re-fetch our own list.
                dataVersion.value++
                _snackbar.trySend(SnackbarEvent("Planned for today"))
                refresh()
            }.onFailure { t ->
                val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                when (err) {
                    is CoreError.NotFound -> {
                        // Double-tap or concurrent edit race: item vanished.
                        // Friendly path matching Qt's "item no longer in backlog" info.
                        _snackbar.trySend(SnackbarEvent("This item is no longer in the backlog."))
                        refresh()
                    }
                    else -> {
                        _snackbar.trySend(SnackbarEvent("Error: ${err.message}"))
                        refresh()
                    }
                }
            }
        }
    }

    /**
     * Shuttles [text] between sections.
     * [toNextWeek]=true: "This week" -> "Next week".
     * [toNextWeek]=false: "Next week" -> "This week".
     */
    fun move(text: String, toNextWeek: Boolean) {
        viewModelScope.launch {
            runCatching {
                repository.moveBacklogItem(text, toNextWeek)
            }.onSuccess {
                refresh()
            }.onFailure { t ->
                val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                when (err) {
                    is CoreError.NotFound -> {
                        _snackbar.trySend(SnackbarEvent("This item is no longer in the backlog."))
                        refresh()
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
        private val repository: BacklogOps,
        private val dataVersion: MutableStateFlow<Int>,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            BacklogViewModel(repository, dataVersion) as T
    }
}
