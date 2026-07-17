package main

// tui.go — full-screen interactive TUI for dpl, launched via `dpl tui` or
// `dpl ui`. Implemented with tview+tcell (pure Go, CGO_ENABLED=0).
//
// Architecture note: all functions that convert store data into display
// structures (buildNodeModel, nodeLabel, stateCheckGlyph, …) are pure and
// have no tview dependency, so they can be unit-tested without a terminal.
// The tview wiring lives in runTUI / tuiApp.

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// nodeKind classifies a tree node so action handlers can dispatch correctly.
type nodeKind int

const (
	kindRoot      nodeKind = iota // the top-level date header
	kindProject                   // a project header (no task)
	kindTask                      // a concrete task (has an Index)
	kindSection                   // "Unfiled", "Recurring", "Recycle Bin"
	kindRecurring                 // a recurring-template entry (read-only)
	kindRecycled                  // a recycle-bin entry (read-only)
)

// hintUnfiled is the context hint shown in the add-task prompt for unfiled tasks.
const hintUnfiled = " (unfiled)"

// nodeRef is the immutable payload attached to each tview.TreeNode. A pointer
// is stored so the node map can share the same struct with the flat model.
type nodeRef struct {
	kind     nodeKind
	task     *store.TreeTask      // non-nil for kindTask
	recur    *store.RecurringTask // non-nil for kindRecurring
	recycled *store.TreeTask      // non-nil for kindRecycled
	project  *store.TreeProject   // non-nil for kindProject
}

// ---------- pure-logic helpers (unit-testable, no tview) ----------

// stateCheckGlyph returns the checkbox string for a task state.
func stateCheckGlyph(s store.ItemState, done bool) string {
	// done (rolled-up) overrides leaf state for display.
	if done {
		return glyphDone
	}
	switch s {
	case store.StateTodo:
		return glyphTodo
	case store.StateDone:
		return glyphDone
	case store.StatePostponed:
		return glyphPostponed
	}
	return glyphTodo
}

// nodeLabel returns the tview-formatted label for a node. tview interprets
// "[::b]text[::-]" style color tags; we only use them for done/postponed items
// and project headers.
func nodeLabel(ref *nodeRef) string {
	switch ref.kind {
	case kindProject:
		p := ref.project
		if p.Done {
			return "[::d]" + p.Name + "[::-]" // dim if project is globally done
		}
		return "[::b]" + p.Name + "[::-]" // bold
	case kindTask:
		t := ref.task
		glyph := stateCheckGlyph(t.State, t.Done)
		indent := strings.Repeat("  ", t.Depth)
		label := fmt.Sprintf("%s%s %s", indent, glyph, t.Text)
		if t.Done {
			return "[::d]" + label + "[::-]"
		}
		if t.State == store.StatePostponed {
			return "[gray]" + label + "[-]"
		}
		return label
	case kindRecurring:
		r := ref.recur
		suffix := ""
		if r.Project != "" {
			suffix = " #" + r.Project
		}
		return "[gray]  " + r.Text + suffix + "[-]"
	case kindRecycled:
		rc := ref.recycled
		proj := ""
		if rc.Project != "" {
			proj = " [" + rc.Project + "]"
		}
		return "[gray]  [x] " + rc.Text + proj + " (" + rc.Date.Format("2006-01-02") + ")[-]"
	case kindSection:
		// section header label is set directly on the node by the caller.
		return ""
	case kindRoot:
		return ""
	}
	return ""
}

// sectionLabel returns the display string for a section header node.
func sectionLabel(name string, count int) string {
	return fmt.Sprintf("[::b]%s[::-] [gray](%d)[-]", name, count)
}

// rootLabel returns the root node label: date + ISO week.
func rootLabel(date time.Time) string {
	_, week := date.ISOWeek()
	return fmt.Sprintf("[::b]%s · W%d[::-]", date.Format("Mon 02 Jan 2006"), week)
}

// flatNode is a row in the renderable model — used by unit tests to check the
// shape of the tree without spinning up a tview app.
type flatNode struct {
	label string
	kind  nodeKind
	ref   *nodeRef
}

// buildFlatModel converts a ProjectTree into an ordered flat slice of display
// nodes (depth-first pre-order). This is the testable core of the tree builder.
func buildFlatModel(pt *store.ProjectTree, date time.Time) []flatNode {
	var out []flatNode

	// root
	out = append(out, flatNode{
		label: rootLabel(date),
		kind:  kindRoot,
		ref:   &nodeRef{kind: kindRoot},
	})

	// projects
	for i := range pt.Projects {
		p := &pt.Projects[i]
		ref := &nodeRef{kind: kindProject, project: p}
		out = append(out, flatNode{label: nodeLabel(ref), kind: kindProject, ref: ref})
		out = append(out, taskNodes(p.Tasks)...)
	}

	// unfiled
	if len(pt.Unfiled) > 0 {
		secRef := &nodeRef{kind: kindSection}
		out = append(out, flatNode{
			label: sectionLabel("Unfiled", len(pt.Unfiled)),
			kind:  kindSection,
			ref:   secRef,
		})
		out = append(out, taskNodes(pt.Unfiled)...)
	}

	// recurring (read-only)
	if len(pt.Recurring) > 0 {
		secRef := &nodeRef{kind: kindSection}
		out = append(out, flatNode{
			label: sectionLabel("Recurring", len(pt.Recurring)),
			kind:  kindSection,
			ref:   secRef,
		})
		for i := range pt.Recurring {
			r := &pt.Recurring[i]
			ref := &nodeRef{kind: kindRecurring, recur: r}
			out = append(out, flatNode{label: nodeLabel(ref), kind: kindRecurring, ref: ref})
		}
	}

	// recycle bin section (collapsed by default in the live tree)
	secRef := &nodeRef{kind: kindSection}
	out = append(out, flatNode{
		label: sectionLabel("Recycle Bin", len(pt.Recycled)),
		kind:  kindSection,
		ref:   secRef,
	})
	for i := range pt.Recycled {
		rc := &pt.Recycled[i]
		ref := &nodeRef{kind: kindRecycled, recycled: rc}
		out = append(out, flatNode{label: nodeLabel(ref), kind: kindRecycled, ref: ref})
	}

	return out
}

// taskNodes recursively returns flatNodes for tasks and their children.
func taskNodes(tasks []store.TreeTask) []flatNode {
	var out []flatNode
	for i := range tasks {
		t := &tasks[i]
		ref := &nodeRef{kind: kindTask, task: t}
		out = append(out, flatNode{label: nodeLabel(ref), kind: kindTask, ref: ref})
		if len(t.Children) > 0 {
			out = append(out, taskNodes(t.Children)...)
		}
	}
	return out
}

// projectIDForTask returns the project ID of the task searching through the
// ProjectTree; returns "" for unfiled tasks.
func projectIDForTask(pt *store.ProjectTree, task *store.TreeTask) string {
	for _, p := range pt.Projects {
		if taskBelongsToProject(p.Tasks, task.Index) {
			return p.ID
		}
	}
	return ""
}

// taskBelongsToProject reports whether any task in the tree rooted at tasks
// has Index == idx.
func taskBelongsToProject(tasks []store.TreeTask, idx int) bool {
	for i := range tasks {
		if tasks[i].Index == idx {
			return true
		}
		if taskBelongsToProject(tasks[i].Children, idx) {
			return true
		}
	}
	return false
}

// nextDay returns the next calendar day after date.
func nextDay(date time.Time) time.Time { return date.AddDate(0, 0, 1) }

// prevDay returns the calendar day before date.
func prevDay(date time.Time) time.Time { return date.AddDate(0, 0, -1) }

// nextWeekDay returns the same weekday, one week later.
func nextWeekDay(date time.Time) time.Time { return date.AddDate(0, 0, 7) }

// prevWeekDay returns the same weekday, one week earlier.
func prevWeekDay(date time.Time) time.Time { return date.AddDate(0, 0, -7) }

// todayMidnight returns today's date at midnight in local time.
func todayMidnight() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

// ---------- tview app ----------

const helpText = `[::b]Nav[::-]  ↑↓ move  ←→/Enter expand/collapse  Space toggle done` +
	`  [::b]Actions[::-]  a add  e edit  d delete  p postpone→tomorrow  P/w postpone→week` +
	`  [ prev day  ] next day  t today  r reload  q quit`

// tuiApp holds the runtime state of the running TUI.
type tuiApp struct {
	app         *tview.Application
	tree        *tview.TreeView
	footer      *tview.TextView
	st          *store.Store
	date        time.Time
	projectTree *store.ProjectTree
	// lastSelectedIndex is the task Index to re-select after a rebuild; -1 =
	// none (select root or first child).
	lastSelectedIndex int
}

// runTUI launches the full-screen TUI. It blocks until the user quits.
func runTUI(st *store.Store, date time.Time) error {
	a := &tuiApp{
		app:               tview.NewApplication(),
		st:                st,
		date:              date,
		lastSelectedIndex: -1,
	}

	// tree widget
	a.tree = tview.NewTreeView()
	a.tree.SetBorder(false)
	a.tree.SetGraphicsColor(tcell.ColorDarkGray)

	// footer
	a.footer = tview.NewTextView()
	a.footer.SetDynamicColors(true)
	a.footer.SetText(helpText)
	a.footer.SetTextAlign(tview.AlignLeft)
	a.footer.SetBorder(false)
	a.footer.SetWrap(false)

	flex := a.buildFlex()

	// initial tree load
	if err := a.reload(); err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	// key bindings
	a.tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() { //nolint:exhaustive // tcell defines many keys; only a subset need handling here
		case tcell.KeyRune:
			return a.handleRune(event)
		case tcell.KeyEnter:
			return a.handleEnter(event)
		case tcell.KeyLeft:
			node := a.tree.GetCurrentNode()
			if node != nil {
				node.SetExpanded(false)
			}
			return nil
		case tcell.KeyRight:
			node := a.tree.GetCurrentNode()
			if node != nil {
				node.SetExpanded(true)
			}
			return nil
		default:
			return event
		}
	})

	a.app.SetRoot(flex, true).EnableMouse(false)
	return a.app.Run()
}

// handleRune dispatches single-character key events in the main tree view.
func (a *tuiApp) handleRune(event *tcell.EventKey) *tcell.EventKey {
	switch event.Rune() {
	case 'q', 'Q':
		a.app.Stop()
	case ' ', 'x':
		a.toggleDone()
	case 'd':
		a.promptDelete()
	case 'a':
		a.promptAdd()
	case 'e':
		a.promptEdit()
	case 'p':
		a.postponeNextDay()
	case 'P', 'w':
		a.postponeNextWeek()
	case '[':
		a.changeDay(prevDay(a.date))
	case ']':
		a.changeDay(nextDay(a.date))
	case 't':
		a.changeDay(todayMidnight())
	case 'r':
		a.reloadAndStatus("Reloaded.")
	default:
		return event
	}
	return nil
}

// handleEnter toggles done on leaf tasks; lets tview handle expand/collapse on
// branch nodes.
func (a *tuiApp) handleEnter(event *tcell.EventKey) *tcell.EventKey {
	node := a.tree.GetCurrentNode()
	if node == nil {
		return event
	}
	ref := nodeRefOf(node)
	if ref == nil {
		return event
	}
	if ref.kind == kindTask && len(node.GetChildren()) == 0 {
		a.toggleDone()
		return nil
	}
	return event // let tview expand/collapse the branch
}

// nodeRefOf extracts the *nodeRef from a TreeNode's reference.
func nodeRefOf(node *tview.TreeNode) *nodeRef {
	if node == nil {
		return nil
	}
	ref, _ := node.GetReference().(*nodeRef)
	return ref
}

// reload fetches a fresh ProjectTree from the store and rebuilds the tview tree.
func (a *tuiApp) reload() error {
	pt, err := a.st.BuildProjectTree(a.date)
	if err != nil {
		return err
	}
	a.projectTree = pt
	a.rebuildTree()
	return nil
}

// reloadAndStatus reloads and shows a status message on success.
func (a *tuiApp) reloadAndStatus(msg string) {
	if err := a.reload(); err != nil {
		a.showError(err)
		return
	}
	a.showStatus(msg)
}

// rebuildTree converts a.projectTree into tview nodes and rebuilds the tree view.
func (a *tuiApp) rebuildTree() {
	pt := a.projectTree
	date := a.date

	root := tview.NewTreeNode(rootLabel(date)).
		SetSelectable(false).
		SetReference(&nodeRef{kind: kindRoot})

	var selectedNode *tview.TreeNode

	// addTaskNodes attaches a task subtree under parent, depth-first.
	var addTaskNodes func(parent *tview.TreeNode, tasks []store.TreeTask)
	addTaskNodes = func(parent *tview.TreeNode, tasks []store.TreeTask) {
		for i := range tasks {
			t := &tasks[i]
			ref := &nodeRef{kind: kindTask, task: t}
			label := nodeLabel(ref)
			node := tview.NewTreeNode(label).SetReference(ref).SetSelectable(true)
			if t.Done {
				node.SetColor(tcell.ColorGray)
			}
			parent.AddChild(node)
			if t.Index == a.lastSelectedIndex {
				selectedNode = node
			}
			if len(t.Children) > 0 {
				addTaskNodes(node, t.Children)
			}
		}
	}

	// Projects
	for i := range pt.Projects {
		p := &pt.Projects[i]
		ref := &nodeRef{kind: kindProject, project: p}
		label := nodeLabel(ref)
		pNode := tview.NewTreeNode(label).SetReference(ref).SetSelectable(true)
		if p.Done {
			pNode.SetColor(tcell.ColorGray)
		} else {
			pNode.SetColor(tcell.ColorYellow)
		}
		root.AddChild(pNode)
		addTaskNodes(pNode, p.Tasks)
	}

	// Unfiled
	if len(pt.Unfiled) > 0 {
		secRef := &nodeRef{kind: kindSection}
		secNode := tview.NewTreeNode(sectionLabel("Unfiled", len(pt.Unfiled))).
			SetReference(secRef).SetSelectable(true).SetColor(tcell.ColorTeal)
		root.AddChild(secNode)
		addTaskNodes(secNode, pt.Unfiled)
	}

	// Recurring (read-only, collapsed by default)
	if len(pt.Recurring) > 0 {
		secRef := &nodeRef{kind: kindSection}
		secNode := tview.NewTreeNode(sectionLabel("Recurring", len(pt.Recurring))).
			SetReference(secRef).SetSelectable(true).SetColor(tcell.ColorTeal)
		secNode.SetExpanded(false)
		root.AddChild(secNode)
		for i := range pt.Recurring {
			r := &pt.Recurring[i]
			ref := &nodeRef{kind: kindRecurring, recur: r}
			rNode := tview.NewTreeNode(nodeLabel(ref)).SetReference(ref).SetSelectable(true)
			rNode.SetColor(tcell.ColorGray)
			secNode.AddChild(rNode)
		}
	}

	// Recycle Bin (collapsed by default)
	{
		secRef := &nodeRef{kind: kindSection}
		secNode := tview.NewTreeNode(sectionLabel("Recycle Bin", len(pt.Recycled))).
			SetReference(secRef).SetSelectable(true).SetColor(tcell.ColorTeal)
		secNode.SetExpanded(false)
		root.AddChild(secNode)
		for i := range pt.Recycled {
			rc := &pt.Recycled[i]
			ref := &nodeRef{kind: kindRecycled, recycled: rc}
			rcNode := tview.NewTreeNode(nodeLabel(ref)).SetReference(ref).SetSelectable(true)
			rcNode.SetColor(tcell.ColorGray)
			secNode.AddChild(rcNode)
		}
	}

	a.tree.SetRoot(root).SetCurrentNode(root)
	if selectedNode != nil {
		a.tree.SetCurrentNode(selectedNode)
	}
}

// selectedTask returns the *store.TreeTask for the currently selected node,
// or nil when the selection is not a task.
func (a *tuiApp) selectedTask() *store.TreeTask {
	node := a.tree.GetCurrentNode()
	if node == nil {
		return nil
	}
	ref := nodeRefOf(node)
	if ref == nil || ref.kind != kindTask {
		return nil
	}
	return ref.task
}

// showStatus sets the footer to a green status message.
func (a *tuiApp) showStatus(msg string) {
	a.footer.SetText("[green]" + tview.Escape(msg) + "[-]\n" + helpText)
}

// showError sets the footer to a red error message.
func (a *tuiApp) showError(err error) {
	a.footer.SetText("[red]Error: " + tview.Escape(err.Error()) + "[-]\n" + helpText)
}

// toggleDone flips the done/todo state of the selected task.
func (a *tuiApp) toggleDone() {
	t := a.selectedTask()
	if t == nil {
		return
	}
	newState := store.StateDone
	if t.Done || t.State == store.StateDone {
		newState = store.StateTodo
	}
	a.lastSelectedIndex = t.Index
	if err := a.st.SetPlanItemState(t.Date, t.Index, newState); err != nil {
		a.showError(err)
		return
	}
	label := "Marked done."
	if newState == store.StateTodo {
		label = "Marked to-do."
	}
	a.reloadAndStatus(label)
}

// promptDelete shows a confirmation modal before deleting.
func (a *tuiApp) promptDelete() {
	t := a.selectedTask()
	if t == nil {
		a.showStatus("Select a task to delete.")
		return
	}
	taskIdx := t.Index
	taskDate := t.Date
	taskText := t.Text
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete %q?\n(moves to recycle bin)", taskText)).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(_ int, label string) {
			a.app.SetRoot(a.buildFlex(), true).SetFocus(a.tree)
			if label != "Delete" {
				a.showStatus("Cancelled.")
				return
			}
			a.lastSelectedIndex = -1
			if err := a.st.DeleteTask(taskDate, taskIdx); err != nil {
				a.showError(err)
				return
			}
			a.reloadAndStatus("Deleted (moved to recycle bin).")
		})
	a.app.SetRoot(modal, false).SetFocus(modal)
}

// addContext carries the routing decision for promptAdd: where to insert the
// new task (project, subtask parent, or unfiled).
type addContext struct {
	projectID string
	parentIdx int // -1 = no parent (top-level)
	hint      string
}

// resolveAddContext determines where a new task should land based on the
// currently selected node.
func (a *tuiApp) resolveAddContext() addContext {
	node := a.tree.GetCurrentNode()
	ref := nodeRefOf(node)
	ctx := addContext{parentIdx: -1, hint: hintUnfiled}
	if ref == nil {
		return ctx
	}
	switch ref.kind {
	case kindProject:
		ctx.projectID = ref.project.ID
		ctx.hint = projectHint(a.projectTree.Projects, ctx.projectID)
	case kindTask:
		ctx = a.addContextForTask(node, ref.task)
	case kindRoot, kindSection, kindRecurring, kindRecycled:
		// unfiled (ctx already initialized with hintUnfiled)
	}
	return ctx
}

// projectHint returns the " [Name]" hint string for a project ID.
func projectHint(projects []store.TreeProject, id string) string {
	for _, p := range projects {
		if p.ID == id {
			return " [" + p.Name + "]"
		}
	}
	return ""
}

// addContextForTask resolves the add-context when a task node is selected: if
// the task has visible children (it's a branch), the new item becomes a
// subtask; otherwise it lands in the same project (or unfiled).
func (a *tuiApp) addContextForTask(node *tview.TreeNode, task *store.TreeTask) addContext {
	ctx := addContext{parentIdx: -1}
	if len(node.GetChildren()) > 0 {
		ctx.parentIdx = task.Index
		ctx.hint = " (subtask)"
		return ctx
	}
	ctx.projectID = projectIDForTask(a.projectTree, task)
	if ctx.projectID != "" {
		ctx.hint = projectHint(a.projectTree.Projects, ctx.projectID)
	} else {
		ctx.hint = hintUnfiled
	}
	return ctx
}

// promptAdd opens an input field to add a new task.
func (a *tuiApp) promptAdd() {
	ctx := a.resolveAddContext()
	label := "New task" + ctx.hint

	form := tview.NewForm()
	form.AddInputField(label, "", 60, nil, nil)
	form.AddButton("Add", func() {
		field, ok := form.GetFormItemByLabel(label).(*tview.InputField)
		if !ok {
			return
		}
		text := strings.TrimSpace(field.GetText())
		a.app.SetRoot(a.buildFlex(), true).SetFocus(a.tree)
		if text == "" {
			a.showStatus("Cancelled.")
			return
		}
		a.doAdd(ctx, text)
	})
	form.AddButton("Cancel", func() {
		a.app.SetRoot(a.buildFlex(), true).SetFocus(a.tree)
		a.showStatus("Cancelled.")
	})
	form.SetBorder(true).SetTitle(" Add task ").SetTitleAlign(tview.AlignLeft)
	form.SetBorderColor(tcell.ColorYellow)

	a.app.SetRoot(centeredForm(form, 7), true).SetFocus(form)
}

// doAdd performs the store write for promptAdd.
func (a *tuiApp) doAdd(ctx addContext, text string) {
	var err error
	switch {
	case ctx.parentIdx >= 0:
		err = a.st.AddSubtask(a.date, ctx.parentIdx, text)
	case ctx.projectID != "":
		err = a.st.AddTaggedTask(a.date, text, ctx.projectID)
	default:
		err = a.st.AddPlanItem(a.date, text)
	}
	if err != nil {
		a.showError(err)
		return
	}
	a.reloadAndStatus("Task added.")
}

// promptEdit opens an input pre-filled with the current task text.
func (a *tuiApp) promptEdit() {
	t := a.selectedTask()
	if t == nil {
		a.showStatus("Select a task to edit.")
		return
	}
	taskIdx := t.Index
	taskDate := t.Date
	currentText := t.Text

	const label = "Edit task"
	form := tview.NewForm()
	form.AddInputField(label, currentText, 60, nil, nil)
	form.AddButton("Save", func() {
		field, ok := form.GetFormItemByLabel(label).(*tview.InputField)
		if !ok {
			return
		}
		newText := strings.TrimSpace(field.GetText())
		a.app.SetRoot(a.buildFlex(), true).SetFocus(a.tree)
		if newText == "" || newText == currentText {
			a.showStatus("No change.")
			return
		}
		a.lastSelectedIndex = taskIdx
		if err := a.st.EditTaskText(taskDate, taskIdx, newText); err != nil {
			a.showError(err)
			return
		}
		a.reloadAndStatus("Task updated.")
	})
	form.AddButton("Cancel", func() {
		a.app.SetRoot(a.buildFlex(), true).SetFocus(a.tree)
		a.showStatus("Cancelled.")
	})
	form.SetBorder(true).SetTitle(" Edit task ").SetTitleAlign(tview.AlignLeft)
	form.SetBorderColor(tcell.ColorYellow)

	a.app.SetRoot(centeredForm(form, 7), true).SetFocus(form)
}

// centeredForm wraps a form in a centered Flex layout of the given height.
func centeredForm(form *tview.Form, height int) *tview.Flex {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, height, 0, true).
			AddItem(nil, 0, 1, false), 70, 0, true).
		AddItem(nil, 0, 1, false)
}

// postponeNextDay moves the selected task to tomorrow's plan.
func (a *tuiApp) postponeNextDay() {
	t := a.selectedTask()
	if t == nil {
		a.showStatus("Select a task to postpone.")
		return
	}
	a.lastSelectedIndex = -1
	if err := a.st.PostponeToNextDay(t.Date, t.Index); err != nil {
		a.showError(err)
		return
	}
	a.reloadAndStatus(fmt.Sprintf("Moved to %s.", nextDay(t.Date).Format("2006-01-02")))
}

// postponeNextWeek marks the task postponed and adds it to next week's backlog.
func (a *tuiApp) postponeNextWeek() {
	t := a.selectedTask()
	if t == nil {
		a.showStatus("Select a task to postpone.")
		return
	}
	a.lastSelectedIndex = -1
	if err := a.st.PostponePlanItem(t.Date, t.Index); err != nil {
		a.showError(err)
		return
	}
	a.reloadAndStatus("Marked [>] and queued in next week's backlog.")
}

// changeDay switches the viewed date and reloads.
func (a *tuiApp) changeDay(newDate time.Time) {
	a.date = newDate
	a.lastSelectedIndex = -1
	a.reloadAndStatus(newDate.Format("Mon 02 Jan 2006"))
}

// buildFlex constructs the main layout (tree + footer). Called on startup and
// after modal dialogs are dismissed so the app root is restored.
func (a *tuiApp) buildFlex() *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.tree, 0, 1, true).
		AddItem(a.footer, 2, 0, false)
}

// cmdTUI is the subcommand handler for `dpl tui` and `dpl ui`.
func cmdTUI(st *store.Store, date time.Time, _ []string, _ io.Writer) error {
	return runTUI(st, date)
}
