package com.cristim.dailyprogress.ui.recurring

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Repeat
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
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
import com.cristim.dailyprogress.model.RecurringTemplateDto
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * Recurring Templates screen (More > Recurring). Lists every recurring
 * template with its schedule caption; add via FAB (typed @-recurrence
 * syntax); remove via trailing delete icon with a confirmation dialog
 * (already-materialized occurrences are not removed, matching Qt).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RecurringScreen(
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    onBack: () -> Unit,
) {
    val vm: RecurringViewModel = viewModel(
        factory = RecurringViewModel.Factory(repository, dataVersion),
    )
    val uiState by vm.uiState.collectAsStateWithLifecycle()
    val isRefreshing by vm.isRefreshing.collectAsStateWithLifecycle()
    val addFieldError by vm.addFieldError.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showAddDialog by rememberSaveable { mutableStateOf(false) }
    var pendingDelete by remember { mutableStateOf<RecurringTemplateDto?>(null) }

    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event -> snackbarHostState.showSnackbar(event.message) }
    }
    LaunchedEffect(Unit) {
        vm.addDoneEvents.collect { showAddDialog = false }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Recurring Templates") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = {
                vm.clearAddFieldError()
                showAddDialog = true
            }) {
                Icon(Icons.Filled.Add, contentDescription = "Add recurring template")
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = uiState) {
            is RecurringUiState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is RecurringUiState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text(
                        "Could not load recurring templates",
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        state.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                    Button(
                        onClick = vm::refresh,
                        modifier = Modifier.padding(top = 16.dp),
                    ) {
                        Text("Retry")
                    }
                }
            }

            is RecurringUiState.Content -> {
                PullToRefreshBox(
                    isRefreshing = isRefreshing,
                    onRefresh = vm::refresh,
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                ) {
                    RecurringContent(
                        templates = state.templates,
                        onDelete = { pendingDelete = it },
                    )
                }
            }
        }
    }

    if (showAddDialog) {
        AddRecurringDialog(
            fieldError = addFieldError,
            onFieldErrorClear = vm::clearAddFieldError,
            onAdd = vm::add,
            onDismiss = { showAddDialog = false },
        )
    }

    pendingDelete?.let { template ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text("Delete this recurring task?") },
            text = { Text("Already-created occurrences stay. This only removes the template.") },
            confirmButton = {
                TextButton(onClick = {
                    vm.remove(template.raw)
                    pendingDelete = null
                }) {
                    Text("Delete", color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = null }) { Text("Cancel") }
            },
        )
    }
}

// ---------------------------------------------------------------------------
// Stateless content
// ---------------------------------------------------------------------------

@Composable
private fun RecurringContent(
    templates: List<RecurringTemplateDto>,
    onDelete: (RecurringTemplateDto) -> Unit,
) {
    if (templates.isEmpty()) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState()),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                "No recurring templates yet",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        return
    }

    LazyColumn(modifier = Modifier.fillMaxSize()) {
        // Index-based keys: duplicate raw lines are not guaranteed unique
        // (same rule as other list screens — never key by content string).
        itemsIndexed(templates, key = { index, _ -> index }) { _, template ->
            RecurringRow(template = template, onDelete = { onDelete(template) })
        }
    }
}

@Composable
private fun RecurringRow(
    template: RecurringTemplateDto,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = 16.dp, end = 4.dp, top = 8.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Filled.Repeat,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.primary,
        )
        Spacer(Modifier.width(12.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(text = template.text, style = MaterialTheme.typography.bodyMedium)
            // describe == null only when this row came from the RecurringJSON
            // management shape rather than TreeJSON; the screen always reads
            // TreeJSON so this should not happen in practice, but render no
            // caption rather than an empty one if it ever does.
            val caption = buildString {
                template.describe?.takeIf { it.isNotEmpty() }?.let { append(it) }
                if (template.project.isNotEmpty()) {
                    if (isNotEmpty()) append(" · ")
                    append(template.project)
                }
            }
            if (caption.isNotEmpty()) {
                Text(
                    text = caption,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        IconButton(onClick = onDelete) {
            Icon(
                Icons.Filled.Delete,
                contentDescription = "Delete ${template.text}",
                tint = MaterialTheme.colorScheme.error,
            )
        }
    }
}

@Composable
private fun AddRecurringDialog(
    fieldError: String?,
    onFieldErrorClear: () -> Unit,
    onAdd: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var text by rememberSaveable { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add recurring template") },
        text = {
            Column {
                Text(
                    "Include a recurrence tag: @daily, @weekly @mon, or @monthly @1. " +
                        "Add @HH:MM for a specific time and #project to file it.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = text,
                    onValueChange = {
                        text = it
                        if (fieldError != null) onFieldErrorClear()
                    },
                    label = { Text("e.g. Standup @weekly @mon @09:00") },
                    singleLine = true,
                    isError = fieldError != null,
                    supportingText = fieldError?.let { message ->
                        { Text(message, color = MaterialTheme.colorScheme.error) }
                    },
                )
            }
        },
        confirmButton = {
            Button(
                onClick = { if (text.isNotBlank()) onAdd(text.trim()) },
                enabled = text.isNotBlank(),
            ) {
                Text("Add")
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}
