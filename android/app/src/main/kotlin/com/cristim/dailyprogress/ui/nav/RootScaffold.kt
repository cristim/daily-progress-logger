package com.cristim.dailyprogress.ui.nav

import android.content.Context
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.DateRange
import androidx.compose.material.icons.filled.MoreHoriz
import androidx.compose.material.icons.filled.Today
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.model.PromptId
import com.cristim.dailyprogress.ui.checkin.CheckinCoordinator
import com.cristim.dailyprogress.ui.checkin.CheckinPresentation
import com.cristim.dailyprogress.ui.checkin.EveningCheckinScreen
import com.cristim.dailyprogress.ui.checkin.MorningCheckinScreen
import com.cristim.dailyprogress.ui.checkin.SharedPrefsSnoozeSkipStorage
import com.cristim.dailyprogress.ui.day.DayScreen
import com.cristim.dailyprogress.ui.more.MoreScreen
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.collect
import java.time.LocalDate

/**
 * Root of the UI: Scaffold with Material 3 bottom NavigationBar + NavHost.
 *
 * Check-in routes (phase A commit 4) and week/backlog/more screen routes
 * (phases B-F) are all registered here so later phases only need to wire
 * their composables rather than restructure navigation.
 */
@Composable
fun RootScaffold(
    repository: CoreRepository,
    dataVersion: MutableStateFlow<Int>,
    navController: NavHostController = rememberNavController(),
) {
    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentRoute = backStackEntry?.destination?.route
    // True while a scheduled check-in is on top; hides the bottom nav so check-ins
    // are truly modal and users cannot tap away to another tab mid-prompt.
    val isCheckinRoute = currentRoute?.startsWith("checkin/") == true

    // -----------------------------------------------------------------------
    // Check-in prompt coordinator: runs on each app resume via repeatOnLifecycle.
    //
    // checkNextSignal is bumped after every check-in dismiss so the coordinator
    // re-checks for additional due prompts in the same resume session (A-1: loop
    // until all due prompts are handled, morning then evening).
    // -----------------------------------------------------------------------
    val context = LocalContext.current
    val coordinator = remember {
        val prefs = context.getSharedPreferences("checkin_coordinator", Context.MODE_PRIVATE)
        CheckinCoordinator(repository, SharedPrefsSnoozeSkipStorage(prefs))
    }
    val checkNextSignal = remember { MutableStateFlow(0) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            // StateFlow always replays its current value, so this fires once on
            // every resume and once more after each dismiss increment.
            checkNextSignal.collect { _ ->
                // A-2: skip when a check-in screen is already on top; prevents
                // stacking duplicates on repeated ON_RESUME while the prompt is open.
                if (navController.currentBackStackEntry
                        ?.destination?.route?.startsWith("checkin/") == true
                ) return@collect

                val prompt = coordinator.nextPresentable() ?: return@collect
                val id = try {
                    PromptId.fromWire(prompt.id)
                } catch (_: Exception) {
                    return@collect // unknown id: already logged in coordinator
                }
                when (id) {
                    PromptId.MORNING ->
                        navController.navigate(Routes.morningCheckin(LocalDate.now(), scheduled = true))
                    PromptId.EVENING ->
                        navController.navigate(Routes.eveningCheckin(LocalDate.now(), scheduled = true))
                    PromptId.WEEK_REVIEW, PromptId.WEEKLY_PLAN, PromptId.WEEKLY_SUMMARY -> {
                        // phase B: not routed yet
                    }
                }
            }
        }
    }

    Scaffold(
        bottomBar = {
            // Hidden during check-ins so the bottom tabs are not tappable while
            // a scheduled prompt is presented (IA rule: check-ins are always modal).
            if (!isCheckinRoute) NavigationBar {
                NavigationBarItem(
                    icon = { Icon(Icons.Filled.Today, contentDescription = null) },
                    label = { Text("Today", textAlign = TextAlign.Center) },
                    selected = currentRoute?.startsWith("day/") == true,
                    onClick = {
                        navController.navigate(Routes.day()) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    },
                )
                NavigationBarItem(
                    icon = { Icon(Icons.Filled.DateRange, contentDescription = null) },
                    label = { Text("Week", textAlign = TextAlign.Center) },
                    selected = currentRoute == Routes.WEEK,
                    onClick = {
                        navController.navigate(Routes.WEEK) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    },
                )
                NavigationBarItem(
                    icon = {
                        Icon(Icons.AutoMirrored.Filled.List, contentDescription = null)
                    },
                    label = { Text("Backlog", textAlign = TextAlign.Center) },
                    selected = currentRoute == Routes.BACKLOG,
                    onClick = {
                        navController.navigate(Routes.BACKLOG) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    },
                )
                NavigationBarItem(
                    icon = { Icon(Icons.Filled.MoreHoriz, contentDescription = null) },
                    label = { Text("More", textAlign = TextAlign.Center) },
                    selected = currentRoute == Routes.MORE ||
                        currentRoute == Routes.PROJECTS ||
                        currentRoute == Routes.RECURRING ||
                        currentRoute == Routes.RECYCLE ||
                        currentRoute == Routes.SYNC ||
                        currentRoute == Routes.SETTINGS,
                    onClick = {
                        navController.navigate(Routes.MORE) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    },
                )
            }
        },
    ) { outerPadding ->
        NavHost(
            navController = navController,
            startDestination = Routes.day(),
            modifier = Modifier
                .fillMaxSize()
                .padding(outerPadding),
        ) {
            // Today tab
            composable(
                route = Routes.DAY,
                arguments = listOf(navArgument("date") { type = NavType.StringType }),
            ) { backStack ->
                val dateStr = backStack.arguments?.getString("date") ?: LocalDate.now().toString()
                DayScreen(
                    initialDate = runCatching { LocalDate.parse(dateStr) }.getOrDefault(LocalDate.now()),
                    repository = repository,
                    dataVersion = dataVersion,
                    onMorningCheckin = {
                        navController.navigate(
                            Routes.morningCheckin(LocalDate.now(), scheduled = false),
                        )
                    },
                    onEveningCheckin = {
                        navController.navigate(
                            Routes.eveningCheckin(LocalDate.now(), scheduled = false),
                        )
                    },
                )
            }

            // Week tab (phase B)
            composable(Routes.WEEK) {
                WeekPlaceholder()
            }

            // Backlog tab (phase C)
            composable(Routes.BACKLOG) {
                BacklogPlaceholder()
            }

            // More tab + nested destinations (phases D-H)
            composable(Routes.MORE) { MoreScreen() }
            composable(Routes.PROJECTS) { MoreScreen() }    // phase E
            composable(Routes.RECURRING) { MoreScreen() }   // phase D
            composable(Routes.RECYCLE) { MoreScreen() }     // phase F
            composable(Routes.SYNC) { MoreScreen() }        // phase H
            composable(Routes.SETTINGS) { MoreScreen() }    // phase G

            // Check-in destinations (routed by coordinator on resume + manual menu)
            composable(
                route = Routes.MORNING_CHECKIN,
                arguments = listOf(
                    navArgument("date") { type = NavType.StringType },
                    navArgument("scheduled") { type = NavType.StringType },
                ),
            ) { backStack ->
                val date = runCatching {
                    LocalDate.parse(backStack.arguments?.getString("date") ?: "")
                }.getOrDefault(LocalDate.now())
                val scheduled = backStack.arguments?.getString("scheduled")?.toBooleanStrictOrNull() != false
                val presentation = if (scheduled) CheckinPresentation.SCHEDULED else CheckinPresentation.MANUAL
                MorningCheckinScreen(
                    date = date,
                    repository = repository,
                    dataVersion = dataVersion,
                    presentation = presentation,
                    onDismiss = {
                        navController.popBackStack()
                        checkNextSignal.value++
                    },
                    onSnooze = {
                        coordinator.snooze(PromptId.MORNING.wire)
                        navController.popBackStack()
                        checkNextSignal.value++
                    },
                    onSkipToday = {
                        coordinator.skipToday(PromptId.MORNING.wire)
                        navController.popBackStack()
                        checkNextSignal.value++
                    },
                )
            }

            composable(
                route = Routes.EVENING_CHECKIN,
                arguments = listOf(
                    navArgument("date") { type = NavType.StringType },
                    navArgument("scheduled") { type = NavType.StringType },
                ),
            ) { backStack ->
                val date = runCatching {
                    LocalDate.parse(backStack.arguments?.getString("date") ?: "")
                }.getOrDefault(LocalDate.now())
                val scheduled = backStack.arguments?.getString("scheduled")?.toBooleanStrictOrNull() != false
                val presentation = if (scheduled) CheckinPresentation.SCHEDULED else CheckinPresentation.MANUAL
                EveningCheckinScreen(
                    date = date,
                    repository = repository,
                    dataVersion = dataVersion,
                    presentation = presentation,
                    onDismiss = {
                        navController.popBackStack()
                        checkNextSignal.value++
                    },
                    onSnooze = {
                        coordinator.snooze(PromptId.EVENING.wire)
                        navController.popBackStack()
                        checkNextSignal.value++
                    },
                    onSkipToday = {
                        coordinator.skipToday(PromptId.EVENING.wire)
                        navController.popBackStack()
                        checkNextSignal.value++
                    },
                )
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Phase-B/C placeholder screens (replaced when those phases land)
// ---------------------------------------------------------------------------

@Composable
private fun WeekPlaceholder() {
    Box(modifier = Modifier.fillMaxSize())
}

@Composable
private fun BacklogPlaceholder() {
    Box(modifier = Modifier.fillMaxSize())
}
