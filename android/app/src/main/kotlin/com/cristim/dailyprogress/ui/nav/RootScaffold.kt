package com.cristim.dailyprogress.ui.nav

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
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.ui.day.DayScreen
import com.cristim.dailyprogress.ui.more.MoreScreen
import kotlinx.coroutines.flow.MutableStateFlow
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

    Scaffold(
        bottomBar = {
            NavigationBar {
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

            // Check-in destinations (phase A commit 4 fills these with real screens)
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
                MorningCheckinPlaceholder(
                    onDismiss = { navController.popBackStack() },
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
                EveningCheckinPlaceholder(
                    onDismiss = { navController.popBackStack() },
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

// ---------------------------------------------------------------------------
// Phase-A check-in placeholders (replaced in commit 4)
// ---------------------------------------------------------------------------

@Composable
private fun MorningCheckinPlaceholder(onDismiss: () -> Unit) {
    Box(modifier = Modifier.fillMaxSize())
}

@Composable
private fun EveningCheckinPlaceholder(onDismiss: () -> Unit) {
    Box(modifier = Modifier.fillMaxSize())
}
