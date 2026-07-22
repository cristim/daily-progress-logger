package com.cristim.dailyprogress.ui.checkin

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.cristim.dailyprogress.core.CoreRepository
import kotlinx.coroutines.flow.MutableStateFlow
import java.time.LocalDate

/**
 * Morning check-in screen: weekly goals read-only, summary of already-planned
 * items, a multiline new-task input, and a candidate carry-over checklist.
 *
 * Presented as a full-screen route from [CheckinCoordinator] (scheduled) or
 * from the Day top-bar overflow menu (manual). Presentation mode determines
 * which action buttons are shown (rule: scheduled -> Snooze + Skip Today;
 * manual -> Close with no bookkeeping).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MorningCheckinScreen(
    date: LocalDate,
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    presentation: CheckinPresentation,
    onDismiss: () -> Unit,
    onSnooze: () -> Unit,
    onSkipToday: () -> Unit,
) {
    val vm: CheckinViewModel = viewModel(
        factory = CheckinViewModel.Factory(repository, dataVersion),
    )
    val uiState by vm.uiState.collectAsStateWithLifecycle()
    val submitting by vm.submitting.collectAsStateWithLifecycle()
    val promptSubmitting by vm.promptSubmitting.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(date) { vm.loadMorning(date) }
    LaunchedEffect(Unit) { vm.doneEvents.collect { onDismiss() } }
    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event -> snackbarHostState.showSnackbar(event.message) }
    }

    var newItemsText by rememberSaveable { mutableStateOf("") }
    var promptEditing by rememberSaveable { mutableStateOf(false) }
    var promptDraft by rememberSaveable { mutableStateOf("") }
    LaunchedEffect(Unit) { vm.promptSavedEvents.collect { promptEditing = false } }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("What are you planning to work on today?") },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = uiState) {
            is CheckinUiState.Loading -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Spacer(Modifier.weight(1f))
                    CircularProgressIndicator()
                    Spacer(Modifier.weight(1f))
                }
            }

            is CheckinUiState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text(
                        "Could not load check-in data.",
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Spacer(Modifier.height(8.dp))
                    Text(
                        state.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = { vm.loadMorning(date) }) { Text("Retry") }
                }
            }

            is CheckinUiState.Morning -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(horizontal = 16.dp),
                ) {
                    // Daily prompt — always the first row, tap to edit.
                    item {
                        Spacer(Modifier.height(4.dp))
                        DailyPromptRow(
                            prompt = state.dailyPrompt,
                            editing = promptEditing,
                            draft = promptDraft,
                            submitting = promptSubmitting,
                            onDraftChange = { promptDraft = it },
                            onStartEdit = {
                                promptDraft = state.dailyPrompt
                                promptEditing = true
                            },
                            onSave = { vm.saveDailyPrompt(promptDraft) },
                            onCancel = { promptEditing = false },
                        )
                        HorizontalDivider(modifier = Modifier.padding(top = 8.dp))
                    }

                    // Weekly goals section (read-only, shown when non-empty)
                    if (state.goals.isNotEmpty()) {
                        item {
                            Spacer(Modifier.height(12.dp))
                            Text(
                                "This week's goals",
                                style = MaterialTheme.typography.labelLarge,
                                color = MaterialTheme.colorScheme.primary,
                            )
                            Spacer(Modifier.height(4.dp))
                        }
                        itemsIndexed(state.goals) { _, goal ->
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
                        item { HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp)) }
                    }

                    // Already-planned summary line
                    if (state.plannedCount > 0) {
                        item {
                            Text(
                                "Already planned today: ${state.plannedCount} item${if (state.plannedCount != 1) "s" else ""}",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(bottom = 8.dp),
                            )
                        }
                    }

                    // New tasks multiline input
                    item {
                        OutlinedTextField(
                            value = newItemsText,
                            onValueChange = { newItemsText = it },
                            label = { Text("Add tasks (one per line)") },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(120.dp),
                            minLines = 3,
                        )
                        Spacer(Modifier.height(12.dp))
                    }

                    // Candidate carry-over checklist (hidden when empty)
                    if (state.candidates.isNotEmpty()) {
                        item {
                            Text(
                                "Carry-over items",
                                style = MaterialTheme.typography.labelLarge,
                                color = MaterialTheme.colorScheme.primary,
                            )
                        }
                        itemsIndexed(state.candidates) { index, candidate ->
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Checkbox(
                                    checked = state.adopted.getOrElse(index) { false },
                                    onCheckedChange = { vm.toggleCandidate(index) },
                                )
                                Text(
                                    text = buildString {
                                        append(candidate.text)
                                        if (candidate.fromBacklog) append("  (backlog)")
                                    },
                                    style = MaterialTheme.typography.bodyMedium,
                                    modifier = Modifier.weight(1f),
                                )
                            }
                        }
                        item { Spacer(Modifier.height(8.dp)) }
                    }

                    // Action buttons
                    item {
                        HorizontalDivider()
                        Spacer(Modifier.height(8.dp))
                        ActionButtonRow(
                            presentation = presentation,
                            onOk = { vm.applyMorning(date, newItemsText) },
                            onSnooze = onSnooze,
                            onSkipOrClose = if (presentation == CheckinPresentation.SCHEDULED) onSkipToday else onDismiss,
                            enabled = !submitting,
                        )
                        Spacer(Modifier.height(16.dp))
                    }
                }
            }

            // Evening state: not expected here but handled for exhaustiveness.
            is CheckinUiState.Evening -> Unit
        }
    }
}

// ---------------------------------------------------------------------------
// Daily prompt row — display mode shows the saved text (or a muted
// placeholder when unset); tapping switches to an inline editor.
// ---------------------------------------------------------------------------

@Composable
private fun DailyPromptRow(
    prompt: String,
    editing: Boolean,
    draft: String,
    /** True while saveDailyPrompt is in flight; disables Save/Cancel. */
    submitting: Boolean,
    onDraftChange: (String) -> Unit,
    onStartEdit: () -> Unit,
    onSave: () -> Unit,
    onCancel: () -> Unit,
) {
    if (editing) {
        Column(modifier = Modifier.fillMaxWidth()) {
            OutlinedTextField(
                value = draft,
                onValueChange = onDraftChange,
                label = { Text("Daily prompt") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                enabled = !submitting,
            )
            Spacer(Modifier.height(8.dp))
            Row {
                Button(onClick = onSave, enabled = !submitting) { Text("Save") }
                Spacer(Modifier.width(8.dp))
                TextButton(onClick = onCancel, enabled = !submitting) { Text("Cancel") }
            }
            Spacer(Modifier.height(4.dp))
        }
    } else {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onStartEdit)
                .padding(vertical = 8.dp),
        ) {
            Text(
                text = prompt.ifEmpty { "Set a daily prompt…" },
                style = MaterialTheme.typography.bodyLarge,
                color = if (prompt.isEmpty()) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
                modifier = Modifier.weight(1f),
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Shared button row — reused by EveningCheckinScreen
// ---------------------------------------------------------------------------

@Composable
internal fun ActionButtonRow(
    presentation: CheckinPresentation,
    onOk: () -> Unit,
    onSnooze: () -> Unit,
    onSkipOrClose: () -> Unit,
    /** False while the apply call is in flight to prevent double-submit. */
    enabled: Boolean = true,
) {
    Row(modifier = Modifier.fillMaxWidth()) {
        Button(
            onClick = onOk,
            enabled = enabled,
            modifier = Modifier
                .weight(1f)
                .padding(end = 4.dp),
        ) {
            Text("OK")
        }
        TextButton(
            onClick = onSnooze,
            modifier = Modifier.padding(horizontal = 4.dp),
        ) {
            Text("Remind in 1h")
        }
        TextButton(onClick = onSkipOrClose) {
            Text(if (presentation == CheckinPresentation.SCHEDULED) "Skip Today" else "Close")
        }
    }
}
