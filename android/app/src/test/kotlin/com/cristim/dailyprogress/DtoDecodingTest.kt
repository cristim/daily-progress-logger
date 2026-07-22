package com.cristim.dailyprogress

import com.cristim.dailyprogress.core.CoreError
import com.cristim.dailyprogress.model.BacklogDto
import com.cristim.dailyprogress.model.ConflictDto
import com.cristim.dailyprogress.model.DuePromptsDto
import com.cristim.dailyprogress.model.EveningAction
import com.cristim.dailyprogress.model.EveningDecisionDto
import com.cristim.dailyprogress.model.EveningDecisionsDto
import com.cristim.dailyprogress.model.MorningCandidateDto
import com.cristim.dailyprogress.model.MorningDecisionsDto
import com.cristim.dailyprogress.model.ProjectDto
import com.cristim.dailyprogress.model.ProjectStatus
import com.cristim.dailyprogress.model.PromptId
import com.cristim.dailyprogress.model.RecurringTemplateDto
import com.cristim.dailyprogress.model.SyncResultDto
import com.cristim.dailyprogress.model.TaskState
import com.cristim.dailyprogress.model.TreeDto
import com.cristim.dailyprogress.model.TreeProjectDto
import com.cristim.dailyprogress.model.TreeTaskDto
import kotlinx.serialization.SerializationException
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * JVM unit tests that decode captured JSON fixtures against the Kotlin DTO
 * layer, proving the decoding layer matches the frozen mobilecore wire contract.
 *
 * Fixtures mirror the JSON shapes documented in mobilecore/dto.go.
 * These tests catch contract drift: a key rename or type change in the Go code
 * will break the fixtures and fail CI before the wrong data reaches the UI.
 */
class DtoDecodingTest {

    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

    // -----------------------------------------------------------------------
    // TreeDTO
    // -----------------------------------------------------------------------

    @Test
    fun `tree with projects and unfiled tasks decodes correctly`() {
        val fixture = """
            {
              "projects": [
                {
                  "id": "proj1",
                  "name": "Work",
                  "done": false,
                  "tasks": [
                    {
                      "index": 0,
                      "depth": 0,
                      "text": "Write report",
                      "state": "todo",
                      "date": "2026-07-17",
                      "done": false,
                      "project": "Work",
                      "children": []
                    },
                    {
                      "index": 1,
                      "depth": 0,
                      "text": "Review PR",
                      "state": "done",
                      "date": "2026-07-17",
                      "done": true,
                      "project": "Work",
                      "children": []
                    }
                  ]
                }
              ],
              "unfiled": [
                {
                  "index": 2,
                  "depth": 0,
                  "text": "Buy groceries",
                  "state": "postponed",
                  "date": "2026-07-17",
                  "done": false,
                  "children": []
                }
              ],
              "recycled": [],
              "recurring": []
            }
        """.trimIndent()

        val dto = json.decodeFromString<TreeDto>(fixture)

        assertEquals(1, dto.projects.size)
        val project = dto.projects[0]
        assertEquals("proj1", project.id)
        assertEquals("Work", project.name)
        assertFalse(project.done)
        assertEquals(2, project.tasks.size)

        val task0 = project.tasks[0]
        assertEquals(0L, task0.index)
        assertEquals(0, task0.depth)
        assertEquals("Write report", task0.text)
        assertEquals(TaskState.TODO, task0.state)
        assertEquals("2026-07-17", task0.date)
        assertFalse(task0.done)

        val task1 = project.tasks[1]
        assertEquals(TaskState.DONE, task1.state)
        assertTrue(task1.done)

        assertEquals(1, dto.unfiled.size)
        val unfiled = dto.unfiled[0]
        assertEquals(2L, unfiled.index)
        assertEquals(TaskState.POSTPONED, unfiled.state)
        assertEquals("", unfiled.project) // omitempty → absent → default ""
    }

    @Test
    fun `task with children decodes subtasks`() {
        val fixture = """
            {
              "projects": [],
              "unfiled": [
                {
                  "index": 5,
                  "depth": 0,
                  "text": "Parent task",
                  "state": "todo",
                  "date": "2026-07-17",
                  "done": false,
                  "children": [
                    {
                      "index": 6,
                      "depth": 1,
                      "text": "Sub-task A",
                      "state": "done",
                      "date": "2026-07-17",
                      "done": true,
                      "children": []
                    }
                  ]
                }
              ],
              "recycled": [],
              "recurring": []
            }
        """.trimIndent()

        val dto = json.decodeFromString<TreeDto>(fixture)
        val parent = dto.unfiled[0]
        assertEquals(5L, parent.index)
        assertEquals(1, parent.children.size)
        val child = parent.children[0]
        assertEquals(6L, child.index)
        assertEquals(1, child.depth)
        assertEquals(TaskState.DONE, child.state)
    }

    @Test
    fun `empty tree decodes with empty lists`() {
        val fixture = """{"projects":[],"unfiled":[],"recycled":[],"recurring":[]}"""
        val dto = json.decodeFromString<TreeDto>(fixture)
        assertTrue(dto.projects.isEmpty())
        assertTrue(dto.unfiled.isEmpty())
        assertTrue(dto.recycled.isEmpty())
        assertTrue(dto.recurring.isEmpty())
    }

    @Test
    fun `tree ignores unknown keys`() {
        // Verify additive core changes never crash old app versions.
        val fixture = """
            {
              "projects": [],
              "unfiled": [],
              "recycled": [],
              "recurring": [],
              "future_field": "should be ignored"
            }
        """.trimIndent()
        // Should not throw.
        val dto = json.decodeFromString<TreeDto>(fixture)
        assertTrue(dto.projects.isEmpty())
    }

    // -----------------------------------------------------------------------
    // Recurring template
    // -----------------------------------------------------------------------

    @Test
    fun `recurring template from TreeJSON decodes all fields`() {
        val fixture = """
            {
              "projects": [],
              "unfiled": [],
              "recycled": [],
              "recurring": [
                {
                  "text": "Standup",
                  "project": "work",
                  "describe": "daily 09:00",
                  "kind": 0,
                  "weekday": 1,
                  "month_day": 1,
                  "hour": 9,
                  "minute": 0,
                  "raw": "Standup @daily @09:00 #work"
                }
              ]
            }
        """.trimIndent()

        val dto = json.decodeFromString<TreeDto>(fixture)
        assertEquals(1, dto.recurring.size)
        val rec = dto.recurring[0]
        assertEquals("Standup", rec.text)
        assertEquals("work", rec.project)
        assertEquals("daily 09:00", rec.describe)
        assertEquals(0, rec.kind)
        assertEquals(9, rec.hour)
        assertEquals("Standup @daily @09:00 #work", rec.raw)
    }

    @Test
    fun `recurring management view (RecurringJSON format) decodes with minimal fields`() {
        // RecurringJSON returns {text, project, raw} only; other fields default.
        val fixture = """
            [
              {"text": "Standup", "project": "", "raw": "Standup @daily"},
              {"text": "Weekly review", "project": "admin", "raw": "Weekly review @weekly @fri"}
            ]
        """.trimIndent()

        val list = json.decodeFromString<List<RecurringTemplateDto>>(fixture)
        assertEquals(2, list.size)
        assertEquals("Standup", list[0].text)
        assertEquals("", list[0].project)
        assertEquals("Standup @daily", list[0].raw)
        // Fields absent from the management payload must decode to null, not
        // ""/0 — a silent default would be indistinguishable from a real
        // kind=0 (daily) template (known-issues.md).
        assertEquals(null, list[0].describe)
        assertEquals(null, list[0].kind)
    }

    @Test
    fun `recurring template distinguishes absent kind from a real kind of 0`() {
        // Regression for the known-issues silent-default bug: before the fix,
        // both an absent kind (management shape) and an explicit kind=0
        // (daily, from TreeJSON) decoded to the same Int 0, making them
        // indistinguishable. Nullable fields must tell these apart.
        val managementShape = """[{"text": "Standup", "project": "", "raw": "Standup @daily"}]"""
        val absentKind = json.decodeFromString<List<RecurringTemplateDto>>(managementShape)[0].kind
        assertEquals(null, absentKind)

        val treeShapeDaily = """
            {
              "projects": [], "unfiled": [], "recycled": [],
              "recurring": [
                {
                  "text": "Standup", "project": "", "describe": "daily 09:00",
                  "kind": 0, "weekday": 0, "month_day": 0, "hour": 9, "minute": 0,
                  "raw": "Standup @daily @09:00"
                }
              ]
            }
        """.trimIndent()
        val realKindZero = json.decodeFromString<TreeDto>(treeShapeDaily).recurring[0].kind
        assertEquals(0, realKindZero)

        // The two must be distinguishable: one is null, the other is the Int 0.
        assertTrue(absentKind == null && realKindZero == 0)
    }

    // -----------------------------------------------------------------------
    // Backlog
    // -----------------------------------------------------------------------

    @Test
    fun `backlog decodes current and next_week lists`() {
        val fixture = """
            {
              "current": ["Write tests", "Fix bug #42"],
              "next_week": ["Plan sprint"]
            }
        """.trimIndent()

        val dto = json.decodeFromString<BacklogDto>(fixture)
        assertEquals(listOf("Write tests", "Fix bug #42"), dto.current)
        assertEquals(listOf("Plan sprint"), dto.nextWeek)
    }

    @Test
    fun `empty backlog decodes with empty lists`() {
        val fixture = """{"current":[],"next_week":[]}"""
        val dto = json.decodeFromString<BacklogDto>(fixture)
        assertTrue(dto.current.isEmpty())
        assertTrue(dto.nextWeek.isEmpty())
    }

    // -----------------------------------------------------------------------
    // Projects
    // -----------------------------------------------------------------------

    @Test
    fun `project list decodes open and closed status`() {
        val fixture = """
            [
              {"id": "abc", "name": "Work", "status": "open"},
              {"id": "xyz", "name": "Archive", "status": "closed"}
            ]
        """.trimIndent()

        val list = json.decodeFromString<List<ProjectDto>>(fixture)
        assertEquals(2, list.size)
        assertEquals(ProjectStatus.OPEN, list[0].status)
        assertEquals(ProjectStatus.CLOSED, list[1].status)
    }

    // -----------------------------------------------------------------------
    // Sync DTOs
    // -----------------------------------------------------------------------

    @Test
    fun `sync result with conflicts decodes correctly`() {
        val fixture = """
            {
              "conflicts": [
                {
                  "path": "daily/2026/07/2026-07-17.md",
                  "conflict_copy": "daily/2026/07/2026-07-17.conflict-phone.md",
                  "time": "2026-07-17T09:00:00Z"
                }
              ],
              "token": "{\"access_token\":\"ya29.xxx\"}"
            }
        """.trimIndent()

        val dto = json.decodeFromString<SyncResultDto>(fixture)
        assertEquals(1, dto.conflicts.size)
        val conflict = dto.conflicts[0]
        assertEquals("daily/2026/07/2026-07-17.md", conflict.path)
        assertEquals("daily/2026/07/2026-07-17.conflict-phone.md", conflict.conflictCopy)
        assertEquals("2026-07-17T09:00:00Z", conflict.time)
        // Token is a raw string (the OAuth JSON, not parsed).
        assertTrue(dto.token.contains("ya29.xxx"))
    }

    @Test
    fun `sync result without token defaults to empty string`() {
        val fixture = """{"conflicts":[]}"""
        val dto = json.decodeFromString<SyncResultDto>(fixture)
        assertTrue(dto.conflicts.isEmpty())
        assertEquals("", dto.token) // omitempty → absent → default ""
    }

    // -----------------------------------------------------------------------
    // DuePrompts
    // -----------------------------------------------------------------------

    @Test
    fun `due prompts decodes prompt list`() {
        val fixture = """
            {
              "due": [
                {"id": 2, "name": "morning"},
                {"id": 3, "name": "evening"}
              ]
            }
        """.trimIndent()

        val dto = json.decodeFromString<DuePromptsDto>(fixture)
        assertEquals(2, dto.due.size)
        assertEquals(2, dto.due[0].id)
        assertEquals("morning", dto.due[0].name)
    }

    @Test
    fun `no due prompts returns empty list`() {
        val fixture = """{"due":[]}"""
        val dto = json.decodeFromString<DuePromptsDto>(fixture)
        assertTrue(dto.due.isEmpty())
    }

    // -----------------------------------------------------------------------
    // TaskState enum
    // -----------------------------------------------------------------------

    @Test
    fun `all three task states deserialise correctly`() {
        fun stateFrom(wire: String): TaskState =
            json.decodeFromString<TreeTaskDto>(
                """{"index":0,"depth":0,"text":"t","state":"$wire","date":"2026-07-17","done":false,"children":[]}"""
            ).state

        assertEquals(TaskState.TODO, stateFrom("todo"))
        assertEquals(TaskState.DONE, stateFrom("done"))
        assertEquals(TaskState.POSTPONED, stateFrom("postponed"))
    }

    @Test
    fun `unknown TaskState value fails loud on decode`() {
        // Contract rule: unknown enum values must throw, never silently default.
        // iOS carries an equivalent test; this ensures Android matches the same guarantee.
        val bogusJson = """
            {"index":0,"depth":0,"text":"t","state":"bogus","date":"2026-07-17","done":false,"children":[]}
        """.trimIndent()
        try {
            json.decodeFromString<TreeTaskDto>(bogusJson)
            org.junit.Assert.fail("Expected SerializationException for unknown TaskState value but none was thrown")
        } catch (_: SerializationException) {
            // expected: kotlinx.serialization rejects unknown enum variants by default
        }
    }

    // -----------------------------------------------------------------------
    // CoreError prefix parsing
    // -----------------------------------------------------------------------

    @Test
    fun `CoreError parses CAS_MISMATCH prefix`() {
        val err = CoreError.parse("CAS_MISMATCH: tree is stale, please refresh")
        assertTrue(err is CoreError.CasMismatch)
    }

    @Test
    fun `CoreError parses NOT_FOUND prefix`() {
        val err = CoreError.parse("NOT_FOUND: item does not exist")
        assertTrue(err is CoreError.NotFound)
    }

    @Test
    fun `CoreError parses BAD_INPUT prefix`() {
        val err = CoreError.parse("BAD_INPUT: invalid date")
        assertTrue(err is CoreError.BadInput)
    }

    @Test
    fun `CoreError parses SYNC_AUTH prefix`() {
        val err = CoreError.parse("SYNC_AUTH: token expired")
        assertTrue(err is CoreError.SyncAuth)
    }

    @Test
    fun `CoreError parses INTERNAL prefix`() {
        val err = CoreError.parse("INTERNAL: unexpected nil pointer")
        assertTrue(err is CoreError.Internal)
    }

    @Test
    fun `CoreError falls back to Unknown for unrecognised prefix`() {
        val err = CoreError.parse("something unexpected happened")
        assertTrue(err is CoreError.Unknown)
    }

    @Test
    fun `CoreError raw message is preserved`() {
        val msg = "CAS_MISMATCH: tree is stale, please refresh"
        val err = CoreError.parse(msg)
        assertEquals(msg, err.raw)
    }

    // -----------------------------------------------------------------------
    // Morning check-in DTOs
    // -----------------------------------------------------------------------

    @Test
    fun `morning candidate bare array decodes with text and from_backlog`() {
        val fixture = """
            [
              {"text": "Write tests", "from_backlog": false},
              {"text": "Old backlog item", "from_backlog": true}
            ]
        """.trimIndent()
        val list = json.decodeFromString<List<MorningCandidateDto>>(fixture)
        assertEquals(2, list.size)
        assertEquals("Write tests", list[0].text)
        assertFalse(list[0].fromBacklog)
        assertEquals("Old backlog item", list[1].text)
        assertTrue(list[1].fromBacklog)
    }

    @Test
    fun `MorningDecisionsDto encodes to exact snake_case JSON`() {
        val dto = MorningDecisionsDto(
            newItems = listOf("Write tests", "Fix bug"),
            adopted = listOf(
                MorningCandidateDto(text = "Review PR", fromBacklog = false),
            ),
        )
        val encoded = json.encodeToString(dto)
        // Verify required snake_case keys are present
        assertTrue("new_items key must be present", encoded.contains("\"new_items\""))
        assertTrue("from_backlog key must be present", encoded.contains("\"from_backlog\""))
        assertTrue("adopted key must be present", encoded.contains("\"adopted\""))
        // Verify values
        assertTrue(encoded.contains("\"Write tests\""))
        assertTrue(encoded.contains("\"Fix bug\""))
        assertTrue(encoded.contains("\"Review PR\""))
    }

    @Test
    fun `MorningDecisionsDto with empty lists encodes correctly`() {
        val dto = MorningDecisionsDto(newItems = emptyList(), adopted = emptyList())
        val encoded = json.encodeToString(dto)
        assertTrue(encoded.contains("\"new_items\":[]"))
        assertTrue(encoded.contains("\"adopted\":[]"))
    }

    // -----------------------------------------------------------------------
    // Evening check-in DTOs
    // -----------------------------------------------------------------------

    @Test
    fun `EveningDecisionsDto encodes decisions with int actions and extra_done`() {
        val dto = EveningDecisionsDto(
            decisions = listOf(
                EveningDecisionDto(text = "Write tests", action = 1),   // done
                EveningDecisionDto(text = "Fix bug", action = 3),        // next_week
            ),
            extraDone = listOf("Bonus task"),
        )
        val encoded = json.encodeToString(dto)
        assertTrue("decisions key required", encoded.contains("\"decisions\""))
        assertTrue("extra_done key required", encoded.contains("\"extra_done\""))
        assertTrue("action must be int 1", encoded.contains("\"action\":1"))
        assertTrue("action must be int 3", encoded.contains("\"action\":3"))
        assertTrue("extra_done contains bonus", encoded.contains("\"Bonus task\""))
    }

    @Test
    fun `EveningDecisionsDto with all action values encodes correctly`() {
        val dto = EveningDecisionsDto(
            decisions = EveningAction.entries.map { action ->
                EveningDecisionDto(text = "task ${action.wire}", action = action.wire)
            },
            extraDone = emptyList(),
        )
        val encoded = json.encodeToString(dto)
        // All five wire values 0-4 must appear
        for (n in 0..4) {
            assertTrue("wire value $n must appear", encoded.contains("\"action\":$n"))
        }
    }

    // -----------------------------------------------------------------------
    // EveningAction enum
    // -----------------------------------------------------------------------

    @Test
    fun `EveningAction fromWire maps all valid wire values`() {
        assertEquals(EveningAction.TODO, EveningAction.fromWire(0))
        assertEquals(EveningAction.DONE, EveningAction.fromWire(1))
        assertEquals(EveningAction.NEXT_DAY, EveningAction.fromWire(2))
        assertEquals(EveningAction.NEXT_WEEK, EveningAction.fromWire(3))
        assertEquals(EveningAction.BACKLOG, EveningAction.fromWire(4))
    }

    @Test(expected = SerializationException::class)
    fun `EveningAction fromWire throws on unknown value`() {
        EveningAction.fromWire(9)
    }

    // -----------------------------------------------------------------------
    // PromptId enum
    // -----------------------------------------------------------------------

    @Test
    fun `PromptId fromWire maps all valid wire values`() {
        assertEquals(PromptId.WEEK_REVIEW, PromptId.fromWire(0))
        assertEquals(PromptId.WEEKLY_PLAN, PromptId.fromWire(1))
        assertEquals(PromptId.MORNING, PromptId.fromWire(2))
        assertEquals(PromptId.EVENING, PromptId.fromWire(3))
        assertEquals(PromptId.WEEKLY_SUMMARY, PromptId.fromWire(4))
    }

    @Test(expected = SerializationException::class)
    fun `PromptId fromWire throws on unknown value`() {
        PromptId.fromWire(9)
    }
}
