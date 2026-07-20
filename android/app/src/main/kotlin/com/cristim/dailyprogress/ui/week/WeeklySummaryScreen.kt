package com.cristim.dailyprogress.ui.week

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.WeeklySummaryDto
import com.cristim.dailyprogress.ui.checkin.ActionButtonRow
import com.cristim.dailyprogress.ui.checkin.CheckinPresentation
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.util.Locale

private val DAY_HEADER_FMT: DateTimeFormatter =
    DateTimeFormatter.ofPattern("EEE d MMM", Locale.ENGLISH)

// File-level sealed interface for WeeklySummaryScreen local state
// (sealed interfaces cannot be declared inside a composable function).
private sealed interface SummaryScreenState {
    data object Loading : SummaryScreenState
    data class Content(val summary: WeeklySummaryDto) : SummaryScreenState
    data class Error(val error: CoreError) : SummaryScreenState
}

/**
 * Weekly summary screen. Shows goals (struck when done) and done-by-day breakdown.
 *
 * Scheduled path ([presentation] == SCHEDULED): calls [CoreRepository.markWeekSummarized]
 * on OK, then [onDismiss]. Manual path: just [onDismiss] on OK (Qt's markOnAccept=false).
 *
 * This is a self-contained screen; it fetches its own summary so it works whether
 * WeekScreen is in the back stack or not (direct coordinator routing).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WeeklySummaryScreen(
    /** Any date in the week to summarize. */
    date: LocalDate,
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    presentation: CheckinPresentation,
    onDismiss: () -> Unit,
    onSnooze: () -> Unit,
    onSkipToday: () -> Unit,
) {
    var localState: SummaryScreenState by remember { mutableStateOf(SummaryScreenState.Loading) }
    var submitting by remember { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    // retryCounter: incremented by the Retry button. Keying LaunchedEffect on
    // (date, retryCounter) ensures it re-runs on Retry even though date hasn't
    // changed — plain LaunchedEffect(date) would stay dormant after an error.
    var retryCounter by remember { mutableIntStateOf(0) }

    // For scheduled path: the loop finds the oldest pending summary week.
    // For manual path: load the summary for the date provided.
    LaunchedEffect(date, retryCounter) {
        localState = SummaryScreenState.Loading
        val targetDate = if (presentation == CheckinPresentation.SCHEDULED) {
            // Find oldest pending summary week and use its date.
            runCatching { repository.weeklySummaryPending(date.toString()) }
                .getOrNull()
                ?.let { pending ->
                    if (pending.pending && pending.week.isNotEmpty()) {
                        runCatching {
                            com.cristim.dailyprogress.util.isoWeekToMonday(pending.week)
                        }.getOrDefault(date)
                    } else {
                        // Nothing pending: dismiss immediately (loop is done)
                        onDismiss()
                        return@LaunchedEffect
                    }
                } ?: date
        } else {
            date
        }
        localState = runCatching { repository.weeklySummary(targetDate.toString()) }
            .fold(
                onSuccess = { SummaryScreenState.Content(it) },
                onFailure = { t ->
                    SummaryScreenState.Error(t as? CoreError ?: CoreError.Unknown(t.message.orEmpty()))
                },
            )
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    val weekLabel = (localState as? SummaryScreenState.Content)?.summary?.week ?: "Weekly Summary"
                    Text("Summary: $weekLabel")
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = localState) {
            is SummaryScreenState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is SummaryScreenState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text("Could not load summary.", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(8.dp))
                    Text(
                        state.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = { retryCounter++ }) { Text("Retry") }
                }
            }

            is SummaryScreenState.Content -> {
                val summary = state.summary
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(horizontal = 16.dp),
                ) {
                    // Goals section (struck when done)
                    if (summary.goals.isNotEmpty()) {
                        item {
                            Spacer(Modifier.height(12.dp))
                            Text(
                                "This week's goals",
                                style = MaterialTheme.typography.titleSmall,
                                color = MaterialTheme.colorScheme.primary,
                            )
                            Spacer(Modifier.height(4.dp))
                        }
                        itemsIndexed(
                            items = summary.goals,
                            key = { index, _ -> "goal_$index" },
                        ) { _, goal ->
                            Text(
                                text = goal.text,
                                style = MaterialTheme.typography.bodyMedium.copy(
                                    textDecoration = if (goal.done) TextDecoration.LineThrough else null,
                                ),
                                color = if (goal.done) {
                                    MaterialTheme.colorScheme.onSurface.copy(alpha = 0.5f)
                                } else {
                                    MaterialTheme.colorScheme.onSurface
                                },
                                modifier = Modifier.padding(vertical = 2.dp),
                            )
                        }
                        item {
                            HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                        }
                    }

                    // Done this week section
                    item {
                        Text(
                            "Done this week",
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.height(4.dp))
                    }

                    val doneByDay = summary.doneByDay.filter { it.items.isNotEmpty() }
                    if (doneByDay.isEmpty()) {
                        item {
                            Text(
                                "Nothing completed yet this week.",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(vertical = 8.dp),
                            )
                        }
                    } else {
                        doneByDay.forEach { dayDone ->
                            item(key = "day_${dayDone.date}") {
                                Spacer(Modifier.height(8.dp))
                                val dateLabel = runCatching {
                                    LocalDate.parse(dayDone.date).format(DAY_HEADER_FMT)
                                }.getOrDefault(dayDone.date)
                                Text(
                                    dateLabel,
                                    style = MaterialTheme.typography.labelMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            itemsIndexed(
                                items = dayDone.items,
                                key = { idx, _ -> "${dayDone.date}_$idx" },
                            ) { _, item ->
                                Text(
                                    "  $item",
                                    style = MaterialTheme.typography.bodyMedium,
                                    modifier = Modifier.padding(vertical = 2.dp),
                                )
                            }
                        }
                        item {
                            val total = doneByDay.sumOf { it.items.size }
                            Spacer(Modifier.height(8.dp))
                            Text(
                                "$total item${if (total != 1) "s" else ""} completed",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }

                    // Action buttons
                    item {
                        HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                        val scope = androidx.compose.runtime.rememberCoroutineScope()
                        ActionButtonRow(
                            presentation = presentation,
                            onOk = {
                                scope.launch {
                                    if (presentation == CheckinPresentation.SCHEDULED) {
                                        submitting = true
                                        runCatching {
                                            repository.markWeekSummarized(summary.start)
                                            dataVersion.value++
                                        }.onFailure { t ->
                                            val err = t as? CoreError
                                                ?: CoreError.Unknown(t.message.orEmpty())
                                            snackbarHostState.showSnackbar(
                                                "Error marking summary: ${err.message}",
                                            )
                                            submitting = false
                                            return@launch
                                        }
                                        submitting = false
                                    }
                                    onDismiss()
                                }
                            },
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
