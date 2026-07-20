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
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.WeeklyGoalDto
import com.cristim.dailyprogress.model.WeeklyPlanDto
import com.cristim.dailyprogress.ui.checkin.ActionButtonRow
import com.cristim.dailyprogress.ui.checkin.CheckinPresentation
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import java.time.LocalDate

// File-level sealed interface for WeeklyPlanSheet local state
// (sealed interfaces cannot be declared inside a composable function).
private sealed interface PlanSheetState {
    data object Loading : PlanSheetState
    data class Content(val plan: WeeklyPlanDto) : PlanSheetState
    data class Error(val error: CoreError) : PlanSheetState
}

/**
 * Weekly plan sheet. Presented as a full-screen route for the WEEKLY_PLAN
 * scheduled prompt (prompt id=1) and (in future) from the WeekScreen toolbar.
 *
 * Shows existing goals with done-toggle + a "add goals" multiline input.
 * Saving does a full-array replace per [CoreRepository.setWeeklyPlan] contract
 * (never sends partial arrays).
 *
 * Scheduled path: shows Snooze + Skip Today per [presentation].
 * Manual path: shows Close button.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WeeklyPlanSheet(
    date: LocalDate,
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    presentation: CheckinPresentation,
    onDismiss: () -> Unit,
    onSnooze: () -> Unit,
    onSkipToday: () -> Unit,
) {
    var localState: PlanSheetState by remember { mutableStateOf(PlanSheetState.Loading) }
    // Mutable goals list: edited in-place by the user.
    var editableGoals: List<WeeklyGoalDto> by remember { mutableStateOf(emptyList()) }
    var addGoalsText by rememberSaveable { mutableStateOf("") }
    var submitting by remember { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    // retryCounter: incremented by the Retry button. Keying LaunchedEffect on
    // (date, retryCounter) ensures it re-runs on Retry even though date hasn't
    // changed — plain LaunchedEffect(date) would stay dormant after an error.
    var retryCounter by remember { mutableIntStateOf(0) }

    LaunchedEffect(date, retryCounter) {
        localState = PlanSheetState.Loading
        localState = runCatching { repository.weeklyPlan(date.toString()) }
            .fold(
                onSuccess = { plan ->
                    editableGoals = plan.goals
                    PlanSheetState.Content(plan)
                },
                onFailure = { t ->
                    PlanSheetState.Error(t as? CoreError ?: CoreError.Unknown(t.message.orEmpty()))
                },
            )
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    val weekLabel = (localState as? PlanSheetState.Content)?.plan?.week ?: "Weekly Plan"
                    Text("Plan: $weekLabel")
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = localState) {
            is PlanSheetState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is PlanSheetState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text("Could not load plan.", style = MaterialTheme.typography.titleMedium)
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

            is PlanSheetState.Content -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(horizontal = 16.dp),
                ) {
                    item {
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "Big things this week",
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.height(4.dp))
                    }

                    // Editable goal rows — index is the stable key (duplicate texts legal)
                    itemsIndexed(
                        items = editableGoals,
                        key = { index, _ -> index },
                    ) { index, goal ->
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Checkbox(
                                checked = goal.done,
                                onCheckedChange = { checked ->
                                    editableGoals = editableGoals.toMutableList()
                                        .also { it[index] = it[index].copy(done = checked) }
                                },
                            )
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
                                modifier = Modifier.weight(1f),
                            )
                        }
                    }

                    // Add new goals
                    item {
                        Spacer(Modifier.height(8.dp))
                        OutlinedTextField(
                            value = addGoalsText,
                            onValueChange = { addGoalsText = it },
                            label = { Text("Add goals (one per line)") },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(96.dp),
                            minLines = 2,
                        )
                        Spacer(Modifier.height(4.dp))
                        Row(modifier = Modifier.fillMaxWidth()) {
                            Spacer(Modifier.weight(1f))
                            TextButton(
                                onClick = {
                                    val newGoals = addGoalsText.lines()
                                        .map { it.trim() }
                                        .filter { it.isNotEmpty() }
                                        .map { WeeklyGoalDto(text = it, done = false) }
                                    if (newGoals.isNotEmpty()) {
                                        editableGoals = editableGoals + newGoals
                                        addGoalsText = ""
                                    }
                                },
                                enabled = addGoalsText.isNotBlank(),
                            ) { Text("Add") }
                        }
                        HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                    }

                    // Action buttons
                    item {
                        ActionButtonRow(
                            presentation = presentation,
                            onOk = {
                                scope.launch {
                                    submitting = true
                                    // Merge any un-added text from the input field before saving.
                                    val pendingGoals = addGoalsText.lines()
                                        .map { it.trim() }
                                        .filter { it.isNotEmpty() }
                                        .map { WeeklyGoalDto(text = it, done = false) }
                                    val goalsToSave = editableGoals + pendingGoals
                                    // Always send at least [] (never "" or "null" per contract).
                                    runCatching {
                                        repository.setWeeklyPlan(date.toString(), goalsToSave)
                                        dataVersion.value++
                                    }.onFailure { t ->
                                        val err = t as? CoreError
                                            ?: CoreError.Unknown(t.message.orEmpty())
                                        snackbarHostState.showSnackbar(
                                            "Error saving plan: ${err.message}",
                                        )
                                        submitting = false
                                        return@launch
                                    }
                                    submitting = false
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
