package com.cristim.dailyprogress.ui.week

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.ReviewAction
import com.cristim.dailyprogress.model.ReviewDecisionDto
import com.cristim.dailyprogress.model.ReviewDecisionsDto
import com.cristim.dailyprogress.ui.checkin.ActionButtonRow
import com.cristim.dailyprogress.ui.checkin.CheckinPresentation
import com.cristim.dailyprogress.ui.day.SnackbarEvent
import com.cristim.dailyprogress.util.isoWeekToMonday
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import java.time.LocalDate

// ---------------------------------------------------------------------------
// Review item (one candidate with its chosen action)
// ---------------------------------------------------------------------------

data class ReviewItem(
    val text: String,
    val action: ReviewAction = ReviewAction.KEEP,
)

// ---------------------------------------------------------------------------
// ViewModel state
// ---------------------------------------------------------------------------

sealed interface ReviewUiState {
    data object Loading : ReviewUiState

    /**
     * Candidates loaded and ready for user decision.
     * [weekLabel] is the "YYYY-Www" string for the reviewed week.
     * [weekDate] is the Monday of that week (used for the core call).
     */
    data class Content(
        val weekLabel: String,
        val weekDate: LocalDate,
        val items: List<ReviewItem>,
    ) : ReviewUiState

    /** No open items remain from this week — show empty state + OK. */
    data class Empty(val weekLabel: String) : ReviewUiState

    data class Error(val error: CoreError) : ReviewUiState
}

// ---------------------------------------------------------------------------
// ViewModel
// ---------------------------------------------------------------------------

/**
 * ViewModel for [WeekReviewScreen]. Self-contained: does not depend on
 * [WeekViewModel]. Manages review candidates + per-row action selections.
 *
 * Scheduled oldest-first loop: [anchorDate] is today; the VM calls
 * [CoreRepository.unreviewedWeek] to find the oldest pending week and loads
 * candidates for it. Manual path: loads candidates for the previous week directly.
 */
class WeekReviewViewModel(
    private val repository: CoreRepository,
    private val dataVersion: MutableStateFlow<Int>,
    private val anchorDate: LocalDate,
    private val scheduled: Boolean,
) : ViewModel() {

    private val _uiState = MutableStateFlow<ReviewUiState>(ReviewUiState.Loading)
    val uiState: StateFlow<ReviewUiState> = _uiState.asStateFlow()

    private val _submitting = MutableStateFlow(false)
    val submitting: StateFlow<Boolean> = _submitting.asStateFlow()

    private val _snackbar = Channel<SnackbarEvent>(Channel.BUFFERED)
    val snackbarEvents = _snackbar.receiveAsFlow()

    /** Emitted after successful apply or empty-candidates OK. */
    private val _done = Channel<Unit>(Channel.BUFFERED)
    val doneEvents = _done.receiveAsFlow()

    init {
        loadReview()
    }

    private fun loadReview() {
        viewModelScope.launch {
            _uiState.value = ReviewUiState.Loading
            runCatching {
                val reviewDate: LocalDate = if (scheduled) {
                    // Oldest-first: find the unreviewed week.
                    val pending = repository.unreviewedWeek(anchorDate.toString())
                    if (!pending.pending || pending.week.isEmpty()) {
                        // Nothing to review — emit done immediately.
                        _done.trySend(Unit)
                        return@runCatching null
                    }
                    isoWeekToMonday(pending.week)
                } else {
                    // Manual: use the previous-week anchor directly.
                    anchorDate
                }
                val dto = repository.weekReviewCandidates(reviewDate.toString())
                if (dto.candidates.isEmpty()) {
                    ReviewUiState.Empty(dto.week)
                } else {
                    ReviewUiState.Content(
                        weekLabel = dto.week,
                        weekDate = reviewDate,
                        items = dto.candidates.map { ReviewItem(it) },
                    )
                }
            }
                .onSuccess { state -> if (state != null) _uiState.value = state }
                .onFailure { t ->
                    val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                    _uiState.value = ReviewUiState.Error(err)
                }
        }
    }

    /** Updates the action for the review item at [index]. */
    fun setAction(index: Int, action: ReviewAction) {
        val content = _uiState.value as? ReviewUiState.Content ?: return
        if (index !in content.items.indices) return
        val updated = content.items.toMutableList()
            .also { it[index] = it[index].copy(action = action) }
        _uiState.value = content.copy(items = updated)
    }

    /**
     * Applies the week review.
     * [rollover] true for scheduled path (move postponed items to next week);
     * false for manual path (single pass, no rollover).
     */
    fun apply(rollover: Boolean) {
        if (_submitting.value) return
        when (val state = _uiState.value) {
            is ReviewUiState.Empty -> {
                // Nothing to apply; emit done (empty OK is valid per plan §6).
                viewModelScope.launch { _done.trySend(Unit) }
                return
            }
            is ReviewUiState.Content -> {
                viewModelScope.launch {
                    _submitting.value = true
                    val decisions = state.items.map { item ->
                        ReviewDecisionDto(text = item.text, action = item.action.wire)
                    }
                    val payload = ReviewDecisionsDto(decisions = decisions, rollover = rollover)
                    runCatching {
                        repository.applyWeekReview(state.weekDate.toString(), payload)
                    }
                        .onSuccess {
                            dataVersion.value++
                            _done.trySend(Unit)
                        }
                        .onFailure { t ->
                            val err = t as? CoreError ?: CoreError.Unknown(t.message.orEmpty())
                            _snackbar.trySend(SnackbarEvent("Error applying review: ${err.message}"))
                        }
                    _submitting.value = false
                }
            }
            else -> { /* Loading/Error: no-op */ }
        }
    }

    class Factory(
        private val repository: CoreRepository,
        private val dataVersion: MutableStateFlow<Int>,
        private val anchorDate: LocalDate,
        private val scheduled: Boolean,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            WeekReviewViewModel(repository, dataVersion, anchorDate, scheduled) as T
    }
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

/**
 * Week review screen. Presents open items from the reviewed week with a
 * 3-way Keep / Postpone / Drop segmented button per row.
 *
 * Scheduled path ([presentation] == SCHEDULED): finds the oldest unreviewed
 * week via [CoreRepository.unreviewedWeek] and applies with rollover=true.
 * Manual path: uses the previous week, rollover=false.
 *
 * Empty-candidates state displays "Nothing left open from <week>. Great job!"
 * and an OK button that dismisses immediately.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WeekReviewScreen(
    /** Today for scheduled path; previous-week Monday for manual path. */
    anchorDate: LocalDate,
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    presentation: CheckinPresentation,
    onDismiss: () -> Unit,
    onSnooze: () -> Unit,
    onSkipToday: () -> Unit,
) {
    val vm: WeekReviewViewModel = viewModel(
        key = "review_${anchorDate}_${presentation.name}",
        factory = WeekReviewViewModel.Factory(
            repository = repository,
            dataVersion = dataVersion,
            anchorDate = anchorDate,
            scheduled = presentation == CheckinPresentation.SCHEDULED,
        ),
    )
    val uiState by vm.uiState.collectAsStateWithLifecycle()
    val submitting by vm.submitting.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(Unit) { vm.doneEvents.collect { onDismiss() } }
    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event -> snackbarHostState.showSnackbar(event.message) }
    }

    val rollover = presentation == CheckinPresentation.SCHEDULED

    Scaffold(
        topBar = {
            @OptIn(ExperimentalMaterial3Api::class)
            androidx.compose.material3.TopAppBar(
                title = {
                    val weekLabel = when (val s = uiState) {
                        is ReviewUiState.Content -> s.weekLabel
                        is ReviewUiState.Empty -> s.weekLabel
                        else -> "Week Review"
                    }
                    Text("Review: $weekLabel")
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = uiState) {
            is ReviewUiState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is ReviewUiState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text("Could not load review data.", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(8.dp))
                    Text(
                        state.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = onDismiss) { Text("Close") }
                }
            }

            is ReviewUiState.Empty -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Spacer(Modifier.weight(1f))
                    Text(
                        "Nothing left open from ${state.weekLabel}. Great job!",
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Spacer(Modifier.weight(1f))
                    ActionButtonRow(
                        presentation = presentation,
                        onOk = { vm.apply(rollover) },
                        onSnooze = onSnooze,
                        onSkipOrClose = if (presentation == CheckinPresentation.SCHEDULED) {
                            onSkipToday
                        } else {
                            onDismiss
                        },
                        enabled = !submitting,
                    )
                    Spacer(Modifier.height(16.dp))
                }
            }

            is ReviewUiState.Content -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(horizontal = 16.dp),
                ) {
                    item {
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "Open items from ${state.weekLabel}",
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.height(4.dp))
                    }

                    // Review candidates — index is the stable key (duplicate texts legal)
                    itemsIndexed(
                        items = state.items,
                        key = { index, _ -> index },
                    ) { index, item ->
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 4.dp),
                        ) {
                            Text(item.text, style = MaterialTheme.typography.bodyMedium)
                            Spacer(Modifier.height(4.dp))
                            ReviewActionRow(
                                selected = item.action,
                                onSelect = { vm.setAction(index, it) },
                            )
                        }
                        HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))
                    }

                    // Action buttons
                    item {
                        Spacer(Modifier.height(8.dp))
                        ActionButtonRow(
                            presentation = presentation,
                            onOk = { vm.apply(rollover) },
                            onSnooze = onSnooze,
                            onSkipOrClose = if (presentation == CheckinPresentation.SCHEDULED) {
                                onSkipToday
                            } else {
                                onDismiss
                            },
                            enabled = !submitting,
                        )
                        Spacer(Modifier.height(16.dp))
                    }
                }
            }
        }
    }
}

/** 3-way segmented button: Keep / Postpone / Drop. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ReviewActionRow(
    selected: ReviewAction,
    onSelect: (ReviewAction) -> Unit,
) {
    val options = listOf(
        ReviewAction.KEEP to "Keep",
        ReviewAction.POSTPONE to "Postpone",
        ReviewAction.DROP to "Drop",
    )
    SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
        options.forEachIndexed { index, (action, label) ->
            SegmentedButton(
                shape = SegmentedButtonDefaults.itemShape(index = index, count = options.size),
                onClick = { onSelect(action) },
                selected = selected == action,
                label = { Text(label, style = MaterialTheme.typography.labelSmall) },
            )
        }
    }
}
