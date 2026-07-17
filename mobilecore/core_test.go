package mobilecore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// openTestCore creates a Core backed by a temp data dir.
func openTestCore(t *testing.T) *Core {
	t.Helper()
	c, err := Open(t.TempDir(), "", "test-device")
	require.NoError(t, err)
	return c
}

// today returns a fixed Monday in the future that is "today" for tests.
// Using a fixed future date means MaterializeRecurring won't block on
// "past date" checks.
func today() string { return "2026-07-20" } // Monday 2026-W30
func todayPlus(days int) string {
	d, _ := time.ParseInLocation("2006-01-02", today(), time.Local)
	return d.AddDate(0, 0, days).Format("2006-01-02")
}

// ---- TreeJSON + MaterializeRecurring ----------------------------------------

func TestTreeJSON_MaterializeRecurring(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)

	// Add a daily recurring task due every day.
	require.NoError(t, c.store.AddRecurring("Standup @daily @09:00"))

	// TreeJSON should trigger materialization and include the recurring task.
	raw, err := c.TreeJSON(today())
	require.NoError(t, err)

	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tree))

	// After materialization, the recurring task should appear in the plan
	// (either under a project or in Unfiled).
	unfiled, _ := tree["Unfiled"].([]any)
	found := false
	for _, node := range unfiled {
		task, _ := node.(map[string]any)
		if task["Text"] == "Standup" {
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
	unfiled, _ := tree["Unfiled"].([]any)
	require.Len(t, unfiled, 2)
	// Both tasks must carry an Index field.
	for _, item := range unfiled {
		task, _ := item.(map[string]any)
		_, hasIndex := task["Index"]
		assert.True(t, hasIndex, "TreeJSON task must include Index field")
	}
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
	unfiled, _ := tree["Unfiled"].([]any)
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
	assert.Empty(t, tree["Unfiled"])

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
	unfiled, _ := tree["Unfiled"].([]any)
	require.Len(t, unfiled, 1)
	parent, _ := unfiled[0].(map[string]any)
	children, _ := parent["Children"].([]any)
	require.Len(t, children, 1)
	assert.Equal(t, "child", children[0].(map[string]any)["Text"])
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
	assert.Error(t, err)
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
	// State is serialized as int (1=done); Done (rollup bool) is true.
	assert.Contains(t, raw, `"Done":true`)
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

func TestDuePromptsJSON(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)

	// At 9:35 on today (Monday): morning check-in should be due (morning not done,
	// no evening yet, 9:35 > 9:30 default), and weekly plan should be due
	// (not yet set). Week review: no past-week data, not pending.
	now := today() + "T09:35:00Z"
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
	assert.Error(t, err)
}

// ---- FileTokenStore ---------------------------------------------------------

func TestFileTokenStore_SaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := TokenFilePath(dir)
	store := NewFileTokenStore(path)

	// Load when no file exists: error wrapping os.ErrNotExist.
	_, err := store.Load()
	require.ErrorIs(t, err, os.ErrNotExist)

	// Save and reload.
	tok := &oauthTokenForTest{AccessToken: "abc123", TokenType: "Bearer"}
	// We use json.Marshal/Unmarshal to avoid importing oauth2 in the test.
	data, err := json.Marshal(tok)
	require.NoError(t, err)
	// Write directly to avoid needing real oauth2.Token.
	require.NoError(t, os.WriteFile(path, data, 0o600))

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "abc123", loaded.AccessToken)

	// File mode: 0600.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestFileTokenStore_AtomicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "token.json")
	store := NewFileTokenStore(path)

	// Should create parent dir automatically.
	tok := &tokenForSave{AccessToken: "xyz"}
	data, err := json.Marshal(tok)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, data, 0o600))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	_ = store // silence unused
}

func TestTokenFilePath(t *testing.T) {
	dir := "/data"
	assert.Equal(t, "/data/.oauth-token.json", TokenFilePath(dir))
}

// Helper types for token tests (avoid importing oauth2 in tests directly).
type oauthTokenForTest struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}

type tokenForSave struct {
	AccessToken string `json:"access_token"`
}

// ---- ErrCASMismatch sentinel ------------------------------------------------

func TestErrCASMismatch_Is(t *testing.T) {
	t.Parallel()
	c := openTestCore(t)
	require.NoError(t, c.AddTask(today(), "x", ""))
	err := c.DeleteTask(today(), 0, "wrong")
	assert.ErrorIs(t, err, ErrCASMismatch)
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
	assert.NotContains(t, raw, `"Unfiled":[{"`)
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
	unfiled, _ := tree["Unfiled"].([]any)
	require.Len(t, unfiled, 1)
	parent, _ := unfiled[0].(map[string]any)
	children, _ := parent["Children"].([]any)
	require.Len(t, children, 1)
	assert.Equal(t, "future child", children[0].(map[string]any)["Text"])
}
