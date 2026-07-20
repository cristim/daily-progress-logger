package com.cristim.dailyprogress

import com.cristim.dailyprogress.model.DayDoneDto
import com.cristim.dailyprogress.model.PendingWeekDto
import com.cristim.dailyprogress.model.ReviewAction
import com.cristim.dailyprogress.model.ReviewDecisionDto
import com.cristim.dailyprogress.model.ReviewDecisionsDto
import com.cristim.dailyprogress.model.WeekReviewCandidatesDto
import com.cristim.dailyprogress.model.WeeklyGoalDto
import com.cristim.dailyprogress.model.WeeklyPlanDto
import com.cristim.dailyprogress.model.WeeklySummaryDto
import com.cristim.dailyprogress.util.isoWeekToMonday
import kotlinx.serialization.SerializationException
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.DayOfWeek
import java.time.LocalDate

/**
 * JVM unit tests for weekly DTOs, ISO-week helper, and ReviewAction enum.
 * Mirrors iOS §8 test requirements exactly (same fixtures, same edge weeks).
 */
class WeekTest {

    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

    // -----------------------------------------------------------------------
    // WeeklyPlan
    // -----------------------------------------------------------------------

    @Test
    fun `WeeklyPlanDto decodes planned true with goals`() {
        val fixture = """
            {
              "week": "2026-W29",
              "planned": true,
              "goals": [
                {"text": "Ship mobile core", "done": false},
                {"text": "Write tests", "done": true}
              ]
            }
        """.trimIndent()
        val dto = json.decodeFromString<WeeklyPlanDto>(fixture)
        assertEquals("2026-W29", dto.week)
        assertTrue(dto.planned)
        assertEquals(2, dto.goals.size)
        assertEquals("Ship mobile core", dto.goals[0].text)
        assertFalse(dto.goals[0].done)
        assertEquals("Write tests", dto.goals[1].text)
        assertTrue(dto.goals[1].done)
    }

    @Test
    fun `WeeklyPlanDto decodes planned false with empty goals`() {
        val fixture = """{"week":"2026-W29","planned":false,"goals":[]}"""
        val dto = json.decodeFromString<WeeklyPlanDto>(fixture)
        assertFalse(dto.planned)
        assertTrue(dto.goals.isEmpty())
    }

    @Test
    fun `WeeklyPlanDto goals absent decodes as empty list`() {
        // omitempty in Go: goals key may be absent when the array is empty
        val fixture = """{"week":"2026-W29","planned":false}"""
        val dto = json.decodeFromString<WeeklyPlanDto>(fixture)
        assertTrue(dto.goals.isEmpty())
    }

    @Test
    fun `WeeklyGoalDto list encodes to correct JSON for SetWeeklyPlan`() {
        val goals = listOf(
            WeeklyGoalDto(text = "Ship it", done = false),
            WeeklyGoalDto(text = "Done already", done = true),
        )
        val encoded = json.encodeToString(goals)
        // Must be a JSON array with both entries
        assertTrue("must be a JSON array", encoded.startsWith("["))
        assertTrue(encoded.contains("\"text\":\"Ship it\""))
        assertTrue(encoded.contains("\"text\":\"Done already\""))
        // done=true must always appear; done=false may be omitted (Go default is false)
        assertTrue(encoded.contains("\"done\":true"))
        // Round-trip: decoding the encoded value must restore both goals exactly
        val decoded = json.decodeFromString<List<WeeklyGoalDto>>(encoded)
        assertEquals(2, decoded.size)
        assertFalse("Ship it goal must be not done", decoded[0].done)
        assertTrue("Done already goal must be done", decoded[1].done)
    }

    @Test
    fun `empty WeeklyGoalDto list encodes to empty array, never null`() {
        // Contract: SetWeeklyPlan must always receive [] not "" or "null"
        val encoded = json.encodeToString(emptyList<WeeklyGoalDto>())
        assertEquals("[]", encoded)
    }

    // -----------------------------------------------------------------------
    // WeeklySummary
    // -----------------------------------------------------------------------

    @Test
    fun `WeeklySummaryDto decodes with done days`() {
        val fixture = """
            {
              "week": "2026-W29",
              "start": "2026-07-13",
              "end": "2026-07-19",
              "summarized": false,
              "reviewed": true,
              "goals": [{"text": "Ship it", "done": true}],
              "done_by_day": [
                {"date": "2026-07-14", "items": ["Task A", "Task B"]},
                {"date": "2026-07-15", "items": ["Task C"]}
              ]
            }
        """.trimIndent()
        val dto = json.decodeFromString<WeeklySummaryDto>(fixture)
        assertEquals("2026-W29", dto.week)
        assertEquals("2026-07-13", dto.start)
        assertFalse(dto.summarized)
        assertTrue(dto.reviewed)
        assertEquals(1, dto.goals.size)
        assertTrue(dto.goals[0].done)
        assertEquals(2, dto.doneByDay.size)
        assertEquals("2026-07-14", dto.doneByDay[0].date)
        assertEquals(listOf("Task A", "Task B"), dto.doneByDay[0].items)
    }

    @Test
    fun `WeeklySummaryDto decodes with zero done days (empty state)`() {
        val fixture = """
            {
              "week":"2026-W29",
              "start":"2026-07-13",
              "end":"2026-07-19",
              "summarized":false,
              "reviewed":false,
              "goals":[],
              "done_by_day":[]
            }
        """.trimIndent()
        val dto = json.decodeFromString<WeeklySummaryDto>(fixture)
        assertTrue(dto.goals.isEmpty())
        assertTrue(dto.doneByDay.isEmpty())
    }

    @Test
    fun `DayDoneDto items absent decodes as empty list`() {
        val fixture = """{"date":"2026-07-14"}"""
        val dto = json.decodeFromString<DayDoneDto>(fixture)
        assertTrue(dto.items.isEmpty())
    }

    // -----------------------------------------------------------------------
    // PendingWeek (shared by WeeklySummaryPendingJSON and UnreviewedWeekJSON)
    // -----------------------------------------------------------------------

    @Test
    fun `PendingWeekDto pending true carries week label`() {
        val fixture = """{"pending":true,"week":"2026-W28"}"""
        val dto = json.decodeFromString<PendingWeekDto>(fixture)
        assertTrue(dto.pending)
        assertEquals("2026-W28", dto.week)
    }

    @Test
    fun `PendingWeekDto pending false has absent week field`() {
        // Go omits "week" when pending=false (omitempty). Default "" is fine
        // because consumers only read it when pending=true.
        val fixture = """{"pending":false}"""
        val dto = json.decodeFromString<PendingWeekDto>(fixture)
        assertFalse(dto.pending)
        // Default value — consumers must check pending before reading week.
        assertEquals("", dto.week)
    }

    // -----------------------------------------------------------------------
    // WeekReviewCandidates
    // -----------------------------------------------------------------------

    @Test
    fun `WeekReviewCandidatesDto decodes week and candidates`() {
        val fixture = """
            {
              "week": "2026-W28",
              "candidates": ["Old task A", "Old task B"]
            }
        """.trimIndent()
        val dto = json.decodeFromString<WeekReviewCandidatesDto>(fixture)
        assertEquals("2026-W28", dto.week)
        assertEquals(listOf("Old task A", "Old task B"), dto.candidates)
    }

    @Test
    fun `WeekReviewCandidatesDto empty candidates decodes correctly`() {
        val fixture = """{"week":"2026-W28","candidates":[]}"""
        val dto = json.decodeFromString<WeekReviewCandidatesDto>(fixture)
        assertTrue(dto.candidates.isEmpty())
    }

    // -----------------------------------------------------------------------
    // ReviewDecisions (sent to ApplyWeekReview)
    // -----------------------------------------------------------------------

    @Test
    fun `ReviewDecisionsDto encodes to exact JSON wire shape`() {
        val payload = ReviewDecisionsDto(
            decisions = listOf(
                ReviewDecisionDto(text = "Keep this", action = ReviewAction.KEEP.wire),
                ReviewDecisionDto(text = "Postpone that", action = ReviewAction.POSTPONE.wire),
                ReviewDecisionDto(text = "Drop it", action = ReviewAction.DROP.wire),
            ),
            rollover = true,
        )
        val encoded = json.encodeToString(payload)
        assertTrue(encoded.contains("\"decisions\""))
        assertTrue(encoded.contains("\"rollover\":true"))
        assertTrue(encoded.contains("\"action\":0"))  // KEEP
        assertTrue(encoded.contains("\"action\":1"))  // POSTPONE
        assertTrue(encoded.contains("\"action\":2"))  // DROP
        assertTrue(encoded.contains("\"text\":\"Keep this\""))
    }

    @Test
    fun `ReviewDecisionsDto rollover false encodes correctly`() {
        val payload = ReviewDecisionsDto(decisions = emptyList(), rollover = false)
        val encoded = json.encodeToString(payload)
        assertTrue(encoded.contains("\"rollover\":false"))
        assertTrue(encoded.contains("\"decisions\":[]"))
    }

    // -----------------------------------------------------------------------
    // ReviewAction enum — fail-loud fromWire
    // -----------------------------------------------------------------------

    @Test
    fun `ReviewAction fromWire maps all valid wire values`() {
        assertEquals(ReviewAction.KEEP, ReviewAction.fromWire(0))
        assertEquals(ReviewAction.POSTPONE, ReviewAction.fromWire(1))
        assertEquals(ReviewAction.DROP, ReviewAction.fromWire(2))
    }

    @Test(expected = SerializationException::class)
    fun `ReviewAction fromWire throws on unknown value`() {
        ReviewAction.fromWire(9)
    }

    @Test(expected = SerializationException::class)
    fun `ReviewAction fromWire throws on negative value`() {
        ReviewAction.fromWire(-1)
    }

    // -----------------------------------------------------------------------
    // isoWeekToMonday — ISO-8601 week to Monday helper
    // -----------------------------------------------------------------------

    @Test
    fun `isoWeekToMonday returns Monday for a mid-year week`() {
        // 2026-W29: July 13 is Monday
        val monday = isoWeekToMonday("2026-W29")
        assertEquals(DayOfWeek.MONDAY, monday.dayOfWeek)
        assertEquals(LocalDate.of(2026, 7, 13), monday)
    }

    @Test
    fun `isoWeekToMonday handles year-boundary week (2026-W01 starts in 2025)`() {
        // 2026-W01: Jan 1 2026 is Thursday, so week 1 starts Mon Dec 29 2025.
        val monday = isoWeekToMonday("2026-W01")
        assertEquals(DayOfWeek.MONDAY, monday.dayOfWeek)
        assertEquals(LocalDate.of(2025, 12, 29), monday)
    }

    @Test
    fun `isoWeekToMonday handles 53-week year (2015-W53)`() {
        // 2015-W53: Dec 28, 2015 is Monday (2015 is a 53-week year)
        val monday = isoWeekToMonday("2015-W53")
        assertEquals(DayOfWeek.MONDAY, monday.dayOfWeek)
        assertEquals(LocalDate.of(2015, 12, 28), monday)
    }

    @Test
    fun `isoWeekToMonday handles week 1 of year where Jan 1 is in previous week-year`() {
        // 2025-W01: Jan 1 2025 is Wednesday, so week 1 starts Mon Dec 30 2024.
        val monday = isoWeekToMonday("2025-W01")
        assertEquals(DayOfWeek.MONDAY, monday.dayOfWeek)
        assertEquals(LocalDate.of(2024, 12, 30), monday)
    }

    @Test(expected = IllegalArgumentException::class)
    fun `isoWeekToMonday throws on invalid format`() {
        isoWeekToMonday("not-a-week")
    }

    @Test
    fun `isoWeekToMonday result is always a Monday`() {
        // Spot-check a range of weeks
        val weeks = listOf("2026-W01", "2026-W10", "2026-W52", "2025-W53", "2024-W01")
        weeks.forEach { week ->
            val monday = isoWeekToMonday(week)
            assertEquals(
                "Expected Monday for $week but got ${monday.dayOfWeek}",
                DayOfWeek.MONDAY,
                monday.dayOfWeek,
            )
        }
    }

    // -----------------------------------------------------------------------
    // Goals rebuild preserves order and states
    // -----------------------------------------------------------------------

    @Test
    fun `goals array rebuild preserves order and mutated done state`() {
        val original = listOf(
            WeeklyGoalDto(text = "A", done = false),
            WeeklyGoalDto(text = "B", done = false),
            WeeklyGoalDto(text = "C", done = true),
        )
        // Simulate setGoalDone(1, true)
        val updated = original.toMutableList()
            .also { it[1] = it[1].copy(done = true) }
        assertEquals("A", updated[0].text)
        assertFalse(updated[0].done)
        assertEquals("B", updated[1].text)
        assertTrue(updated[1].done) // mutated
        assertEquals("C", updated[2].text)
        assertTrue(updated[2].done) // unchanged
    }

    @Test
    fun `goals with duplicate texts are distinguishable only by index`() {
        // Duplicate goal texts are legal (per plan §6 and Qt parity).
        // The UI uses index as the stable key, not text.
        val goals = listOf(
            WeeklyGoalDto(text = "Same", done = false),
            WeeklyGoalDto(text = "Same", done = true),
        )
        assertEquals("Same", goals[0].text)
        assertEquals("Same", goals[1].text)
        assertFalse(goals[0].done)
        assertTrue(goals[1].done)
        // Encoding must produce both entries; round-trip restores both correctly.
        val encoded = json.encodeToString(goals)
        assertTrue("must contain done:true", encoded.contains("\"done\":true"))
        val decoded = json.decodeFromString<List<WeeklyGoalDto>>(encoded)
        assertEquals(2, decoded.size)
        assertFalse("first goal (done=false) must decode correctly", decoded[0].done)
        assertTrue("second goal (done=true) must decode correctly", decoded[1].done)
    }
}
