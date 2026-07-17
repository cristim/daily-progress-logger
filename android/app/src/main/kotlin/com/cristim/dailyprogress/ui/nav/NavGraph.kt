package com.cristim.dailyprogress.ui.nav

import androidx.compose.runtime.Composable
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.cristim.dailyprogress.core.CoreRepository
import com.cristim.dailyprogress.ui.day.DayScreen
import java.time.LocalDate

/** Route constants for the navigation graph. */
object Routes {
    const val DAY = "day/{date}"

    fun day(date: LocalDate = LocalDate.now()): String = "day/$date"
}

@Composable
fun AppNavGraph(
    repository: CoreRepository,
    navController: NavHostController = rememberNavController(),
) {
    NavHost(
        navController = navController,
        startDestination = Routes.day(),
    ) {
        composable(
            route = Routes.DAY,
            arguments = listOf(navArgument("date") { type = NavType.StringType }),
        ) { backStackEntry ->
            val dateStr = backStackEntry.arguments?.getString("date")
                ?: LocalDate.now().toString()
            DayScreen(
                initialDate = runCatching { LocalDate.parse(dateStr) }.getOrDefault(LocalDate.now()),
                repository = repository,
            )
        }
    }
}
