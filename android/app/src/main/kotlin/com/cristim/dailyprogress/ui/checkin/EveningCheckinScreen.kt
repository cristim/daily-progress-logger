package com.cristim.dailyprogress.ui.checkin

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
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
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
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.EveningAction
import kotlinx.coroutines.flow.MutableStateFlow
import java.time.LocalDate

/**
 * Evening check-in screen: one row per plan item with a 5-way action dropdown,
 * plus a multiline input for bonus accomplished items.
 *
 * The 5 actions (0=todo, 1=done, 2=next day, 3=next week, 4=backlog) match
 * the evening action constants in mobilecore/checkin.go. Initial selection is
 * seeded via [CheckinViewModel.eveningActionForState].
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EveningCheckinScreen(
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
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(date) { vm.loadEvening(date) }
    LaunchedEffect(Unit) { vm.doneEvents.collect { onDismiss() } }
    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event -> snackbarHostState.showSnackbar(event.message) }
    }

    var extraText by rememberSaveable { mutableStateOf("") }

    Scaffold(
        topBar = {
            TopAppBar(title = { Text("How did today go?") })
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = uiState) {
            is CheckinUiState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
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
                    Text("Could not load check-in data.", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(8.dp))
                    Text(
                        state.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = { vm.loadEvening(date) }) { Text("Retry") }
                }
            }

            is CheckinUiState.Evening -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(horizontal = 16.dp),
                ) {
                    if (state.items.isEmpty()) {
                        item {
                            Spacer(Modifier.height(32.dp))
                            Text(
                                "No plan was recorded for today.",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.fillMaxWidth(),
                            )
                        }
                    } else {
                        // Task rows with per-row 5-way action dropdown
                        itemsIndexed(state.items) { index, item ->
                            EveningTaskRow(
                                item = item,
                                onActionChange = { action -> vm.setEveningAction(index, action) },
                            )
                        }
                    }

                    // Bonus accomplished items
                    item {
                        Spacer(Modifier.height(12.dp))
                        Text(
                            "Anything else you accomplished?",
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.height(4.dp))
                        OutlinedTextField(
                            value = extraText,
                            onValueChange = { extraText = it },
                            label = { Text("One item per line") },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(100.dp),
                            minLines = 2,
                        )
                        Spacer(Modifier.height(12.dp))
                    }

                    // Action buttons
                    item {
                        HorizontalDivider()
                        Spacer(Modifier.height(8.dp))
                        ActionButtonRow(
                            presentation = presentation,
                            onOk = { vm.applyEvening(date, extraText) },
                            onSnooze = onSnooze,
                            onSkipOrClose = if (presentation == CheckinPresentation.SCHEDULED) onSkipToday else onDismiss,
                            enabled = !submitting,
                        )
                        Spacer(Modifier.height(16.dp))
                    }
                }
            }

            // Morning state: not expected here.
            is CheckinUiState.Morning -> Unit
        }
    }
}

// ---------------------------------------------------------------------------
// Per-row composable
// ---------------------------------------------------------------------------

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun EveningTaskRow(
    item: EveningItem,
    onActionChange: (EveningAction) -> Unit,
) {
    var menuExpanded by remember { mutableStateOf(false) }
    val indentDp = (item.depth * 20).dp

    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = indentDp, top = 4.dp, bottom = 4.dp),
    ) {
        Text(
            text = item.text,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )

        // Compact exposed dropdown for the 5-way action
        ExposedDropdownMenuBox(
            expanded = menuExpanded,
            onExpandedChange = { menuExpanded = !menuExpanded },
        ) {
            OutlinedTextField(
                value = item.action.label(),
                onValueChange = {},
                readOnly = true,
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = menuExpanded) },
                modifier = Modifier
                    .menuAnchor(MenuAnchorType.PrimaryNotEditable)
                    .padding(start = 8.dp),
                textStyle = MaterialTheme.typography.bodySmall,
                singleLine = true,
            )
            ExposedDropdownMenu(
                expanded = menuExpanded,
                onDismissRequest = { menuExpanded = false },
            ) {
                EveningAction.entries.forEach { action ->
                    DropdownMenuItem(
                        text = { Text(action.label()) },
                        onClick = {
                            onActionChange(action)
                            menuExpanded = false
                        },
                    )
                }
            }
        }
    }
}

private fun EveningAction.label(): String = when (this) {
    EveningAction.TODO -> "Not done"
    EveningAction.DONE -> "Done"
    EveningAction.NEXT_DAY -> "Next day"
    EveningAction.NEXT_WEEK -> "Next week"
    EveningAction.BACKLOG -> "Backlog"
}
