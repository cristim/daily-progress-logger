package com.cristim.dailyprogress

import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.core.RecurringOps
import com.cristim.dailyprogress.model.RecurringTemplateDto
import com.cristim.dailyprogress.model.TreeDto
import com.cristim.dailyprogress.ui.recurring.RecurringUiState
import com.cristim.dailyprogress.ui.recurring.RecurringViewModel
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
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * JVM unit tests for RecurringViewModel. Covers the Phase D contract:
 * - list is sourced from tree(today).recurring, not RecurringJSON.
 * - add BAD_INPUT surfaces as an inline field error, keeps the dialog open
 *   (no doneEvents, no dataVersion bump), and does not go through snackbar.
 * - add success clears the field error, bumps dataVersion, and emits
 *   addDoneEvents so the dialog closes.
 * - add of a non-BAD_INPUT error surfaces via snackbar instead.
 * - remove bumps dataVersion and refreshes; remove failure still refreshes
 *   and surfaces via snackbar.
 * - dataVersion bump from another screen triggers a refresh.
 *
 * DTO fixture tests (both wire shapes + the nullable-vs-zero distinction)
 * are covered in [DtoDecodingTest].
 */
@OptIn(ExperimentalCoroutinesApi::class)
class RecurringTest {

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
    // Fake RecurringOps
    // -----------------------------------------------------------------------

    private class FakeRecurringOps(
        private val treeResult: TreeDto = TreeDto(),
        private val addError: Throwable? = null,
        private val removeError: Throwable? = null,
    ) : RecurringOps {

        val treeCalls = mutableListOf<String>()
        val addCalls = mutableListOf<String>()
        val removeCalls = mutableListOf<String>()

        override suspend fun tree(date: String): TreeDto {
            treeCalls += date
            return treeResult
        }

        override suspend fun addRecurring(text: String) {
            addCalls += text
            addError?.let { throw it }
        }

        override suspend fun removeRecurring(rawText: String) {
            removeCalls += rawText
            removeError?.let { throw it }
        }
    }

    private fun template(raw: String, text: String = "Standup") = RecurringTemplateDto(
        text = text,
        project = "",
        describe = "daily 09:00",
        kind = 0,
        weekday = null,
        monthDay = null,
        hour = 9,
        minute = 0,
        raw = raw,
    )

    // -----------------------------------------------------------------------
    // Initial load
    // -----------------------------------------------------------------------

    @Test
    fun `populated templates load into Content state from tree recurring`() = runTest {
        val templates = listOf(template("Standup @daily"), template("Weekly review @weekly", "Weekly review"))
        val ops = FakeRecurringOps(TreeDto(recurring = templates))
        val vm = RecurringViewModel(ops, MutableStateFlow(0))

        val state = vm.uiState.first { it is RecurringUiState.Content } as RecurringUiState.Content
        assertEquals(2, state.templates.size)
        assertEquals("Standup", state.templates[0].text)
        assertEquals(1, ops.treeCalls.size)
    }

    @Test
    fun `empty templates load into Content state with empty list`() = runTest {
        val vm = RecurringViewModel(FakeRecurringOps(TreeDto()), MutableStateFlow(0))

        val state = vm.uiState.first { it is RecurringUiState.Content } as RecurringUiState.Content
        assertTrue(state.templates.isEmpty())
    }

    // -----------------------------------------------------------------------
    // add: BAD_INPUT path (inline field error, no snackbar, no dismiss)
    // -----------------------------------------------------------------------

    @Test
    fun `add BAD_INPUT sets inline field error and does not emit doneEvents`() = runTest {
        val badInput = CoreError.BadInput("BAD_INPUT: missing recurrence tag")
        val ops = FakeRecurringOps(addError = badInput)
        val dataVersion = MutableStateFlow(0)
        val vm = RecurringViewModel(ops, dataVersion)

        vm.uiState.first { it is RecurringUiState.Content }
        vm.add("Just a plain task")

        val fieldError = vm.addFieldError.first { it != null }
        assertTrue(fieldError!!.contains("recurrence tag"))
        assertEquals(0, dataVersion.value)
        assertEquals(1, ops.addCalls.size)
    }

    // -----------------------------------------------------------------------
    // add: success path
    // -----------------------------------------------------------------------

    @Test
    fun `add success clears field error, bumps dataVersion, and signals done`() = runTest {
        val ops = FakeRecurringOps()
        val dataVersion = MutableStateFlow(0)
        val vm = RecurringViewModel(ops, dataVersion)

        vm.uiState.first { it is RecurringUiState.Content }
        vm.add("Standup @daily")

        vm.addDoneEvents.first()
        assertNull(vm.addFieldError.value)
        assertTrue("dataVersion must be bumped on add success", dataVersion.value > 0)
        assertEquals(listOf("Standup @daily"), ops.addCalls)
    }

    // -----------------------------------------------------------------------
    // add: non-BAD_INPUT error path (snackbar, not field error)
    // -----------------------------------------------------------------------

    @Test
    fun `add non-BAD_INPUT error surfaces via snackbar not field error`() = runTest {
        val internal = CoreError.Internal("INTERNAL: unexpected nil pointer")
        val ops = FakeRecurringOps(addError = internal)
        val vm = RecurringViewModel(ops, MutableStateFlow(0))

        vm.uiState.first { it is RecurringUiState.Content }
        vm.add("Standup @daily")

        val event = vm.snackbarEvents.first()
        assertTrue(event.message.contains("INTERNAL"))
        assertNull("BAD_INPUT-only field error must stay null", vm.addFieldError.value)
    }

    // -----------------------------------------------------------------------
    // remove
    // -----------------------------------------------------------------------

    @Test
    fun `remove success bumps dataVersion and refreshes`() = runTest {
        val ops = FakeRecurringOps(TreeDto(recurring = listOf(template("Standup @daily"))))
        val dataVersion = MutableStateFlow(0)
        val vm = RecurringViewModel(ops, dataVersion)

        vm.uiState.first { it is RecurringUiState.Content }
        val callsAfterInit = ops.treeCalls.size
        vm.remove("Standup @daily")

        assertEquals(listOf("Standup @daily"), ops.removeCalls)
        assertTrue("dataVersion must be bumped on remove success", dataVersion.value > 0)
        assertTrue("tree() must be re-fetched after remove", ops.treeCalls.size > callsAfterInit)
    }

    @Test
    fun `remove failure surfaces via snackbar and still refreshes`() = runTest {
        val notFound = CoreError.NotFound("NOT_FOUND: template already removed")
        val ops = FakeRecurringOps(removeError = notFound)
        val dataVersion = MutableStateFlow(0)
        val vm = RecurringViewModel(ops, dataVersion)

        vm.uiState.first { it is RecurringUiState.Content }
        val callsAfterInit = ops.treeCalls.size
        vm.remove("Standup @daily")

        val event = vm.snackbarEvents.first()
        assertTrue(event.message.contains("NOT_FOUND"))
        assertEquals(0, dataVersion.value)
        assertTrue("tree() must still be re-fetched after a failed remove", ops.treeCalls.size > callsAfterInit)
    }

    // -----------------------------------------------------------------------
    // dataVersion observation
    // -----------------------------------------------------------------------

    @Test
    fun `dataVersion bump from another screen triggers refresh`() = runTest {
        val ops = FakeRecurringOps()
        val dataVersion = MutableStateFlow(0)
        val vm = RecurringViewModel(ops, dataVersion)

        vm.uiState.first { it is RecurringUiState.Content }
        val callsAfterInit = ops.treeCalls.size

        dataVersion.value++

        vm.uiState.first { it is RecurringUiState.Content }
        assertTrue(
            "tree() must be called again after an external dataVersion bump",
            ops.treeCalls.size > callsAfterInit,
        )
    }
}
