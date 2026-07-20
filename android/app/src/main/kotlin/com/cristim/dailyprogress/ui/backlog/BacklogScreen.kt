package com.cristim.dailyprogress.ui.backlog

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.BacklogDto
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * Backlog tab screen. Shows "This week" and "Next week" sections; each item
 * has two trailing icon buttons:
 *  - Plan today (down arrow): adopts the item into today's daily plan.
 *  - Shuttle (directional arrow): moves the item between sections.
 *
 * Adopt always targets today (LocalDate.now()), not the day-navigation state
 * of any other tab — matches Qt which uses time.Now() for adopt.
 *
 * NOT_FOUND on either action surfaces as a friendly snackbar + refresh,
 * never as an error state (matches Qt's "item is no longer in the backlog"
 * info dialog).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BacklogScreen(
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
) {
    val vm: BacklogViewModel = viewModel(
        factory = BacklogViewModel.Factory(repository, dataVersion),
    )
    val uiState by vm.uiState.collectAsStateWithLifecycle()
    val isRefreshing by vm.isRefreshing.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(Unit) {
        vm.snackbarEvents.collect { event -> snackbarHostState.showSnackbar(event.message) }
    }

    Scaffold(
        topBar = { TopAppBar(title = { Text("Backlog") }) },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = uiState) {
            is BacklogUiState.Loading -> {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }

            is BacklogUiState.Error -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .padding(16.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text(
                        "Could not load backlog",
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

            is BacklogUiState.Content -> {
                PullToRefreshBox(
                    isRefreshing = isRefreshing,
                    onRefresh = vm::refresh,
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(padding),
                ) {
                    BacklogContent(
                        backlog = state.backlog,
                        onAdopt = vm::adopt,
                        onMove = vm::move,
                    )
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Stateless content
// ---------------------------------------------------------------------------

@Composable
private fun BacklogContent(
    backlog: BacklogDto,
    onAdopt: (String) -> Unit,
    onMove: (String, Boolean) -> Unit,
) {
    val hasContent = backlog.current.isNotEmpty() || backlog.nextWeek.isNotEmpty()

    if (!hasContent) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState()),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                "Nothing in the backlog",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        return
    }

    LazyColumn(modifier = Modifier.fillMaxSize()) {
        if (backlog.current.isNotEmpty()) {
            item(key = "current-header") {
                BacklogSectionHeader("This week")
            }
            // Row keys are section + index. Duplicate texts are legal (same item
            // added twice) — never use the string as a key (shared convention).
            itemsIndexed(
                items = backlog.current,
                key = { index, _ -> "current-$index" },
            ) { _, text ->
                BacklogRow(
                    text = text,
                    shuttleContentDescription = "Move to next week",
                    shuttleIcon = {
                        Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null)
                    },
                    onAdopt = { onAdopt(text) },
                    onShuttle = { onMove(text, true) },
                )
            }
        }

        if (backlog.nextWeek.isNotEmpty()) {
            item(key = "next-week-header") {
                BacklogSectionHeader("Next week")
            }
            itemsIndexed(
                items = backlog.nextWeek,
                key = { index, _ -> "next-$index" },
            ) { _, text ->
                BacklogRow(
                    text = text,
                    shuttleContentDescription = "Move to this week",
                    shuttleIcon = {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null)
                    },
                    onAdopt = { onAdopt(text) },
                    onShuttle = { onMove(text, false) },
                )
            }
        }
    }
}

@Composable
private fun BacklogSectionHeader(label: String) {
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
private fun BacklogRow(
    text: String,
    shuttleContentDescription: String,
    shuttleIcon: @Composable () -> Unit,
    onAdopt: () -> Unit,
    onShuttle: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = 16.dp, end = 4.dp, top = 4.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )
        // Plan today: adopt item into today's plan (down arrow = pull into today).
        IconButton(onClick = onAdopt) {
            Icon(
                Icons.Filled.ArrowDownward,
                contentDescription = "Plan today",
                tint = MaterialTheme.colorScheme.primary,
            )
        }
        // Shuttle: move to the other section (arrow direction matches the destination).
        IconButton(onClick = onShuttle) {
            shuttleIcon()
        }
    }
}
