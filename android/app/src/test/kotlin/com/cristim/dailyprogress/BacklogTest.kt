package com.cristim.dailyprogress

import com.cristim.dailyprogress.core.BacklogOps
import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.model.BacklogDto
import com.cristim.dailyprogress.ui.backlog.BacklogUiState
import com.cristim.dailyprogress.ui.backlog.BacklogViewModel
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

/**
 * JVM unit tests for BacklogViewModel. Tests the Phase C contract:
 * - adopt success: snackbar "Planned for today" + dataVersion bump.
 * - adopt/move NOT_FOUND: friendly snackbar + refresh, no error state.
 * - dataVersion change observed: ViewModel refreshes.
 *
 * DTO fixture tests (populate / empty) are already covered in
 * [DtoDecodingTest]; this file focuses on ViewModel behaviour via
 * a fake [BacklogOps] implementation (same pattern as WeekReviewViewModelTest).
 */
@OptIn(ExperimentalCoroutinesApi::class)
class BacklogTest {

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
    // Fake BacklogOps implementations
    // -----------------------------------------------------------------------

    /**
     * Simple fake BacklogOps that tracks calls and optionally injects errors.
     */
    private class FakeBacklogOps(
        private val backlogResult: BacklogDto = BacklogDto(),
        private val adoptError: Throwable? = null,
        private val moveError: Throwable? = null,
    ) : BacklogOps {

        val adoptCalls = mutableListOf<Pair<String, String>>()   // date -> text
        val moveCalls = mutableListOf<Pair<String, Boolean>>()   // text -> toNextWeek
        var backlogCallCount = 0

        override suspend fun backlog(): BacklogDto {
            backlogCallCount++
            return backlogResult
        }

        override suspend fun adoptFromBacklog(date: String, text: String) {
            adoptCalls += date to text
            adoptError?.let { throw it }
        }

        override suspend fun moveBacklogItem(text: String, toNextWeek: Boolean) {
            moveCalls += text to toNextWeek
            moveError?.let { throw it }
        }
    }

    // -----------------------------------------------------------------------
    // Initial load
    // -----------------------------------------------------------------------

    @Test
    fun `populated backlog loads into Content state`() = runTest {
        val backlog = BacklogDto(
            current = listOf("Write tests", "Fix bug #42"),
            nextWeek = listOf("Plan sprint"),
        )
        val vm = BacklogViewModel(FakeBacklogOps(backlog), MutableStateFlow(0))

        val state = vm.uiState.first { it is BacklogUiState.Content } as BacklogUiState.Content
        assertEquals(2, state.backlog.current.size)
        assertEquals("Write tests", state.backlog.current[0])
        assertEquals("Fix bug #42", state.backlog.current[1])
        assertEquals(1, state.backlog.nextWeek.size)
        assertEquals("Plan sprint", state.backlog.nextWeek[0])
    }

    @Test
    fun `empty backlog loads into Content state with empty lists`() = runTest {
        val vm = BacklogViewModel(FakeBacklogOps(BacklogDto()), MutableStateFlow(0))

        val state = vm.uiState.first { it is BacklogUiState.Content } as BacklogUiState.Content
        assertTrue(state.backlog.current.isEmpty())
        assertTrue(state.backlog.nextWeek.isEmpty())
    }

    // -----------------------------------------------------------------------
    // adopt: success path
    // -----------------------------------------------------------------------

    @Test
    fun `adopt success sends Planned-for-today snackbar and bumps dataVersion`() = runTest {
        val ops = FakeBacklogOps(BacklogDto(current = listOf("Task A")))
        val dataVersion = MutableStateFlow(0)
        val vm = BacklogViewModel(ops, dataVersion)

        vm.uiState.first { it is BacklogUiState.Content }
        vm.adopt("Task A")

        val event = vm.snackbarEvents.first()
        assertEquals("Planned for today: Task A", event.message)
        assertTrue("dataVersion must be bumped on adopt success", dataVersion.value > 0)
    }

    @Test
    fun `adopt records the correct date and text`() = runTest {
        val ops = FakeBacklogOps(BacklogDto(current = listOf("Buy milk")))
        val vm = BacklogViewModel(ops, MutableStateFlow(0))

        vm.uiState.first { it is BacklogUiState.Content }
        vm.adopt("Buy milk")

        assertEquals(1, ops.adoptCalls.size)
        val (date, text) = ops.adoptCalls[0]
        // Date must be a valid YYYY-MM-DD (LocalDate.now().toString()).
        assertTrue("Date must match YYYY-MM-DD pattern", date.matches(Regex("\\d{4}-\\d{2}-\\d{2}")))
        assertEquals("Buy milk", text)
    }

    // -----------------------------------------------------------------------
    // adopt: NOT_FOUND path (friendly snackbar, no error state)
    // -----------------------------------------------------------------------

    @Test
    fun `adopt NOT_FOUND shows friendly snackbar and keeps content visible`() = runTest {
        val notFound = CoreError.NotFound("NOT_FOUND: item not in backlog")
        val ops = FakeBacklogOps(
            backlogResult = BacklogDto(current = listOf("Task A")),
            adoptError = notFound,
        )
        val dataVersion = MutableStateFlow(0)
        val vm = BacklogViewModel(ops, dataVersion)

        vm.uiState.first { it is BacklogUiState.Content }
        vm.adopt("Task A")

        val event = vm.snackbarEvents.first()
        assertEquals("This item is no longer in the backlog.", event.message)
        // dataVersion must NOT be bumped on failure.
        assertEquals(0, dataVersion.value)
        // State must remain Content (not Error) after a NOT_FOUND.
        assertTrue(
            "State must remain Content after NOT_FOUND",
            vm.uiState.value is BacklogUiState.Content,
        )
    }

    // -----------------------------------------------------------------------
    // move: NOT_FOUND path
    // -----------------------------------------------------------------------

    @Test
    fun `move NOT_FOUND shows friendly snackbar and keeps content visible`() = runTest {
        val notFound = CoreError.NotFound("NOT_FOUND: item not in backlog")
        val ops = FakeBacklogOps(
            backlogResult = BacklogDto(current = listOf("Task B")),
            moveError = notFound,
        )
        val vm = BacklogViewModel(ops, MutableStateFlow(0))

        vm.uiState.first { it is BacklogUiState.Content }
        vm.move("Task B", toNextWeek = true)

        val event = vm.snackbarEvents.first()
        assertEquals("This item is no longer in the backlog.", event.message)
        assertTrue(
            "State must remain Content after NOT_FOUND",
            vm.uiState.value is BacklogUiState.Content,
        )
    }

    @Test
    fun `move records correct text and direction`() = runTest {
        val ops = FakeBacklogOps(BacklogDto(current = listOf("Task C"), nextWeek = listOf("Task D")))
        val vm = BacklogViewModel(ops, MutableStateFlow(0))

        vm.uiState.first { it is BacklogUiState.Content }
        vm.move("Task C", toNextWeek = true)

        assertEquals(1, ops.moveCalls.size)
        assertEquals("Task C" to true, ops.moveCalls[0])
    }

    // -----------------------------------------------------------------------
    // dataVersion observation: change triggers refresh
    // -----------------------------------------------------------------------

    @Test
    fun `dataVersion bump causes backlog to refresh`() = runTest {
        val ops = FakeBacklogOps(BacklogDto())
        val dataVersion = MutableStateFlow(0)
        val vm = BacklogViewModel(ops, dataVersion)

        // Wait for initial load.
        vm.uiState.first { it is BacklogUiState.Content }
        val callsAfterInit = ops.backlogCallCount

        // Simulate another screen mutating data (e.g. DayScreen MoveToBacklog).
        dataVersion.value++

        // ViewModel must re-fetch backlog after the bump.
        vm.uiState.first { it is BacklogUiState.Content }
        assertTrue(
            "backlog() must be called again after dataVersion bump",
            ops.backlogCallCount > callsAfterInit,
        )
    }
}
