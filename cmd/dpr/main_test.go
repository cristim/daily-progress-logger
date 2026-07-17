package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cristim/daily-progress-logger/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDate is a fixed date used in tests so output is deterministic.
var testDate = time.Date(2025, 6, 9, 0, 0, 0, 0, time.Local) // Monday

// newTestStore creates a store backed by a temporary directory.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	return st
}

// runCmd runs a command function and returns its stdout.
func runCmd(t *testing.T, fn func(w *bytes.Buffer) error) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, fn(&buf))
	return buf.String()
}

// runCmdErr runs a command function discarding stdout, returning only the error.
func runCmdErr(fn func(w *bytes.Buffer) error) error {
	var buf bytes.Buffer
	return fn(&buf)
}

// TestAddAndList verifies that adding tasks makes them appear in list output.
func TestAddAndList(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"First task"}, &bytes.Buffer{}))
	require.NoError(t, cmdAdd(st, date, []string{"Second task"}, &bytes.Buffer{}))

	output := runCmd(t, func(w *bytes.Buffer) error { return cmdList(st, date, nil, w) })
	assert.Contains(t, output, "  1 [ ] First task")
	assert.Contains(t, output, "  2 [ ] Second task")
}

// TestDoneMarking verifies done and undone toggling.
func TestDoneMarking(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Task A"}, &bytes.Buffer{}))
	require.NoError(t, cmdAdd(st, date, []string{"Task B"}, &bytes.Buffer{}))

	// Mark first done.
	output := runCmd(t, func(w *bytes.Buffer) error { return cmdDone(st, date, []string{"1"}, w) })
	assert.Contains(t, output, "[x] Task A")
	assert.Contains(t, output, "[ ] Task B")

	// Mark it undone again.
	output = runCmd(t, func(w *bytes.Buffer) error { return cmdUndone(st, date, []string{"1"}, w) })
	assert.Contains(t, output, "[ ] Task A")
}

// TestEditText verifies that edit replaces item text.
func TestEditText(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Original text"}, &bytes.Buffer{}))

	output := runCmd(t, func(w *bytes.Buffer) error {
		return cmdEdit(st, date, []string{"1", "Updated", "text"}, w)
	})
	assert.Contains(t, output, "Updated text")
	assert.NotContains(t, output, "Original text")
}

// TestRmMovesToRecycle verifies that rm removes from the plan (not permanently).
func TestRmMovesToRecycle(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Keep me"}, &bytes.Buffer{}))
	require.NoError(t, cmdAdd(st, date, []string{"Delete me"}, &bytes.Buffer{}))

	var buf bytes.Buffer
	require.NoError(t, cmdRm(st, date, []string{"2"}, &buf))
	output := buf.String()

	// rm prints a confirmation mentioning the item name and recycle bin.
	assert.Contains(t, output, "recycle bin")
	assert.Contains(t, output, "Delete me") // appears in the confirmation line
	// The updated plan list must only show the surviving item.
	assert.Contains(t, output, "Keep me")

	// Recycle bin must contain the deleted item.
	bin, err := st.LoadRecycleBin()
	require.NoError(t, err)
	require.Len(t, bin, 1)
	assert.Equal(t, "Delete me", bin[0].Item.Text)
}

// TestPostponeWeek verifies that --week marks the item postponed ([>]).
func TestPostponeWeek(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Carry me forward"}, &bytes.Buffer{}))

	output := runCmd(t, func(w *bytes.Buffer) error {
		return cmdPostpone(st, date, []string{"--week", "1"}, w)
	})
	assert.Contains(t, output, "[>] Carry me forward")

	// Must also appear in next week's backlog.
	b, err := st.LoadBacklog()
	require.NoError(t, err)
	require.Len(t, b.NextWeek, 1)
	assert.Equal(t, "Carry me forward", b.NextWeek[0])
}

// TestPostponeNextDay verifies default postpone moves to tomorrow.
func TestPostponeNextDay(t *testing.T) {
	st := newTestStore(t)
	date := testDate
	tomorrow := date.AddDate(0, 0, 1)

	require.NoError(t, cmdAdd(st, date, []string{"Move to tomorrow"}, &bytes.Buffer{}))
	var buf bytes.Buffer
	require.NoError(t, cmdPostpone(st, date, []string{"1"}, &buf))
	assert.Contains(t, buf.String(), tomorrow.Format(dateFormat))

	// Should be gone from today.
	items, err := loadPlan(st, date)
	require.NoError(t, err)
	assert.Empty(t, items)

	// Should appear in tomorrow's plan.
	items, err = loadPlan(st, tomorrow)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Move to tomorrow", items[0].Text)
}

// TestAddProject verifies that project add creates a project and prints its id.
func TestAddProject(t *testing.T) {
	st := newTestStore(t)

	output := runCmd(t, func(w *bytes.Buffer) error {
		return cmdProject(st, []string{"add", "My Project"}, w)
	})
	assert.Contains(t, output, "my-project")

	projects, err := st.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "My Project", projects[0].Name)
	assert.Equal(t, "my-project", projects[0].ID)
}

// TestAddTaggedTask verifies --project tags the task.
func TestAddTaggedTask(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	_, err := st.AddProject("Ship It")
	require.NoError(t, err)

	output := runCmd(t, func(w *bytes.Buffer) error {
		return cmdAdd(st, date, []string{"--project", "ship-it", "Write release notes"}, w)
	})
	assert.Contains(t, output, "#ship-it")
	assert.Contains(t, output, "Write release notes")
}

// TestAddSubtask verifies --parent adds an indented subtask.
func TestAddSubtask(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Parent task"}, &bytes.Buffer{}))
	output := runCmd(t, func(w *bytes.Buffer) error {
		return cmdAdd(st, date, []string{"--parent", "1", "Child task"}, w)
	})
	// Child should appear indented (2 spaces per depth level) below parent.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "Parent task")
	assert.Contains(t, lines[1], "  [ ] Child task") // 2-space indent at depth 1
}

// TestAddFlagsAfterText is the regression guard for the bug where stdlib
// flag.Parse stopped at the first positional token, so "add <text> --project X"
// swallowed "--project X" into the task text (creating an untagged task literally
// named "<text> --project X"). Flags must be honored in any position.
func TestAddFlagsAfterText(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"project after text", []string{"write the docs", "--project", "marketing"}},
		{"project before text", []string{"--project", "marketing", "write the docs"}},
		{"project interleaved", []string{"write", "the", "--project", "marketing", "docs"}},
		{"project equals form after text", []string{"write the docs", "--project=marketing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			date := testDate
			_, err := st.AddProject("Marketing")
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, cmdAdd(st, date, tc.args, &buf))
			out := buf.String()

			// Tag must be applied and the flag must NOT leak into the text.
			assert.Contains(t, out, "#marketing")
			assert.NotContains(t, out, "--project")

			// Verify via JSON that the text is clean and the project is set.
			var jbuf bytes.Buffer
			require.NoError(t, cmdList(st, date, []string{"--json"}, &jbuf))
			var items []jsonItem
			require.NoError(t, json.Unmarshal(jbuf.Bytes(), &items))
			require.Len(t, items, 1)
			assert.Equal(t, "marketing", items[0].Project)
			// Text must contain the words but none of the flag tokens.
			assert.NotContains(t, items[0].Text, "--project")
			assert.NotContains(t, items[0].Text, "marketing")
			assert.Contains(t, items[0].Text, "docs")
		})
	}
}

// TestAddParentAfterText guards the same bug for --parent: "add <text> --parent N"
// must create a subtask of item N, not a top-level task named "<text> --parent N".
func TestAddParentAfterText(t *testing.T) {
	for _, args := range [][]string{
		{"cover parsing", "--parent", "1"},    // flag after text (the reported bug)
		{"cover", "--parent", "1", "parsing"}, // interleaved
		{"cover parsing", "--parent=1"},       // equals form after text
	} {
		st := newTestStore(t)
		date := testDate
		require.NoError(t, cmdAdd(st, date, []string{"Parent task"}, &bytes.Buffer{}))

		var buf bytes.Buffer
		require.NoError(t, cmdAdd(st, date, args, &buf))

		items, err := loadPlan(st, date)
		require.NoError(t, err)
		require.Len(t, items, 2)
		// The new item is an indented subtask (depth 1) with clean text.
		assert.Equal(t, 1, items[1].Depth, "args=%v", args)
		assert.NotContains(t, items[1].Text, "--parent")
		assert.NotContains(t, items[1].Text, "cover parsing --parent")
	}
}

// TestAddUnknownFlagErrors verifies an unrecognized flag fails loudly instead of
// silently leaking into the task text.
func TestAddUnknownFlagErrors(t *testing.T) {
	st := newTestStore(t)
	err := runCmdErr(func(w *bytes.Buffer) error {
		return cmdAdd(st, testDate, []string{"hello", "--bogus"}, w)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")
	items, err2 := loadPlan(st, testDate)
	require.NoError(t, err2)
	assert.Empty(t, items) // no partial write
}

// TestAddDashLeadingText verifies "--" lets task text begin with a dash.
func TestAddDashLeadingText(t *testing.T) {
	st := newTestStore(t)
	date := testDate
	require.NoError(t, cmdAdd(st, date, []string{"--", "--not-a-flag", "text"}, &bytes.Buffer{}))
	items, err := loadPlan(st, date)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "--not-a-flag text", items[0].Text)
}

// TestGlobalDateAfterSubcommand verifies the global --date flag is honored when
// appended after the subcommand and its arguments (commonly-appended usage).
func TestGlobalDateAfterSubcommand(t *testing.T) {
	st := newTestStore(t)
	target := testDate.AddDate(0, 0, 5)

	var buf bytes.Buffer
	require.NoError(t, run(
		[]string{"--data-dir", st.DataDir, "add", "future task", "--date", target.Format(dateFormat)},
		&buf, &bytes.Buffer{}, "",
	))
	// Task must land on the target date, not today.
	items, err := loadPlan(st, target)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "future task", items[0].Text)
}

// TestBadIndex verifies clear error output for out-of-range indices.
func TestBadIndex(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Only task"}, &bytes.Buffer{}))

	err := runCmdErr(func(w *bytes.Buffer) error { return cmdDone(st, date, []string{"5"}, w) })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

// TestBadSlug verifies that an unknown --project slug is rejected with a clear
// error before any store write.
func TestBadSlug(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	err := runCmdErr(func(w *bytes.Buffer) error {
		return cmdAdd(st, date, []string{"--project", "nonexistent", "My task"}, w)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "not found")

	// Plan must be empty: no partial write.
	items, err2 := loadPlan(st, date)
	require.NoError(t, err2)
	assert.Empty(t, items)
}

// TestParentAndProjectError verifies that combining --parent and --project is
// rejected (subtasks cannot be independently tagged).
func TestParentAndProjectError(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Parent"}, &bytes.Buffer{}))
	_, err := st.AddProject("Work")
	require.NoError(t, err)

	err = runCmdErr(func(w *bytes.Buffer) error {
		return cmdAdd(st, date, []string{"--parent", "1", "--project", "work", "Sub"}, w)
	})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "cannot both be specified")
}

// TestJSONOutput verifies --json emits valid JSON with expected fields.
func TestJSONOutput(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	_, err := st.AddProject("Demo")
	require.NoError(t, err)
	require.NoError(t, cmdAdd(st, date, []string{"Untagged task"}, &bytes.Buffer{}))
	require.NoError(t, cmdAdd(st, date, []string{"--project", "demo", "Tagged task"}, &bytes.Buffer{}))

	var buf bytes.Buffer
	require.NoError(t, cmdList(st, date, []string{"--json"}, &buf))

	var items []jsonItem
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 2)

	assert.Equal(t, 1, items[0].Number)
	assert.Equal(t, "Untagged task", items[0].Text)
	assert.Equal(t, "todo", items[0].State)
	assert.Empty(t, items[0].Project)

	assert.Equal(t, 2, items[1].Number)
	assert.Equal(t, "Tagged task", items[1].Text)
	assert.Equal(t, "demo", items[1].Project)
}

// TestBacklogMoveAndList verifies moving an item to backlog and listing it.
func TestBacklogMoveAndList(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	require.NoError(t, cmdAdd(st, date, []string{"Backlog candidate"}, &bytes.Buffer{}))
	require.NoError(t, cmdAdd(st, date, []string{"Stay today"}, &bytes.Buffer{}))

	var buf bytes.Buffer
	require.NoError(t, cmdBacklog(st, date, []string{"1"}, &buf))
	// Confirmation line mentions the moved item; updated plan shows only the remainder.
	assert.Contains(t, buf.String(), "Backlog candidate") // in confirmation line
	assert.Contains(t, buf.String(), "Stay today")
	// Plan must no longer have the moved item (verify via store).
	items, err := loadPlan(st, date)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Stay today", items[0].Text)

	var listBuf bytes.Buffer
	require.NoError(t, cmdBacklog(st, date, []string{"list"}, &listBuf))
	assert.Contains(t, listBuf.String(), "Backlog candidate")
	assert.Contains(t, listBuf.String(), "Current (1)")
}

// TestProjectsList verifies projects listing after adding projects.
func TestProjectsList(t *testing.T) {
	st := newTestStore(t)

	require.NoError(t, cmdProject(st, []string{"add", "Alpha"}, &bytes.Buffer{}))
	require.NoError(t, cmdProject(st, []string{"add", "Beta"}, &bytes.Buffer{}))

	output := runCmd(t, func(w *bytes.Buffer) error { return cmdProjects(st, w) })
	assert.Contains(t, output, "alpha")
	assert.Contains(t, output, "Alpha")
	assert.Contains(t, output, "beta")
	assert.Contains(t, output, "Beta")
	assert.Contains(t, output, "open")
}

// TestRunDispatch verifies the top-level run function dispatches correctly.
func TestRunDispatch(t *testing.T) {
	st := newTestStore(t)
	date := testDate

	// Pre-populate via store directly so we can test run() with --data-dir.
	err := st.AddPlanItem(date, "Direct item")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = run(
		[]string{"--data-dir", st.DataDir, "--date", date.Format(dateFormat), "list"},
		&buf, &bytes.Buffer{}, "",
	)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Direct item")
}

// TestRunUnknownSubcommand verifies exit code 2 for bad subcommands.
func TestRunUnknownSubcommand(t *testing.T) {
	err := run([]string{"--data-dir", t.TempDir(), "bogus"}, &bytes.Buffer{}, &bytes.Buffer{}, "")
	require.Error(t, err)
	var ec *exitError
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, 2, ec.code)
}

// TestRunDefaultCmdDispatch verifies that with no subcommand, an empty
// defaultCmd prints usage while a non-empty one dispatches to that command.
func TestRunDefaultCmdDispatch(t *testing.T) {
	dir := t.TempDir()

	var usage bytes.Buffer
	require.NoError(t, run([]string{"--data-dir", dir}, &usage, &bytes.Buffer{}, ""))
	assert.Contains(t, usage.String(), "Usage: dpr", "empty defaultCmd should print usage")

	// A non-empty defaultCmd dispatches to that subcommand (use "list", which
	// is safe and needs no TTY, as a stand-in for the interactive "tui").
	var listed bytes.Buffer
	require.NoError(t, run([]string{"--data-dir", dir}, &listed, &bytes.Buffer{}, "list"))
	assert.NotContains(t, listed.String(), "Usage: dpr", "defaultCmd should dispatch, not print usage")
}
