package com.cristim.dailyprogress.ui.nav

import java.time.LocalDate

/** Route constants for the navigation graph. All route strings live here. */
object Routes {
    // Bottom nav destinations
    const val DAY = "day/{date}"
    const val WEEK = "week"
    const val BACKLOG = "backlog"
    const val MORE = "more"

    // Check-in destinations (added in Phase A commit 4)
    const val MORNING_CHECKIN = "checkin/morning/{date}/{scheduled}"
    const val EVENING_CHECKIN = "checkin/evening/{date}/{scheduled}"

    // Week sub-routes (phase B): review, summary, plan-edit
    const val WEEK_REVIEW = "week/review/{date}/{scheduled}"
    const val WEEK_SUMMARY = "week/summary/{date}/{scheduled}"
    const val WEEK_PLAN = "week/plan/{date}/{scheduled}"

    // More sub-routes (filled in later phases)
    const val PROJECTS = "projects"
    const val RECURRING = "recurring"
    const val RECYCLE = "recycle"
    const val SYNC = "sync"
    const val SETTINGS = "settings"

    fun day(date: LocalDate = LocalDate.now()): String = "day/$date"
    fun morningCheckin(date: LocalDate = LocalDate.now(), scheduled: Boolean = true): String =
        "checkin/morning/$date/$scheduled"
    fun eveningCheckin(date: LocalDate = LocalDate.now(), scheduled: Boolean = true): String =
        "checkin/evening/$date/$scheduled"
    fun weekReview(date: LocalDate = LocalDate.now(), scheduled: Boolean = true): String =
        "week/review/$date/$scheduled"
    fun weekSummary(date: LocalDate = LocalDate.now(), scheduled: Boolean = true): String =
        "week/summary/$date/$scheduled"
    fun weekPlan(date: LocalDate = LocalDate.now(), scheduled: Boolean = true): String =
        "week/plan/$date/$scheduled"
}
