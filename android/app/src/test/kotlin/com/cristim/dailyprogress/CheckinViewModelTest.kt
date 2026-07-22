package com.cristim.dailyprogress

import com.cristim.dailyprogress.core.CheckinOps
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.model.EveningDecisionsDto
import com.cristim.dailyprogress.model.MorningCandidateDto
import com.cristim.dailyprogress.model.MorningDecisionsDto
import com.cristim.dailyprogress.model.TreeDto
import com.cristim.dailyprogress.model.WeeklyPlanDto
import com.cristim.dailyprogress.ui.checkin.CheckinUiState
import com.cristim.dailyprogress.ui.checkin.CheckinViewModel
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
 * JVM unit tests for CheckinViewModel's daily-prompt behaviour: loading it
 * as part of loadMorning, saving it via saveDailyPrompt, and the error path.
 * Uses a fake [CheckinOps] (same pattern as BacklogTest / WeekReviewViewModelTest)
 * so the ViewModel is testable without a real mobilecore.Core handle.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CheckinViewModelTest {

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
    // Fake CheckinOps
    // -----------------------------------------------------------------------

    private class FakeCheckinOps(
        private val candidates: List<MorningCandidateDto> = emptyList(),
        private val plan: WeeklyPlanDto = WeeklyPlanDto(week = "2026-W30", planned = false),
        private val tree: TreeDto = TreeDto(),
        private val dailyPromptText: String = "",
        private val setDailyPromptError: Throwable? = null,
    ) : CheckinOps {

        val setDailyPromptCalls = mutableListOf<String>()
        var dailyPromptCallCount = 0

        override suspend fun morningCandidates(date: String): List<MorningCandidateDto> = candidates

        override suspend fun applyMorning(date: String, decisions: MorningDecisionsDto) = Unit

        override suspend fun weeklyPlan(date: String): WeeklyPlanDto = plan

        override suspend fun tree(date: String): TreeDto = tree

        override suspend fun applyEvening(date: String, decisions: EveningDecisionsDto) = Unit

        override suspend fun dailyPrompt(): String {
            dailyPromptCallCount++
            return dailyPromptText
        }

        override suspend fun setDailyPrompt(text: String) {
            setDailyPromptCalls += text
            setDailyPromptError?.let { throw it }
        }
    }

    // -----------------------------------------------------------------------
    // loadMorning: daily prompt loading
    // -----------------------------------------------------------------------

    @Test
    fun `loadMorning loads the saved daily prompt into Morning state`() = runTest {
        val ops = FakeCheckinOps(dailyPromptText = "Ship the release")
        val vm = CheckinViewModel(ops, MutableStateFlow(0))

        vm.loadMorning(LocalDate.now())
        val state = vm.uiState.first { it is CheckinUiState.Morning } as CheckinUiState.Morning

        assertEquals("Ship the release", state.dailyPrompt)
        assertEquals(1, ops.dailyPromptCallCount)
    }

    @Test
    fun `loadMorning with an unset prompt yields empty dailyPrompt`() = runTest {
        val ops = FakeCheckinOps(dailyPromptText = "")
        val vm = CheckinViewModel(ops, MutableStateFlow(0))

        vm.loadMorning(LocalDate.now())
        val state = vm.uiState.first { it is CheckinUiState.Morning } as CheckinUiState.Morning

        assertEquals("", state.dailyPrompt)
    }

    // -----------------------------------------------------------------------
    // saveDailyPrompt: success path
    // -----------------------------------------------------------------------

    @Test
    fun `saveDailyPrompt trims and persists the text, updating state`() = runTest {
        val ops = FakeCheckinOps(dailyPromptText = "")
        val vm = CheckinViewModel(ops, MutableStateFlow(0))

        vm.loadMorning(LocalDate.now())
        vm.uiState.first { it is CheckinUiState.Morning }

        vm.saveDailyPrompt("  Ship the release  ")

        assertEquals(listOf("Ship the release"), ops.setDailyPromptCalls)
        val state = vm.uiState.value as CheckinUiState.Morning
        assertEquals("Ship the release", state.dailyPrompt)
    }

    @Test
    fun `saveDailyPrompt emits promptSavedEvents on success`() = runTest {
        val ops = FakeCheckinOps()
        val vm = CheckinViewModel(ops, MutableStateFlow(0))

        vm.loadMorning(LocalDate.now())
        vm.uiState.first { it is CheckinUiState.Morning }
        vm.saveDailyPrompt("New prompt")

        // Should not suspend: an event was already sent.
        vm.promptSavedEvents.first()
    }

    @Test
    fun `saveDailyPrompt does not bump dataVersion`() = runTest {
        val ops = FakeCheckinOps()
        val dataVersion = MutableStateFlow(0)
        val vm = CheckinViewModel(ops, dataVersion)

        vm.loadMorning(LocalDate.now())
        vm.uiState.first { it is CheckinUiState.Morning }
        vm.saveDailyPrompt("New prompt")

        assertEquals(0, dataVersion.value)
    }

    // -----------------------------------------------------------------------
    // saveDailyPrompt: error path
    // -----------------------------------------------------------------------

    @Test
    fun `saveDailyPrompt failure surfaces a snackbar and leaves state unchanged`() = runTest {
        val error = CoreError.Internal("INTERNAL: disk full")
        val ops = FakeCheckinOps(dailyPromptText = "Old prompt", setDailyPromptError = error)
        val vm = CheckinViewModel(ops, MutableStateFlow(0))

        vm.loadMorning(LocalDate.now())
        vm.uiState.first { it is CheckinUiState.Morning }
        vm.saveDailyPrompt("New prompt")

        val event = vm.snackbarEvents.first()
        assertTrue(event.message.contains("disk full"))
        val state = vm.uiState.value as CheckinUiState.Morning
        assertEquals("Old prompt", state.dailyPrompt)
    }
}
