package com.cristim.dailyprogress.ui.day

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
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
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.CheckBox
import androidx.compose.material.icons.filled.CheckBoxOutlineBlank
import androidx.compose.material.icons.filled.IndeterminateCheckBox
import androidx.compose.material.icons.filled.Today
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.TaskState
import com.cristim.dailyprogress.model.TreeProjectDto
import com.cristim.dailyprogress.model.TreeTaskDto
import java.time.LocalDate
import java.time.format.DateTimeFormatter

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

@Composable
fun DayScreen(
    initialDate: LocalDate,
    repository: CoreRepository,
) {
    val vm: DayViewModel = viewModel(factory = DayViewModel.Factory(repository, initialDate))
    val uiState by vm.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    // Collect snackbar events from the ViewModel.
    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event ->
            snackbarHostState.showSnackbar(event.message)
        }
    }

    DayScreenContent(
        uiState = uiState,
        snackbarHostState = snackbarHostState,
        onPrevDay = vm::prevDay,
        onNextDay = vm::nextDay,
        onToday = vm::today,
        onRefresh = vm::refresh,
        onToggleDone = vm::toggleTaskDone,
        onSetPostponed = vm::setTaskPostponed,
        onDelete = vm::deleteTask,
        onEdit = vm::editTaskText,
        onPostponeNextDay = vm::postponeToNextDay,
        onPostponeNextWeek = vm::postponeToNextWeek,
        onMoveToBacklog = vm::moveToBacklog,
        onAddTask = vm::addTask,
    )
}

// ---------------------------------------------------------------------------
// Stateless content
// ---------------------------------------------------------------------------

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DayScreenContent(
    uiState: DayUiState,
    snackbarHostState: SnackbarHostState,
    onPrevDay: () -> Unit,
    onNextDay: () -> Unit,
    onToday: () -> Unit,
    onRefresh: () -> Unit,
    onToggleDone: (index: Long, expectedText: String, date: String, currentState: TaskState) -> Unit,
    onSetPostponed: (index: Long, expectedText: String, date: String) -> Unit,
    onDelete: (index: Long, expectedText: String, date: String) -> Unit,
    onEdit: (index: Long, expectedText: String, date: String, newText: String) -> Unit,
    onPostponeNextDay: (index: Long, expectedText: String, date: String) -> Unit,
    onPostponeNextWeek: (index: Long, expectedText: String, date: String) -> Unit,
    onMoveToBacklog: (index: Long, expectedText: String, date: String) -> Unit,
    onAddTask: (text: String, projectId: String) -> Unit,
) {
    // Derive the displayed date from state for the top bar.
    val displayDate = when (uiState) {
        is DayUiState.Content -> uiState.date
        is DayUiState.Error -> uiState.date
        DayUiState.Loading -> LocalDate.now()
    }
    val dateLabel = displayDate.format(DateTimeFormatter.ofPattern("EEE, d MMM yyyy"))
    val isToday = displayDate == LocalDate.now()

    // Bottom sheet / dialog state.
    var selectedTask by remember { mutableStateOf<TreeTaskDto?>(null) }
    var showAddDialog by remember { mutableStateOf(false) }
    var addTaskProjectId by remember { mutableStateOf("") }
    var editDialogTask by remember { mutableStateOf<TreeTaskDto?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        IconButton(onClick = onPrevDay) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Previous day")
                        }
                        Text(
                            text = dateLabel,
                            modifier = Modifier.clickable { /* TODO: date picker */ },
                        )
                        IconButton(onClick = onNextDay) {
                            Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = "Next day")
                        }
                        if (!isToday) {
                            IconButton(onClick = onToday) {
                                Icon(Icons.Filled.Today, contentDescription = "Go to today")
                            }
                        }
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = {
                addTaskProjectId = ""
                showAddDialog = true
            }) {
                Icon(Icons.Filled.Add, contentDescription = "Add task")
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (uiState) {
            DayUiState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is DayUiState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text("Could not load tasks", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(8.dp))
                    Text(
                        uiState.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = onRefresh) { Text("Retry") }
                }
            }

            is DayUiState.Content -> {
                val tree = uiState.tree
                val date = uiState.date.toString()

                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                ) {
                    // Projects section.
                    tree.projects.forEach { project ->
                        item(key = "proj-header-${project.id}") {
                            ProjectHeader(
                                project = project,
                                onAddProjectTask = {
                                    addTaskProjectId = project.id
                                    showAddDialog = true
                                },
                            )
                        }
                        items(
                            items = project.tasks,
                            key = { task -> "proj-${project.id}-task-${task.index}" },
                        ) { task ->
                            TaskRow(
                                task = task,
                                onToggleDone = { onToggleDone(task.index, task.text, date, task.state) },
                                onLongPress = { selectedTask = task },
                            )
                            if (task.children.isNotEmpty()) {
                                task.children.forEach { child ->
                                    TaskRow(
                                        task = child,
                                        onToggleDone = {
                                            onToggleDone(child.index, child.text, date, child.state)
                                        },
                                        onLongPress = { selectedTask = child },
                                    )
                                }
                            }
                        }
                    }

                    // Unfiled section.
                    if (tree.unfiled.isNotEmpty()) {
                        item(key = "unfiled-header") {
                            SectionHeader("Unfiled")
                        }
                        items(
                            items = tree.unfiled,
                            key = { task -> "unfiled-task-${task.index}" },
                        ) { task ->
                            TaskRow(
                                task = task,
                                onToggleDone = { onToggleDone(task.index, task.text, date, task.state) },
                                onLongPress = { selectedTask = task },
                            )
                            task.children.forEach { child ->
                                TaskRow(
                                    task = child,
                                    onToggleDone = {
                                        onToggleDone(child.index, child.text, date, child.state)
                                    },
                                    onLongPress = { selectedTask = child },
                                )
                            }
                        }
                    }

                    // Empty state.
                    if (tree.projects.isEmpty() && tree.unfiled.isEmpty()) {
                        item(key = "empty") {
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 48.dp),
                                contentAlignment = Alignment.Center,
                            ) {
                                Text(
                                    "No tasks for $dateLabel",
                                    style = MaterialTheme.typography.bodyLarge,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    }

                    // Recurring preview (compact, tap-through deferred to v1 Recurring screen).
                    if (tree.recurring.isNotEmpty()) {
                        item(key = "recurring-header") {
                            SectionHeader("Recurring (${tree.recurring.size})")
                        }
                    }

                    // Recycled preview (compact, tap-through deferred to Recycle screen).
                    if (tree.recycled.isNotEmpty()) {
                        item(key = "recycled-header") {
                            SectionHeader("Recently deleted (${tree.recycled.size})")
                        }
                    }
                }

                // Task actions bottom sheet.
                selectedTask?.let { task ->
                    TaskActionsSheet(
                        task = task,
                        date = date,
                        onDismiss = { selectedTask = null },
                        onToggleDone = {
                            onToggleDone(task.index, task.text, date, task.state)
                            selectedTask = null
                        },
                        onSetPostponed = {
                            onSetPostponed(task.index, task.text, date)
                            selectedTask = null
                        },
                        onPostponeNextDay = {
                            onPostponeNextDay(task.index, task.text, date)
                            selectedTask = null
                        },
                        onPostponeNextWeek = {
                            onPostponeNextWeek(task.index, task.text, date)
                            selectedTask = null
                        },
                        onMoveToBacklog = {
                            onMoveToBacklog(task.index, task.text, date)
                            selectedTask = null
                        },
                        onDelete = {
                            onDelete(task.index, task.text, date)
                            selectedTask = null
                        },
                        onEdit = {
                            editDialogTask = task
                            selectedTask = null
                        },
                    )
                }
            }
        }
    }

    // Add task dialog.
    if (showAddDialog) {
        AddTaskDialog(
            projectId = addTaskProjectId,
            onAdd = { text, projectId ->
                onAddTask(text, projectId)
                showAddDialog = false
            },
            onDismiss = { showAddDialog = false },
        )
    }

    // Edit task dialog.
    editDialogTask?.let { task ->
        EditTaskDialog(
            current = task.text,
            onSave = { newText ->
                onEdit(task.index, task.text, task.date, newText)
                editDialogTask = null
            },
            onDismiss = { editDialogTask = null },
        )
    }
}

// ---------------------------------------------------------------------------
// Composable components
// ---------------------------------------------------------------------------

@Composable
private fun SectionHeader(label: String) {
    Text(
        text = label,
        style = MaterialTheme.typography.labelLarge,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 6.dp),
    )
    HorizontalDivider()
}

@Composable
private fun ProjectHeader(
    project: TreeProjectDto,
    onAddProjectTask: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = project.name,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.SemiBold,
            color = if (project.done) MaterialTheme.colorScheme.outline
                    else MaterialTheme.colorScheme.primary,
            modifier = Modifier.weight(1f),
        )
        IconButton(onClick = onAddProjectTask) {
            Icon(Icons.Filled.Add, contentDescription = "Add task to ${project.name}")
        }
    }
    HorizontalDivider()
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun TaskRow(
    task: TreeTaskDto,
    onToggleDone: () -> Unit,
    onLongPress: () -> Unit,
) {
    val indent = (task.depth * 24).dp
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(
                onClick = {},
                onLongClick = onLongPress,
            )
            .padding(start = 16.dp + indent, end = 16.dp, top = 4.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(
            checked = task.state == TaskState.DONE,
            onCheckedChange = { onToggleDone() },
        )
        Spacer(Modifier.width(8.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = task.text,
                style = MaterialTheme.typography.bodyMedium,
                color = when (task.state) {
                    TaskState.DONE -> MaterialTheme.colorScheme.outline
                    TaskState.POSTPONED -> MaterialTheme.colorScheme.tertiary
                    TaskState.TODO -> MaterialTheme.colorScheme.onSurface
                },
            )
            if (task.state == TaskState.POSTPONED) {
                Text(
                    text = "postponed",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.tertiary,
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TaskActionsSheet(
    task: TreeTaskDto,
    date: String,
    onDismiss: () -> Unit,
    onToggleDone: () -> Unit,
    onSetPostponed: () -> Unit,
    onPostponeNextDay: () -> Unit,
    onPostponeNextWeek: () -> Unit,
    onMoveToBacklog: () -> Unit,
    onDelete: () -> Unit,
    onEdit: () -> Unit,
) {
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 32.dp),
        ) {
            Text(
                text = task.text,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(horizontal = 24.dp, vertical = 12.dp),
                maxLines = 2,
            )
            HorizontalDivider()

            val isDone = task.state == TaskState.DONE
            ActionItem(
                icon = if (isDone) Icons.Filled.CheckBoxOutlineBlank else Icons.Filled.CheckBox,
                label = if (isDone) "Mark as to-do" else "Mark as done",
                onClick = onToggleDone,
            )
            if (task.state != TaskState.POSTPONED) {
                ActionItem(
                    icon = Icons.Filled.IndeterminateCheckBox,
                    label = "Mark as postponed",
                    onClick = onSetPostponed,
                )
            }
            ActionItem(
                icon = Icons.AutoMirrored.Filled.ArrowForward,
                label = "Postpone to tomorrow",
                onClick = onPostponeNextDay,
            )
            ActionItem(
                icon = Icons.AutoMirrored.Filled.ArrowForward,
                label = "Postpone to next week",
                onClick = onPostponeNextWeek,
            )
            ActionItem(
                icon = Icons.Filled.Add,
                label = "Move to backlog",
                onClick = onMoveToBacklog,
            )
            ActionItem(
                icon = Icons.Filled.Add,
                label = "Edit text",
                onClick = onEdit,
            )
            HorizontalDivider()
            ActionItem(
                icon = Icons.Filled.Add,
                label = "Delete",
                onClick = onDelete,
                destructive = true,
            )
        }
    }
}

@Composable
private fun ActionItem(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    onClick: () -> Unit,
    destructive: Boolean = false,
) {
    ListItem(
        headlineContent = {
            Text(
                text = label,
                color = if (destructive) MaterialTheme.colorScheme.error
                        else MaterialTheme.colorScheme.onSurface,
            )
        },
        leadingContent = {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = if (destructive) MaterialTheme.colorScheme.error
                       else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        },
        modifier = Modifier.clickable(onClick = onClick),
    )
}

@Composable
private fun AddTaskDialog(
    projectId: String,
    onAdd: (text: String, projectId: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var text by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add task") },
        text = {
            OutlinedTextField(
                value = text,
                onValueChange = { text = it },
                label = { Text("Task text") },
                singleLine = true,
            )
        },
        confirmButton = {
            Button(
                onClick = { if (text.isNotBlank()) onAdd(text.trim(), projectId) },
            ) { Text("Add") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}

@Composable
private fun EditTaskDialog(
    current: String,
    onSave: (newText: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var text by remember(current) { mutableStateOf(current) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Edit task") },
        text = {
            OutlinedTextField(
                value = text,
                onValueChange = { text = it },
                label = { Text("Task text") },
                singleLine = true,
            )
        },
        confirmButton = {
            Button(
                onClick = { if (text.isNotBlank()) onSave(text.trim()) },
            ) { Text("Save") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}
