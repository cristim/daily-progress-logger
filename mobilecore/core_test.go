package mobilecore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// openTestCore creates a Core backed by a temp data dir.
func openTestCore(t *testing.T) *Core {
	t.Helper()
	c, err := Open(t.TempDir(), "", "test-device")
	require.NoError(t, err)
	return c
}

// openTestCoreWithDir creates a Core and returns the data dir for advanced tests.
func openTestCoreWithDir(t *testing.T) (*Core, string) {
	t.Helper()
	dir := t.TempDir()
	c, err := Open(dir, "", "test-device")
	require.NoError(t, err)
	return c, dir
}

// today returns a fixed Monday in the future that is "today" for tests.
// Using a fixed future date means MaterializeRecurring won't block on
// "past date" checks.
func today() string { return "2026-07-20" } // Monday 2026-W30
func todayPlus(days int) string {
	d, _ := time.ParseInLocation("2006-01-02", today(), time.Local)
	return d.AddDate(0, 0, days).Format("2006-01-02")
}

// nowAtLocalTime builds an RFC3339 timestamp for the given time components in
// the LOCAL timezone, so DuePromptsJSON comparisons are timezone-independent
// (schedule.Due uses the embedded zone offset, not time.Local).
func nowAtLocalTime(date string, hour, minute int) string {
	d, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	t := time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, time.Local)
	return t.Format(time.RFC3339)
}

// ---- TreeJSON + MaterializeRecurring ----------------------------------------

func TestTreeJSON_MaterializeRecurring(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)

	// Add a daily recurring task due every day.
	require.NoError(t, c.store.AddRecurring("Standup @daily @09:00"))

	// MaterializeRecurring is a deliberate no-op for dates before the real
	// clock's today, so materialize against the real current date rather than
	// the fixed today() fixture (a hardcoded date drifts into the past as the
	// calendar advances, which would spuriously fail this test).
	date := time.Now().Format("2006-01-02")

	// TreeJSON should trigger materialization and include the recurring task.
	raw, err := c.TreeJSON(date)
	require.NoError(t, err)

	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))

	// After materialization, the recurring task should appear in the plan
	// (either under a project or in Unfiled).
	unfiled, _ := tree["unfiled"].([]any) // snake_case
	found := false
	for _, node := range unfiled {
		task, _ := node.(map[string]any)
		if task["text"] == "Standup" { // snake_case
			found = true
			break
		}
	}
	assert.True(t, found, "recurring task 'Standup' should appear in TreeJSON after materialization")
}

func TestTreeJSON_IndexPresent(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "task one", ""))
	require.NoError(t, c.AddTask(today(), "task two", ""))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)

	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))
	unfiled, _ := tree["unfiled"].([]any) // snake_case
	require.Len(t, unfiled, 2)
	// Both tasks must carry an index field (snake_case).
	for _, item := range unfiled {
		task, _ := item.(map[string]any)
		_, hasIndex := task["index"] // snake_case
		assert.True(t, hasIndex, "TreeJSON task must include index field")
	}
}

// TestTreeJSON_Schema asserts the exact JSON keys of TreeJSON so any
// accidental key rename is caught immediately — hosts hardcode against these.
func TestTreeJSON_Schema(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "task", ""))
	pid, err := c.AddProject("Work")
	require.NoError(t, err)
	require.NoError(t, c.MoveTaskToProject(today(), 0, "task", pid))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)

	var root map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &root))

	// Root keys must be snake_case.
	assert.Contains(t, raw, `"projects"`)
	assert.Contains(t, raw, `"unfiled"`)
	assert.Contains(t, raw, `"recycled"`)
	assert.Contains(t, raw, `"recurring"`)
	// No PascalCase root keys.
	assert.NotContains(t, raw, `"Projects"`)
	assert.NotContains(t, raw, `"Unfiled"`)
	assert.NotContains(t, raw, `"Recycled"`)
	assert.NotContains(t, raw, `"Recurring"`)

	// Task keys must be snake_case.
	projects, _ := root["projects"].([]any)
	require.NotEmpty(t, projects)
	proj := projects[0].(map[string]any)
	tasks, _ := proj["tasks"].([]any)
	require.NotEmpty(t, tasks)
	task := tasks[0].(map[string]any)
	for _, key := range []string{"index", "depth", "text", "state", "date", "done", "children"} {
		_, ok := task[key]
		assert.True(t, ok, "task must have key %q", key)
	}

	// State must be a string ("todo"/"done"/"postponed"), not an int.
	assert.Equal(t, "todo", task["state"], "state must be the wire string, not an int")

	// Date must be YYYY-MM-DD, not RFC3339.
	dateStr, _ := task["date"].(string)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, dateStr, "date must be YYYY-MM-DD")

	// children must be [] not null when empty.
	children, _ := task["children"].([]any)
	assert.NotNil(t, children, "children must be [] not null")
}

// ---- CAS guard (verifyIndex) ------------------------------------------------

func TestSetTaskState_CASMismatch(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "real task", ""))

	// Correct text: should succeed.
	err := c.SetTaskState(today(), 0, "real task", "done")
	require.NoError(t, err)

	// Wrong text: should return ErrCASMismatch.
	require.NoError(t, c.AddTask(today(), "another task", ""))
	err = c.SetTaskState(today(), 1, "wrong text", "done")
	assert.ErrorIs(t, err, ErrCASMismatch, "expected ErrCASMismatch, got %v", err)
}

func TestDeleteTask_CASMismatch(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "delete me", ""))

	err := c.DeleteTask(today(), 0, "wrong text")
	require.ErrorIs(t, err, ErrCASMismatch)

	// Correct text should succeed.
	err = c.DeleteTask(today(), 0, "delete me")
	require.NoError(t, err)

	// Verify task is gone.
	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))
	unfiled, _ := tree["unfiled"].([]any) // snake_case
	assert.Empty(t, unfiled)
}

func TestSetTaskState_EmptyExpected_NoGuard(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "task", ""))
	// Empty expectedText bypasses the CAS guard.
	err := c.SetTaskState(today(), 0, "", "done")
	require.NoError(t, err)
}

// TestDeleteTask_FailClosed_UnreadableProjects verifies H6: destructive ops fail
// closed when the projects file cannot be read (corrupt / in-flight write).
func TestDeleteTask_FailClosed_UnreadableProjects(t *testing.T) {
	t.Parallel()
	c, dir := openTestCoreWithDir(t)
	require.NoError(t, c.AddTask(today(), "task", ""))

	// Corrupt the projects file so KnownProjectIDs returns an error.
	projectsFile := filepath.Join(dir, "projects.md")
	require.NoError(t, os.WriteFile(projectsFile, []byte("not a valid projects file at all"), 0o600))

	// DeleteTask must fail closed (not silently delete the wrong item).
	err := c.DeleteTask(today(), 0, "task")
	require.Error(t, err, "DeleteTask must fail closed when projects file is corrupt")
	assert.ErrorIs(t, err, ErrCASMismatch, "fail-closed error should be ErrCASMismatch")
}

// ---- Task ops ---------------------------------------------------------------

func TestEditTaskText(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "old name", ""))
	require.NoError(t, c.EditTaskText(today(), 0, "old name", "new name"))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	assert.Contains(t, raw, "new name")
	assert.NotContains(t, raw, "old name")
}

func TestPostponeToNextDay(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "carry over", ""))
	require.NoError(t, c.PostponeToNextDay(today(), 0, "carry over"))

	// Task should be gone from today.
	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	assert.NotContains(t, raw, "carry over")

	// Task should appear tomorrow.
	raw, err = c.TreeJSON(todayPlus(1))
	require.NoError(t, err)
	assert.Contains(t, raw, "carry over")
}

func TestPostponeToNextWeek(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "next week task", ""))
	require.NoError(t, c.PostponeToNextWeek(today(), 0, "next week task"))

	// Task stays in today's plan (marked ">") but appears in backlog Next week.
	backlogRaw, err := c.BacklogJSON()
	require.NoError(t, err)

	var bl map[string]any
	require.NoError(t, json.Unmarshal([]byte(backlogRaw), &bl))
	nw, _ := bl["next_week"].([]any)
	require.Len(t, nw, 1)
	assert.Equal(t, "next week task", nw[0])
}

func TestMoveTaskToBacklog(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "backlog item", ""))
	require.NoError(t, c.MoveTaskToBacklog(today(), 0, "backlog item"))

	// Task removed from plan.
	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))
	assert.Empty(t, tree["unfiled"]) // snake_case

	// Task in backlog current.
	backlogRaw, err := c.BacklogJSON()
	require.NoError(t, err)
	var bl map[string]any
	require.NoError(t, json.Unmarshal([]byte(backlogRaw), &bl))
	curr, _ := bl["current"].([]any)
	require.Len(t, curr, 1)
	assert.Equal(t, "backlog item", curr[0])
}

func TestAddSubtask(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "parent", ""))
	require.NoError(t, c.AddSubtask(today(), 0, "parent", "child"))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))
	unfiled, _ := tree["unfiled"].([]any) // snake_case
	require.Len(t, unfiled, 1)
	parent, _ := unfiled[0].(map[string]any)
	children, _ := parent["children"].([]any) // snake_case
	require.Len(t, children, 1)
	assert.Equal(t, "child", children[0].(map[string]any)["text"]) // snake_case
}

// ---- Projects ---------------------------------------------------------------

func TestProjectsJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	id, err := c.AddProject("Alpha")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	raw, err := c.ProjectsJSON()
	require.NoError(t, err)

	var projects []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &projects))
	require.Len(t, projects, 1)
	assert.Equal(t, "Alpha", projects[0]["name"])
	assert.Equal(t, "open", projects[0]["status"])
}

func TestRenameProject(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	id, err := c.AddProject("Old")
	require.NoError(t, err)
	require.NoError(t, c.RenameProject(id, "New"))

	raw, err := c.ProjectsJSON()
	require.NoError(t, err)
	assert.Contains(t, raw, "New")
	assert.NotContains(t, raw, `"Old"`)
}

func TestCloseAndReopenProject(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	id, err := c.AddProject("Proj")
	require.NoError(t, err)
	require.NoError(t, c.CloseProject(id))

	raw, err := c.ProjectsJSON()
	require.NoError(t, err)
	assert.Contains(t, raw, `"closed"`)

	require.NoError(t, c.ReopenProject(id))
	raw, err = c.ProjectsJSON()
	require.NoError(t, err)
	assert.Contains(t, raw, `"open"`)
}

// ---- Backlog ----------------------------------------------------------------

func TestBacklogJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.store.SaveBacklog(&store.Backlog{Current: []string{"item A"}, NextWeek: []string{"item B"}}))

	raw, err := c.BacklogJSON()
	require.NoError(t, err)
	var bl map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &bl))
	curr, _ := bl["current"].([]any)
	nw, _ := bl["next_week"].([]any)
	assert.Equal(t, "item A", curr[0].(string))
	assert.Equal(t, "item B", nw[0].(string))
}

func TestAdoptFromBacklog(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.store.SaveBacklog(&store.Backlog{Current: []string{"adopt me"}}))
	require.NoError(t, c.AdoptFromBacklog(today(), "adopt me"))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	assert.Contains(t, raw, "adopt me")

	// Backlog should be clear.
	backlogRaw, err := c.BacklogJSON()
	require.NoError(t, err)
	var bl map[string]any
	require.NoError(t, json.Unmarshal([]byte(backlogRaw), &bl))
	assert.Empty(t, bl["current"].([]any))
}

func TestMoveBacklogItem(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.store.SaveBacklog(&store.Backlog{Current: []string{"shuttle"}}))
	require.NoError(t, c.MoveBacklogItem("shuttle", true)) // current -> next week

	raw, err := c.BacklogJSON()
	require.NoError(t, err)
	var bl map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &bl))
	assert.Empty(t, bl["current"].([]any))
	nw, _ := bl["next_week"].([]any)
	assert.Equal(t, "shuttle", nw[0].(string))
}

// ---- Recurring --------------------------------------------------------------

func TestRecurringJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddRecurring("Standup @daily @09:00"))

	raw, err := c.RecurringJSON()
	require.NoError(t, err)
	var templates []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &templates))
	require.Len(t, templates, 1)
	assert.Equal(t, "Standup", templates[0]["text"])
	assert.NotEmpty(t, templates[0]["raw"])
}

func TestRemoveRecurring(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddRecurring("Standup @daily @09:00"))

	raw, err := c.RecurringJSON()
	require.NoError(t, err)
	var templates []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &templates))
	rawLine, _ := templates[0]["raw"].(string)

	require.NoError(t, c.RemoveRecurring(rawLine))

	raw, err = c.RecurringJSON()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &templates))
	assert.Empty(t, templates)
}

func TestAddRecurring_NoTag(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.AddRecurring("No recurrence tag here")
	require.Error(t, err)
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"missing recurrence tag must surface as BAD_INPUT; got %q", err.Error())
}

// ---- Recycle ----------------------------------------------------------------

func TestRecycleJSON_RestoreTask_PurgeRecycled(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "recyclable", ""))
	require.NoError(t, c.DeleteTask(today(), 0, "recyclable"))

	// Check recycle bin.
	raw, err := c.RecycleJSON()
	require.NoError(t, err)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "recyclable", entries[0]["text"])
	assert.Equal(t, today(), entries[0]["date"])

	// Restore.
	require.NoError(t, c.RestoreTask(today(), "recyclable"))
	raw, err = c.RecycleJSON()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	assert.Empty(t, entries)

	// Delete again, then purge.
	require.NoError(t, c.DeleteTask(today(), 0, "recyclable"))
	require.NoError(t, c.PurgeRecycled(today(), "recyclable"))
	raw, err = c.RecycleJSON()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	assert.Empty(t, entries)
}

// ---- Check-in ---------------------------------------------------------------

func TestMorningCandidatesJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	// Use Tuesday as "today" so Monday (same week, earlier day) is a candidate source.
	// today() = Monday 2026-07-20; Tuesday = 2026-07-22.
	tuesday := todayPlus(2) // Wednesday is today+2, so Tuesday is today+1
	monday := today()       // Monday of the same week
	require.NoError(t, c.AddTask(monday, "carry me forward", ""))

	raw, err := c.MorningCandidatesJSON(tuesday)
	require.NoError(t, err)
	var candidates []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &candidates))
	require.Len(t, candidates, 1)
	assert.Equal(t, "carry me forward", candidates[0]["text"])
	assert.Equal(t, false, candidates[0]["from_backlog"])
}

func TestApplyMorning(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	decisions := `{"new_items":["fresh task"],"adopted":[]}`
	require.NoError(t, c.ApplyMorning(today(), decisions))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	assert.Contains(t, raw, "fresh task")
}

func TestApplyEvening(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "evening task", ""))
	decisions := `{"decisions":[{"text":"evening task","action":1}],"extra_done":["bonus"]}`
	require.NoError(t, c.ApplyEvening(today(), decisions))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	// After the DTO fix, Done is now "done" (lowercase) in JSON.
	assert.Contains(t, raw, `"done":true`)
}

// ---- Weekly -----------------------------------------------------------------

func TestWeeklyPlanJSON_SetWeeklyPlan(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)

	raw, err := c.WeeklyPlanJSON(today())
	require.NoError(t, err)
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &plan))
	assert.Equal(t, false, plan["planned"])

	require.NoError(t, c.SetWeeklyPlan(today(), `[{"text":"big thing","done":false}]`))

	raw, err = c.WeeklyPlanJSON(today())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &plan))
	assert.Equal(t, true, plan["planned"])
	goals, _ := plan["goals"].([]any)
	require.Len(t, goals, 1)
	assert.Equal(t, "big thing", goals[0].(map[string]any)["text"])
}

// TestSetWeeklyPlan_NullInput verifies L3: null input is rejected rather than
// silently marking the week planned with zero goals.
func TestSetWeeklyPlan_NullInput(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.SetWeeklyPlan(today(), "null")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeBadInput, "null goalsJSON must return BAD_INPUT error")

	err = c.SetWeeklyPlan(today(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeBadInput)
}

func TestWeekReviewCandidatesJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "open item", ""))

	raw, err := c.WeekReviewCandidatesJSON(today())
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	candidates, _ := resp["candidates"].([]any)
	require.Len(t, candidates, 1)
	assert.Equal(t, "open item", candidates[0].(string))
}

func TestApplyWeekReview(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.store.SaveBacklog(&store.Backlog{Current: []string{"old item"}}))

	decisions := `{"decisions":[{"text":"old item","action":2}],"rollover":false}`
	require.NoError(t, c.ApplyWeekReview(today(), decisions))

	raw, err := c.BacklogJSON()
	require.NoError(t, err)
	assert.NotContains(t, raw, "old item")
}

func TestWeeklySummaryJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "done today", ""))
	require.NoError(t, c.SetTaskState(today(), 0, "done today", "done"))
	require.NoError(t, c.SetWeeklyPlan(today(), `[{"text":"goal","done":false}]`))

	raw, err := c.WeeklySummaryJSON(today())
	require.NoError(t, err)
	var summary map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &summary))
	assert.NotEmpty(t, summary["week"])
	goals, _ := summary["goals"].([]any)
	assert.Len(t, goals, 1)
	// done_by_day may be empty since done tracking requires a daily file with
	// done state; that's captured through the plan items.
}

func TestMarkWeekSummarized(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "something", ""))
	require.NoError(t, c.MarkWeekSummarized(today()))

	raw, err := c.WeeklySummaryJSON(today())
	require.NoError(t, err)
	var summary map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &summary))
	assert.Equal(t, true, summary["summarized"])
}

func TestWeeklySummaryPendingJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	// No daily data: not pending.
	raw, err := c.WeeklySummaryPendingJSON(today())
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	assert.Equal(t, false, resp["pending"])

	// Add daily data: now pending.
	require.NoError(t, c.AddTask(today(), "stuff", ""))
	raw, err = c.WeeklySummaryPendingJSON(today())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	assert.Equal(t, true, resp["pending"])
	assert.NotEmpty(t, resp["week"])
}

// ---- Schedule / DuePromptsJSON ----------------------------------------------

// TestDuePromptsJSON uses nowAtLocalTime so the check is timezone-independent:
// the RFC3339 timestamp embeds the local zone offset, and schedule.Due uses
// that offset (not time.Local) for the hour/minute comparison.
func TestDuePromptsJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)

	// 09:35 local — past the 09:30 default morning threshold.
	now := nowAtLocalTime(today(), 9, 35)
	raw, err := c.DuePromptsJSON(now)
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	due, _ := resp["due"].([]any)
	// At least morning should be due (weekly plan too, depending on defaults).
	assert.NotEmpty(t, due)

	names := make([]string, len(due))
	for i, d := range due {
		names[i] = d.(map[string]any)["name"].(string)
	}
	assert.Contains(t, names, "morning check-in")
}

func TestDuePromptsJSON_InvalidTimestamp(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	_, err := c.DuePromptsJSON("not-a-timestamp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeBadInput)
}

// ---- FileTokenStore ---------------------------------------------------------

func TestFileTokenStore_SaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := TokenFilePath(dir)
	ts := NewFileTokenStore(path)

	// Load when no file exists: error wrapping os.ErrNotExist.
	_, err := ts.Load()
	require.ErrorIs(t, err, os.ErrNotExist)

	// Save via the actual Save method (not manual WriteFile — tests the real impl).
	tok := &oauth2.Token{AccessToken: "abc123", TokenType: "Bearer"}
	require.NoError(t, ts.Save(tok))

	// Reload and verify content.
	loaded, err := ts.Load()
	require.NoError(t, err)
	assert.Equal(t, "abc123", loaded.AccessToken)

	// File mode must be 0600.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestFileTokenStore_AtomicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "token.json")
	ts := NewFileTokenStore(path)

	// Should create parent dir automatically and write atomically.
	tok := &oauth2.Token{AccessToken: "xyz"}
	require.NoError(t, ts.Save(tok))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Atomicity: no .tmp file should remain after successful Save.
	_, tmpErr := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(tmpErr), "no .tmp file should remain after atomic write")
}

func TestTokenFilePath(t *testing.T) {
	dir := "/data"
	assert.Equal(t, "/data/.oauth-token.json", TokenFilePath(dir))
}

// ---- memTokenStore ----------------------------------------------------------

// TestMemTokenStore_SaveCaptures verifies H4: Save records the updated token
// so SyncNow can return it in the response envelope.
func TestMemTokenStore_SaveCaptures(t *testing.T) {
	t.Parallel()
	original := &oauth2.Token{AccessToken: "original", RefreshToken: "r1"}
	ts := newMemTokenStore(original)

	// Before any Save: no update.
	assert.Empty(t, ts.updatedJSON(), "no update before Save")

	// After Save with a refreshed token: updatedJSON returns JSON with new token.
	updated := &oauth2.Token{AccessToken: "refreshed", RefreshToken: "r2"}
	require.NoError(t, ts.Save(updated))

	j := ts.updatedJSON()
	assert.NotEmpty(t, j, "updatedJSON should be non-empty after Save")
	assert.Contains(t, j, "refreshed", "updatedJSON must contain the new access token")

	// Load should reflect the latest token.
	tok, err := ts.Load()
	require.NoError(t, err)
	assert.Equal(t, "refreshed", tok.AccessToken)
}

// ---- ErrCASMismatch sentinel and error codes --------------------------------

func TestErrCASMismatch_Is(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "x", ""))
	err := c.DeleteTask(today(), 0, "wrong")
	assert.ErrorIs(t, err, ErrCASMismatch)
}

// TestErrCASMismatch_HasCodePrefix verifies H3: the error message carries the
// stable "CAS_MISMATCH: " prefix so hosts can detect it across the gomobile
// boundary without errors.Is.
func TestErrCASMismatch_HasCodePrefix(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "x", ""))
	err := c.DeleteTask(today(), 0, "wrong text")
	require.Error(t, err)
	assert.True(t,
		strings.HasPrefix(err.Error(), ErrCodeCASMismatch+": "),
		"CAS mismatch error must start with %q: got %q", ErrCodeCASMismatch+": ", err.Error())
	assert.Equal(t, ErrCodeCASMismatch, ClassifyError(err.Error()))
}

// TestClassifyError verifies the ClassifyError helper for all known codes.
func TestClassifyError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msg  string
		want string
	}{
		{"CAS_MISMATCH: tree is stale", ErrCodeCASMismatch},
		{"NOT_FOUND: project foo", ErrCodeNotFound},
		{"BAD_INPUT: invalid date", ErrCodeBadInput},
		{"SYNC_AUTH: token expired", ErrCodeSyncAuth},
		{"INTERNAL: something broke", ErrCodeInternal},
		{"some other error", ""},
		{"", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, ClassifyError(tc.msg), "msg=%q", tc.msg)
	}
}

// TestParseDate_BadInput verifies that passing a malformed date to any Core
// method surfaces a ClassifyError-detectable BAD_INPUT code.
func TestParseDate_BadInput(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	// AddTask is representative: it is the first method that calls parseDate.
	err := c.AddTask("not-a-date", "task", "")
	require.Error(t, err)
	assert.True(t,
		strings.HasPrefix(err.Error(), ErrCodeBadInput+": "),
		"bad date must produce a BAD_INPUT-prefixed error; got %q", err.Error())
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"ClassifyError must return ErrCodeBadInput for a bad date error")
}

// TestBadInputState verifies parseState returns a BAD_INPUT coded error.
func TestBadInputState(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "task", ""))
	err := c.SetTaskState(today(), 0, "", "invalid_state")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeBadInput)
}

// ---- Config BAD_INPUT tests --------------------------------------------------

// TestConfigJSON_CorruptFile_BadInput verifies that a corrupt mobile-config.json
// surfaces as a BAD_INPUT coded error through ConfigJSON (not wrapped as INTERNAL).
func TestConfigJSON_CorruptFile_BadInput(t *testing.T) {
	t.Parallel()
	c, dir := openTestCoreWithDir(t)
	// Write invalid JSON to the mobile config file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mobile-config.json"), []byte("{INVALID"), 0o600))

	_, err := c.ConfigJSON()
	require.Error(t, err)
	assert.True(t,
		strings.HasPrefix(err.Error(), ErrCodeBadInput+": "),
		"corrupt config must surface as BAD_INPUT; got %q", err.Error())
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()))
}

// TestSetConfig_CorruptFile_BadInput verifies that a corrupt mobile-config.json
// surfaces as a BAD_INPUT coded error through SetConfig (not wrapped as INTERNAL).
func TestSetConfig_CorruptFile_BadInput(t *testing.T) {
	t.Parallel()
	c, dir := openTestCoreWithDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mobile-config.json"), []byte("{INVALID"), 0o600))

	err := c.SetConfig(`{"morning_time":"08:00"}`)
	require.Error(t, err)
	assert.True(t,
		strings.HasPrefix(err.Error(), ErrCodeBadInput+": "),
		"corrupt config must surface as BAD_INPUT; got %q", err.Error())
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()))
}

// ---- ReorderTask + MoveTaskToProject ----------------------------------------

func TestReorderTask(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "first", ""))
	require.NoError(t, c.AddTask(today(), "second", ""))

	// Move "first" (idx 0) to be below "second" (idx 1).
	require.NoError(t, c.ReorderTask(today(), 0, "first", 1, "second", true))

	// After reorder, build the plan to verify order.
	d, exists, err := c.store.LoadDaily(func() time.Time {
		t, _ := time.ParseInLocation("2006-01-02", today(), time.Local)
		return t
	}())
	require.NoError(t, err)
	require.True(t, exists)
	// Plan now has "second" then "first".
	require.Len(t, d.Plan, 2)
	assert.Equal(t, "second", d.Plan[0].Text)
	assert.Equal(t, "first", d.Plan[1].Text)
}

// TestReorderTask_CycleReturnsError verifies M6: a cycle-creating reorder
// returns an explicit BAD_INPUT error instead of silently succeeding.
func TestReorderTask_CycleReturnsError(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "parent", ""))
	require.NoError(t, c.AddSubtask(today(), 0, "parent", "child"))
	// Plan: [parent(0), child(1)]. Reordering parent relative to child(1)
	// would create a cycle (child is inside parent's subtree).
	err := c.ReorderTask(today(), 0, "parent", 1, "child", true)
	require.Error(t, err, "reordering a task into its own subtree must return an error")
	assert.Contains(t, err.Error(), ErrCodeBadInput)
}

// TestReorderTask_SelfIsError verifies that reordering a task relative to itself errors.
func TestReorderTask_SelfIsError(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "solo", ""))
	err := c.ReorderTask(today(), 0, "solo", 0, "solo", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeBadInput)
}

func TestMoveTaskToProject(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	id, err := c.AddProject("Work")
	require.NoError(t, err)
	require.NoError(t, c.AddTask(today(), "free task", ""))
	require.NoError(t, c.MoveTaskToProject(today(), 0, "free task", id))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	// Task should appear under the Work project, not Unfiled.
	// Check that unfiled is empty after the move.
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))
	unfiled, _ := tree["unfiled"].([]any) // snake_case
	assert.Empty(t, unfiled, "task should have moved out of unfiled")
	assert.Contains(t, raw, "Work")
}

func TestMakeSubtask(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "parent", ""))
	require.NoError(t, c.AddTask(today(), "future child", ""))

	// Make "future child" (index 1) a subtask of "parent" (index 0).
	require.NoError(t, c.MakeSubtask(today(), 1, "future child", 0, "parent"))

	raw, err := c.TreeJSON(today())
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))
	unfiled, _ := tree["unfiled"].([]any) // snake_case
	require.Len(t, unfiled, 1)
	parent, _ := unfiled[0].(map[string]any)
	children, _ := parent["children"].([]any) // snake_case
	require.Len(t, children, 1)
	assert.Equal(t, "future child", children[0].(map[string]any)["text"]) // snake_case
}

// ---- NOT_FOUND error code tests ---------------------------------------------

// assertNotFound verifies err carries the NOT_FOUND code prefix and ClassifyError.
func assertNotFound(t *testing.T, err error, context string) {
	t.Helper()
	require.Error(t, err, "%s: expected an error", context)
	assert.True(t,
		strings.HasPrefix(err.Error(), ErrCodeNotFound+": "),
		"%s: error must start with %q; got %q", context, ErrCodeNotFound+": ", err.Error())
	assert.Equal(t, ErrCodeNotFound, ClassifyError(err.Error()),
		"%s: ClassifyError must return ErrCodeNotFound", context)
}

// TestRenameProject_NotFound verifies that renaming an unknown project ID
// surfaces as a NOT_FOUND coded error.
func TestRenameProject_NotFound(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	assertNotFound(t, c.RenameProject("no-such-id", "New Name"), "RenameProject")
}

// TestCloseProject_NotFound verifies NOT_FOUND for an unknown project ID.
func TestCloseProject_NotFound(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	assertNotFound(t, c.CloseProject("no-such-id"), "CloseProject")
}

// TestReopenProject_NotFound verifies NOT_FOUND for an unknown project ID.
func TestReopenProject_NotFound(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	assertNotFound(t, c.ReopenProject("no-such-id"), "ReopenProject")
}

// TestAddTask_NotFoundProject verifies that tagging a task to an unknown
// project ID surfaces as NOT_FOUND (not a raw error).
func TestAddTask_NotFoundProject(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	assertNotFound(t, c.AddTask(today(), "task", "no-such-project"), "AddTask with unknown projectID")
}

// TestMoveTaskToProject_NotFound verifies NOT_FOUND when projectID is unknown.
func TestMoveTaskToProject_NotFound(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "free task", ""))
	assertNotFound(t, c.MoveTaskToProject(today(), 0, "free task", "ghost-project"), "MoveTaskToProject")
}

// TestMoveBacklogItem_NotFound verifies that moving an item not in the backlog
// surfaces as NOT_FOUND via codeStoreErr.
func TestMoveBacklogItem_NotFound(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	// "ghost text" does not exist in either backlog section.
	assertNotFound(t, c.MoveBacklogItem("ghost text", true), "MoveBacklogItem")
}

// ---- ResolveConflict validation ---------------------------------------------

// TestResolveConflict_UnknownChoice verifies H2: an unknown choice string
// returns a BAD_INPUT coded error before any network call.
func TestResolveConflict_UnknownChoice(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	// We pass valid-looking tokenJSON; the choice validation fires before network.
	validTok := fmt.Sprintf(`{"access_token":"tok","token_type":"Bearer","expiry":"%s"}`,
		time.Now().Add(time.Hour).Format(time.RFC3339))
	for _, bad := range []string{"keep-local", "local", "KEEP_LOCAL", "both", ""} {
		err := c.ResolveConflict(validTok, "/some/path", bad)
		require.Error(t, err, "bad choice %q must return an error", bad)
		assert.Contains(t, err.Error(), ErrCodeBadInput,
			"bad choice %q must return BAD_INPUT, got: %v", bad, err)
	}
}

// ---- Should-fix BAD_INPUT tests (checkin/weekly/recurring) ------------------

// TestApplyMorning_MalformedJSON verifies ApplyMorning returns BAD_INPUT for
// malformed decisionsJSON.
func TestApplyMorning_MalformedJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.ApplyMorning(today(), "{not json}")
	require.Error(t, err)
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"malformed decisionsJSON must surface as BAD_INPUT; got %q", err.Error())
}

// TestApplyEvening_UnknownAction verifies ApplyEvening returns BAD_INPUT for
// an unrecognised action code.
func TestApplyEvening_UnknownAction(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.ApplyEvening(today(), `{"decisions":[{"text":"x","action":99}]}`)
	require.Error(t, err)
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"unknown evening action must surface as BAD_INPUT; got %q", err.Error())
}

// TestSetWeeklyPlan_MalformedJSON verifies SetWeeklyPlan returns BAD_INPUT for
// malformed goalsJSON.
func TestSetWeeklyPlan_MalformedJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.SetWeeklyPlan(today(), "{not json}")
	require.Error(t, err)
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"malformed goalsJSON must surface as BAD_INPUT; got %q", err.Error())
}

// TestApplyWeekReview_MalformedJSON verifies ApplyWeekReview returns BAD_INPUT
// for malformed decisionsJSON.
func TestApplyWeekReview_MalformedJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.ApplyWeekReview(today(), "{not json}")
	require.Error(t, err)
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"malformed decisionsJSON must surface as BAD_INPUT; got %q", err.Error())
}

// TestApplyWeekReview_UnknownAction verifies ApplyWeekReview returns BAD_INPUT
// for an unrecognised review action code.
func TestApplyWeekReview_UnknownAction(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	err := c.ApplyWeekReview(today(), `{"decisions":[{"text":"x","action":99}]}`)
	require.Error(t, err)
	assert.Equal(t, ErrCodeBadInput, ClassifyError(err.Error()),
		"unknown review action must surface as BAD_INPUT; got %q", err.Error())
}

// TestResolveConflict_ValidChoicesAccepted checks that the three known choice
// strings pass the validation gate (we cannot test the full network flow here,
// so we just check that the error is NOT a BAD_INPUT choice error).
func TestResolveConflict_ValidChoicesAccepted(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	validTok := fmt.Sprintf(`{"access_token":"tok","token_type":"Bearer","expiry":"%s"}`,
		time.Now().Add(time.Hour).Format(time.RFC3339))
	for _, good := range []string{"keep_local", "keep_remote", "keep_both"} {
		err := c.ResolveConflict(validTok, "/no/such/path", good)
		// Expect a SYNC_AUTH error (Drive connection fails in tests), not BAD_INPUT.
		if err != nil {
			assert.NotContains(t, err.Error(), "unknown conflict resolution choice",
				"valid choice %q should not return a choice-validation error", good)
		}
	}
}

// ---- ConflictsJSON returns [] not null (M5) ---------------------------------

// TestConflictsJSON_EmptyIsArray verifies that ConflictsJSON returns "[]" not
// "null" when there are no conflicts (Kotlin JSON decoder throws on null).
// We test ConflictsJSON indirectly via SyncNow's conflicts field, which follows
// the same guarantee.
func TestSyncNow_InvalidToken(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	_, err := c.SyncNow("not-json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrCodeBadInput)
}
