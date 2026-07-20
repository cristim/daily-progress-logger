package com.cristim.dailyprogress

import com.cristim.dailyprogress.core.WeekReviewOps
import com.cristim.dailyprogress.model.PendingWeekDto
import com.cristim.dailyprogress.model.ReviewDecisionsDto
import com.cristim.dailyprogress.model.WeekReviewCandidatesDto
import com.cristim.dailyprogress.ui.week.ReviewUiState
import com.cristim.dailyprogress.ui.week.WeekReviewViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.LocalDate

/**
 * Tests for WeekReviewViewModel — focuses on the A1 fix: empty-candidates OK
 * must call applyWeekReview so the week is marked reviewed and the scheduled
 * loop terminates.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class WeekReviewViewModelTest {

    private val testDispatcher = UnconfinedTestDispatcher()

    @Before
    fun setup() {
        Dispatchers.setMain(testDispatcher)
    }

    @After
    fun teardown() {
        Dispatchers.resetMain()
    }

    // -----------------------------------------------------------------------
    // Fake WeekReviewOps implementations
    // -----------------------------------------------------------------------

    /** Returns empty candidates for the given anchorDate. */
    private class EmptyCandidatesFakeOps(
        private val weekDate: LocalDate,
        private val weekLabel: String = "2026-W27",
        val applyCalls: MutableList<Pair<String, ReviewDecisionsDto>> = mutableListOf(),
    ) : WeekReviewOps {
        override suspend fun unreviewedWeek(date: String): PendingWeekDto =
            // Manual path does not call this; provide a safe default.
            PendingWeekDto(pending = false, week = "")

        override suspend fun weekReviewCandidates(date: String): WeekReviewCandidatesDto =
            WeekReviewCandidatesDto(week = weekLabel, candidates = emptyList())

        override suspend fun applyWeekReview(date: String, decisions: ReviewDecisionsDto) {
            applyCalls += date to decisions
        }
    }

    /** Returns candidates for a scheduled-path test (pending week present). */
    private class ScheduledEmptyCandidatesFakeOps(
        private val pendingWeekLabel: String,
        private val pendingWeekDate: LocalDate,
        val applyCalls: MutableList<Pair<String, ReviewDecisionsDto>> = mutableListOf(),
    ) : WeekReviewOps {
        override suspend fun unreviewedWeek(date: String): PendingWeekDto =
            PendingWeekDto(pending = true, week = pendingWeekLabel)

        override suspend fun weekReviewCandidates(date: String): WeekReviewCandidatesDto =
            WeekReviewCandidatesDto(week = pendingWeekLabel, candidates = emptyList())

        override suspend fun applyWeekReview(date: String, decisions: ReviewDecisionsDto) {
            applyCalls += date to decisions
        }
    }

    // -----------------------------------------------------------------------
    // A1 tests — empty-candidates OK must call applyWeekReview
    // -----------------------------------------------------------------------

    @Test
    fun `apply on Empty state calls applyWeekReview with empty decisions`() = runTest(testDispatcher) {
        val weekDate = LocalDate.of(2026, 7, 6) // a Monday
        val fakeOps = EmptyCandidatesFakeOps(weekDate)
        val dataVersion = MutableStateFlow(0)

        val vm = WeekReviewViewModel(
            ops = fakeOps,
            dataVersion = dataVersion,
            anchorDate = weekDate,
            scheduled = false,
        )

        // Wait for initial load (Empty state since candidates is empty)
        val state = vm.uiState.first { it !is ReviewUiState.Loading }
        assertTrue("expected Empty state, got $state", state is ReviewUiState.Empty)
        assertEquals(weekDate, (state as ReviewUiState.Empty).weekDate)

        vm.apply(rollover = false)

        // Give the coroutine time to execute
        testScheduler.advanceUntilIdle()

        assertEquals("applyWeekReview must be called exactly once", 1, fakeOps.applyCalls.size)
        val (capturedDate, capturedPayload) = fakeOps.applyCalls[0]
        assertEquals(weekDate.toString(), capturedDate)
        assertTrue("decisions must be empty", capturedPayload.decisions.isEmpty())
        assertEquals(false, capturedPayload.rollover)
    }

    @Test
    fun `apply on Empty state bumps dataVersion on success`() = runTest(testDispatcher) {
        val weekDate = LocalDate.of(2026, 7, 6)
        val fakeOps = EmptyCandidatesFakeOps(weekDate)
        val dataVersion = MutableStateFlow(0)

        val vm = WeekReviewViewModel(
            ops = fakeOps,
            dataVersion = dataVersion,
            anchorDate = weekDate,
            scheduled = false,
        )
        vm.uiState.first { it !is ReviewUiState.Loading }

        vm.apply(rollover = false)
        testScheduler.advanceUntilIdle()

        assertEquals("dataVersion must be incremented after empty-OK apply", 1, dataVersion.value)
    }

    @Test
    fun `apply on Empty state passes rollover=true for scheduled path`() = runTest(testDispatcher) {
        val anchorDate = LocalDate.now()
        // Pending week is one week before today (Mon)
        val pendingDate = LocalDate.of(2026, 7, 6)
        val fakeOps = ScheduledEmptyCandidatesFakeOps(
            pendingWeekLabel = "2026-W27",
            pendingWeekDate = pendingDate,
        )
        val dataVersion = MutableStateFlow(0)

        val vm = WeekReviewViewModel(
            ops = fakeOps,
            dataVersion = dataVersion,
            anchorDate = anchorDate,
            scheduled = true,
        )
        vm.uiState.first { it !is ReviewUiState.Loading }

        vm.apply(rollover = true)
        testScheduler.advanceUntilIdle()

        assertEquals(1, fakeOps.applyCalls.size)
        assertEquals(true, fakeOps.applyCalls[0].second.rollover)
    }

    @Test
    fun `Empty state carries weekDate used for apply call`() = runTest(testDispatcher) {
        val weekDate = LocalDate.of(2026, 7, 13) // W29 Monday
        val fakeOps = EmptyCandidatesFakeOps(weekDate, weekLabel = "2026-W29")
        val dataVersion = MutableStateFlow(0)

        val vm = WeekReviewViewModel(
            ops = fakeOps,
            dataVersion = dataVersion,
            anchorDate = weekDate,
            scheduled = false,
        )
        val state = vm.uiState.first { it !is ReviewUiState.Loading }
        assertTrue(state is ReviewUiState.Empty)
        val empty = state as ReviewUiState.Empty
        assertEquals("2026-W29", empty.weekLabel)
        assertEquals(weekDate, empty.weekDate)
    }
}
