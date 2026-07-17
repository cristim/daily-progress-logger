package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// ---------- stateCheckGlyph ----------

func TestStateCheckGlyph(t *testing.T) {
	tests := []struct {
		name  string
		state store.ItemState
		done  bool
		want  string
	}{
		{"todo leaf", store.StateTodo, false, "[ ]"},
		{"done leaf via state", store.StateDone, false, "[x]"},
		{"done via rollup", store.StateTodo, true, "[x]"},
		{"postponed", store.StatePostponed, false, "[>]"},
		{"postponed but rolled-up done", store.StatePostponed, true, "[x]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stateCheckGlyph(tc.state, tc.done))
		})
	}
}

// ---------- nodeLabel ----------

func TestNodeLabel_Task(t *testing.T) {
	task := &store.TreeTask{Index: 0, Depth: 0, Text: "Buy groceries", State: store.StateTodo, Done: false}
	ref := &nodeRef{kind: kindTask, task: task}
	label := nodeLabel(ref)
	assert.Contains(t, label, "[ ]")
	assert.Contains(t, label, "Buy groceries")
}

func TestNodeLabel_TaskDone(t *testing.T) {
	task := &store.TreeTask{Index: 0, Depth: 0, Text: "Buy groceries", State: store.StateDone, Done: true}
	ref := &nodeRef{kind: kindTask, task: task}
	label := nodeLabel(ref)
	// done tasks get the dim tview tag
	assert.Contains(t, label, "[::d]")
	assert.Contains(t, label, "[x]")
}

func TestNodeLabel_TaskPostponed(t *testing.T) {
	task := &store.TreeTask{Index: 0, Depth: 0, Text: "Defer me", State: store.StatePostponed, Done: false}
	ref := &nodeRef{kind: kindTask, task: task}
	label := nodeLabel(ref)
	assert.Contains(t, label, "[>]")
}

func TestNodeLabel_TaskDepth(t *testing.T) {
	task := &store.TreeTask{Index: 1, Depth: 2, Text: "Nested", State: store.StateTodo, Done: false}
	ref := &nodeRef{kind: kindTask, task: task}
	label := nodeLabel(ref)
	// depth 2 -> four spaces indent
	assert.Contains(t, label, "    [ ] Nested")
}

func TestNodeLabel_Project(t *testing.T) {
	p := &store.TreeProject{ID: "proj-1", Name: "My Project", Done: false}
	ref := &nodeRef{kind: kindProject, project: p}
	label := nodeLabel(ref)
	assert.Contains(t, label, "My Project")
	assert.Contains(t, label, "[::b]") // bold
}

func TestNodeLabel_ProjectDone(t *testing.T) {
	p := &store.TreeProject{ID: "proj-1", Name: "Done Project", Done: true}
	ref := &nodeRef{kind: kindProject, project: p}
	label := nodeLabel(ref)
	assert.Contains(t, label, "[::d]") // dim
}

func TestNodeLabel_Recurring(t *testing.T) {
	r := &store.RecurringTask{Text: "Daily standup", Project: "work"}
	ref := &nodeRef{kind: kindRecurring, recur: r}
	label := nodeLabel(ref)
	assert.Contains(t, label, "Daily standup")
	assert.Contains(t, label, "#work")
}

func TestNodeLabel_Recycled(t *testing.T) {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	rc := &store.TreeTask{Text: "Old task", Project: "proj", Date: date}
	ref := &nodeRef{kind: kindRecycled, recycled: rc}
	label := nodeLabel(ref)
	assert.Contains(t, label, "Old task")
	assert.Contains(t, label, "[proj]")
	assert.Contains(t, label, "2026-01-15")
}

// ---------- rootLabel ----------

func TestRootLabel(t *testing.T) {
	// Mon 14 Jul 2025 is week 29
	date := time.Date(2025, 7, 14, 0, 0, 0, 0, time.Local)
	label := rootLabel(date)
	assert.Contains(t, label, "Mon 14 Jul 2025")
	assert.Contains(t, label, "W29")
}

// ---------- sectionLabel ----------

func TestSectionLabel(t *testing.T) {
	label := sectionLabel("Unfiled", 3)
	assert.Contains(t, label, "Unfiled")
	assert.Contains(t, label, "(3)")
}

// ---------- buildFlatModel ----------

func TestBuildFlatModel_Empty(t *testing.T) {
	pt := &store.ProjectTree{}
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	nodes := buildFlatModel(pt, date)
	// should have at minimum: root + "Recycle Bin" section
	require.GreaterOrEqual(t, len(nodes), 2)
	assert.Equal(t, kindRoot, nodes[0].kind)
}

func TestBuildFlatModel_ProjectsAndTasks(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	task1 := store.TreeTask{Index: 0, Depth: 0, Text: "Task A", State: store.StateTodo, Date: date}
	task2 := store.TreeTask{Index: 1, Depth: 0, Text: "Task B", State: store.StateDone, Date: date, Done: true}
	pt := &store.ProjectTree{
		Projects: []store.TreeProject{
			{ID: "p1", Name: "Alpha", Tasks: []store.TreeTask{task1, task2}},
		},
	}
	nodes := buildFlatModel(pt, date)

	kinds := make([]nodeKind, len(nodes))
	for i, n := range nodes {
		kinds[i] = n.kind
	}
	// root, project, two task nodes, recycleSection
	assert.Equal(t, kindRoot, kinds[0])
	assert.Equal(t, kindProject, kinds[1])
	assert.Equal(t, kindTask, kinds[2])
	assert.Equal(t, kindTask, kinds[3])
	// recycle bin section at the end
	assert.Equal(t, kindSection, kinds[len(kinds)-1])
}

func TestBuildFlatModel_Unfiled(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	task := store.TreeTask{Index: 0, Depth: 0, Text: "No project", State: store.StateTodo, Date: date}
	pt := &store.ProjectTree{Unfiled: []store.TreeTask{task}}
	nodes := buildFlatModel(pt, date)

	kindsFound := make([]nodeKind, 0, len(nodes))
	for _, n := range nodes {
		kindsFound = append(kindsFound, n.kind)
	}
	assert.Contains(t, kindsFound, kindSection) // "Unfiled" section
	assert.Contains(t, kindsFound, kindTask)
}

func TestBuildFlatModel_Recurring(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	r := store.RecurringTask{Text: "Daily standup"}
	pt := &store.ProjectTree{Recurring: []store.RecurringTask{r}}
	nodes := buildFlatModel(pt, date)

	hasRecurring := false
	for _, n := range nodes {
		if n.kind == kindRecurring {
			hasRecurring = true
		}
	}
	assert.True(t, hasRecurring, "expected at least one kindRecurring node")
}

func TestBuildFlatModel_NestedTasks(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	child := store.TreeTask{Index: 1, Depth: 1, Text: "Subtask", State: store.StateTodo, Date: date}
	parent := store.TreeTask{
		Index: 0, Depth: 0, Text: "Parent", State: store.StateTodo, Date: date,
		Children: []store.TreeTask{child},
	}
	pt := &store.ProjectTree{Unfiled: []store.TreeTask{parent}}
	nodes := buildFlatModel(pt, date)

	var tasks []flatNode
	for _, n := range nodes {
		if n.kind == kindTask {
			tasks = append(tasks, n)
		}
	}
	require.Len(t, tasks, 2)
	assert.Equal(t, "Parent", tasks[0].ref.task.Text)
	assert.Equal(t, "Subtask", tasks[1].ref.task.Text)
}

// ---------- projectIDForTask ----------

func TestProjectIDForTask(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	task := store.TreeTask{Index: 2, Depth: 0, Text: "Do something", Date: date}
	pt := &store.ProjectTree{
		Projects: []store.TreeProject{
			{ID: "alpha", Name: "Alpha", Tasks: []store.TreeTask{task}},
		},
	}
	assert.Equal(t, "alpha", projectIDForTask(pt, &task))
}

func TestProjectIDForTask_Unfiled(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	task := store.TreeTask{Index: 5, Depth: 0, Text: "No project", Date: date}
	pt := &store.ProjectTree{
		Projects: []store.TreeProject{
			{ID: "alpha", Name: "Alpha", Tasks: []store.TreeTask{}},
		},
		Unfiled: []store.TreeTask{task},
	}
	assert.Empty(t, projectIDForTask(pt, &task))
}

// ---------- date helpers ----------

func TestNextPrevDay(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	assert.Equal(t, time.Date(2026, 7, 18, 0, 0, 0, 0, time.Local), nextDay(date))
	assert.Equal(t, time.Date(2026, 7, 16, 0, 0, 0, 0, time.Local), prevDay(date))
}

func TestNextPrevWeek(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)
	assert.Equal(t, time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local), nextWeekDay(date))
	assert.Equal(t, time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local), prevWeekDay(date))
}

func TestMonthBoundary(t *testing.T) {
	date := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)
	next := nextDay(date)
	assert.Equal(t, time.August, next.Month())
	assert.Equal(t, 1, next.Day())
}
