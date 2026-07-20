package com.cristim.dailyprogress.ui.week

import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.RateReview
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
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
import androidx.navigation.NavController
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.ui.nav.Routes
import kotlinx.coroutines.flow.MutableStateFlow
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.util.Locale

private val DAY_HEADER_FMT: DateTimeFormatter =
    DateTimeFormatter.ofPattern("EEE d MMM", Locale.ENGLISH)

/**
 * Week tab screen. Shows:
 *  - Week navigation header (prev / label / next, "This Week" reset)
 *  - "Big things" plan section: goal rows with done-toggle + add-goals input
 *  - Review-pending badge/button (visible when prior weeks await review)
 *  - "Done this week" section: done-by-day groups with per-day header
 *  - Overflow menu: "This Week's Summary..." (manual) + "Review Last Week..." (manual)
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WeekScreen(
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    navController: NavController,
) {
    val vm: WeekViewModel = viewModel(
        factory = WeekViewModel.Factory(repository, dataVersion),
    )
    val uiState by vm.uiState.collectAsStateWithLifecycle()
    val submitting by vm.submitting.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event -> snackbarHostState.showSnackbar(event.message) }
    }

    var addGoalsText by rememberSaveable { mutableStateOf("") }
    var menuExpanded by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    val label = when (val s = uiState) {
                        is WeekUiState.Content -> s.summary.week
                        else -> "Week"
                    }
                    Text(label)
                },
                navigationIcon = {
                    IconButton(onClick = { vm.prevWeek() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Previous week")
                    }
                },
                actions = {
                    IconButton(onClick = { vm.nextWeek() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = "Next week")
                    }
                    // Overflow menu
                    IconButton(onClick = { menuExpanded = true }) {
                        Icon(Icons.Filled.MoreVert, contentDescription = "More options")
                    }
                    DropdownMenu(
                        expanded = menuExpanded,
                        onDismissRequest = { menuExpanded = false },
                    ) {
                        DropdownMenuItem(
                            text = { Text("This Week") },
                            onClick = {
                                menuExpanded = false
                                vm.thisWeek()
                            },
                        )
                        DropdownMenuItem(
                            text = { Text("This Week's Summary...") },
                            onClick = {
                                menuExpanded = false
                                // Manual summary: never marks summarized (scheduled=false)
                                val date = (uiState as? WeekUiState.Content)?.referenceDate
                                    ?: LocalDate.now()
                                navController.navigate(Routes.weekSummary(date, scheduled = false))
                            },
                        )
                        DropdownMenuItem(
                            text = { Text("Review Last Week...") },
                            onClick = {
                                menuExpanded = false
                                // Manual review: previous week, rollover=false
                                val lastWeek = LocalDate.now().minusWeeks(1)
                                navController.navigate(Routes.weekReview(lastWeek, scheduled = false))
                            },
                        )
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = uiState) {
            is WeekUiState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is WeekUiState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text(
                        "Could not load week data.",
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Spacer(Modifier.height(8.dp))
                    Text(
                        state.error.message.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = { vm.refresh() }) { Text("Retry") }
                }
            }

            is WeekUiState.Content -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(horizontal = 16.dp),
                ) {
                    // -------------------------------------------------------
                    // Review-pending badge (only when prior weeks await review)
                    // -------------------------------------------------------
                    if (state.reviewPending) {
                        item {
                            Spacer(Modifier.height(8.dp))
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp),
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Icon(
                                    Icons.Filled.RateReview,
                                    contentDescription = null,
                                    tint = MaterialTheme.colorScheme.error,
                                )
                                Text(
                                    "Prior week(s) need review",
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.error,
                                )
                                Spacer(Modifier.weight(1f))
                                TextButton(onClick = {
                                    val lastWeek = LocalDate.now().minusWeeks(1)
                                    navController.navigate(
                                        Routes.weekReview(lastWeek, scheduled = false),
                                    )
                                }) { Text("Review") }
                            }
                            HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))
                        }
                    }

                    // -------------------------------------------------------
                    // Big things section (weekly plan goals)
                    // -------------------------------------------------------
                    item {
                        Spacer(Modifier.height(12.dp))
                        Text(
                            "Big things",
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.height(4.dp))
                    }

                    // Goal rows — use index as key (duplicate texts are legal)
                    itemsIndexed(
                        items = state.plan.goals,
                        key = { index, _ -> index },
                    ) { index, goal ->
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Checkbox(
                                checked = goal.done,
                                onCheckedChange = { checked -> vm.setGoalDone(index, checked) },
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

                    // Add-goals input (one goal per line)
                    item {
                        Spacer(Modifier.height(4.dp))
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
                        Row(horizontalArrangement = Arrangement.End, modifier = Modifier.fillMaxWidth()) {
                            TextButton(
                                onClick = {
                                    vm.addGoals(addGoalsText)
                                    addGoalsText = ""
                                },
                                enabled = !submitting && addGoalsText.isNotBlank(),
                            ) {
                                Text("Add")
                            }
                        }
                        HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                    }

                    // -------------------------------------------------------
                    // Done this week section (grouped by day)
                    // -------------------------------------------------------
                    item {
                        Text(
                            "Done this week",
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Spacer(Modifier.height(4.dp))
                    }

                    val doneByDay = state.summary.doneByDay.filter { it.items.isNotEmpty() }

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
                                    text = "  $item",
                                    style = MaterialTheme.typography.bodyMedium,
                                    modifier = Modifier.padding(vertical = 2.dp),
                                )
                            }
                        }

                        // Total count footer
                        item {
                            val total = doneByDay.sumOf { it.items.size }
                            Spacer(Modifier.height(8.dp))
                            Text(
                                "$total item${if (total != 1) "s" else ""} completed this week",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }

                    item { Spacer(Modifier.height(24.dp)) }
                }
            }
        }
    }
}
