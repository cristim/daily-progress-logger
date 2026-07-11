package ui

import (
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// nodeKeyRole is the item-data role holding each tree node's identity string:
// "p:<projectID>", "t:<date>:<index>" (index into that day's plan), or "u:"
// for Unfiled. taskTextRole additionally caches a task node's display text
// (cosmetic uses only, e.g. a notification message; never used to resolve a
// store action, which always goes through the index in nodeKeyRole).
const (
	nodeKeyRole  = int(qt.UserRole)
	taskTextRole = int(qt.UserRole) + 1
)

// taskFunc applies a store operation to the plan item at idx on a given day.
type taskFunc func(date time.Time, idx int) error

// buildTree rebuilds the whole tree from the aggregated model, preserving each
// node's expand state (nodes not seen before default to expanded).
func (w *mainWindow) buildTree(model *store.ProjectTree) {
	w.tree.Clear()
	for _, p := range model.Projects {
		w.addProjectNode(p)
	}
	if len(model.Unfiled) > 0 {
		w.addUnfiledNode(model.Unfiled)
	}
	if len(model.Recurring) > 0 {
		w.addRecurringNode(model.Recurring)
	}
	if len(model.Recycled) > 0 {
		w.addRecycleNode(model.Recycled)
	}
}

// addRecurringNode builds the collapsible Recurring section listing each
// recurring template with its human-readable schedule and a Delete action.
func (w *mainWindow) addRecurringNode(tasks []store.RecurringTask) {
	item := qt.NewQTreeWidgetItem()
	item.SetData(0, nodeKeyRole, qt.NewQVariant11("rec:"))
	w.tree.AddTopLevelItem(item)
	label := qt.NewQLabel3("<b>Recurring</b>")
	label.SetTextFormat(qt.RichText)
	w.tree.SetItemWidget(item, 0, label.QWidget)
	for _, t := range tasks {
		child := qt.NewQTreeWidgetItem6(item)
		w.tree.SetItemWidget(child, 0, w.recurringRow(t))
	}
	item.SetExpanded(w.expandedOr("rec:", false)) // collapsed by default
}

// recurringRow shows a recurring template's text and schedule ("weekly Mon
// 09:30") plus a Delete action that removes the template.
func (w *mainWindow) recurringRow(t store.RecurringTask) *qt.QWidget {
	row, layout := newRowWidget()
	display, truncated := elideText(t.Text, 80)
	label := qt.NewQLabel3(html.EscapeString(display))
	label.SetTextFormat(qt.RichText)
	if truncated {
		label.SetToolTip(t.Text)
	}
	layout.AddWidget2(label.QWidget, 1)

	sched := qt.NewQLabel3(fmt.Sprintf(`<span style="color:#888888">%s</span>`,
		html.EscapeString(t.Rec.Describe())))
	sched.SetTextFormat(qt.RichText)
	layout.AddWidget(sched.QWidget)

	raw := t.Raw
	layout.AddWidget(w.textButton("Delete", "Delete this recurring task", func() {
		if err := w.app.store.RemoveRecurring(raw); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	}))
	return row
}

func (w *mainWindow) addProjectNode(p store.TreeProject) {
	key := "p:" + p.ID
	item := qt.NewQTreeWidgetItem()
	item.SetData(0, nodeKeyRole, qt.NewQVariant11(key))
	w.tree.AddTopLevelItem(item)
	w.tree.SetItemWidget(item, 0, w.projectRow(p))
	for _, task := range p.Tasks {
		w.addTaskNode(item, task)
	}
	item.SetExpanded(w.expandedOr(key, true))
}

// addTaskNode adds task's row under parent and recurses over its children, so
// the tree shows the full Project -> Task -> Subtask -> ... nesting.
func (w *mainWindow) addTaskNode(parent *qt.QTreeWidgetItem, task store.TreeTask) {
	item := qt.NewQTreeWidgetItem6(parent)
	item.SetData(0, nodeKeyRole, qt.NewQVariant11(taskKeyOf(task)))
	item.SetData(0, taskTextRole, qt.NewQVariant11(task.Text))
	w.tree.SetItemWidget(item, 0, w.taskRow(task))
	for _, child := range task.Children {
		w.addTaskNode(item, child)
	}
	item.SetExpanded(w.expandedOr(taskKeyOf(task), true))
}

func (w *mainWindow) addUnfiledNode(tasks []store.TreeTask) {
	item := qt.NewQTreeWidgetItem()
	item.SetData(0, nodeKeyRole, qt.NewQVariant11("u:"))
	w.tree.AddTopLevelItem(item)
	label := qt.NewQLabel3("<b>Unfiled</b>")
	label.SetTextFormat(qt.RichText)
	w.tree.SetItemWidget(item, 0, label.QWidget)
	for _, task := range tasks {
		w.addTaskNode(item, task)
	}
	item.SetExpanded(w.expandedOr("u:", true))
}

// addRecycleNode builds the collapsible Recycle Bin holding deleted tasks, each
// restorable to its day or purgeable.
func (w *mainWindow) addRecycleNode(tasks []store.TreeTask) {
	item := qt.NewQTreeWidgetItem()
	item.SetData(0, nodeKeyRole, qt.NewQVariant11("r:"))
	w.tree.AddTopLevelItem(item)
	label := qt.NewQLabel3("<b>Recycle Bin</b>")
	label.SetTextFormat(qt.RichText)
	w.tree.SetItemWidget(item, 0, label.QWidget)
	for _, task := range tasks {
		child := qt.NewQTreeWidgetItem6(item)
		w.tree.SetItemWidget(child, 0, w.recycleRow(task))
	}
	item.SetExpanded(w.expandedOr("r:", false)) // collapsed by default
}

// recycleRow shows a deleted task with its original day plus Restore / Delete
// (permanent) actions.
func (w *mainWindow) recycleRow(task store.TreeTask) *qt.QWidget {
	row, layout := newRowWidget()
	display, truncated := elideText(task.Text, 90)
	label := taskLabel(display, task.State, task.State == store.StateDone)
	if truncated {
		label.SetToolTip(task.Text)
	}
	layout.AddWidget2(label.QWidget, 1)

	dateLabel := qt.NewQLabel3(fmt.Sprintf(`<span style="color:#888888">%s</span>`,
		html.EscapeString(task.Date.Format("2 Jan"))))
	dateLabel.SetTextFormat(qt.RichText)
	layout.AddWidget(dateLabel.QWidget)

	date, text := task.Date, task.Text
	layout.AddWidget(w.textButton("Restore", "Restore to its day", func() {
		if err := w.app.store.RestoreTask(date, text); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	}))
	layout.AddWidget(w.textButton("Delete", "Delete permanently", func() {
		if err := w.app.store.PurgeRecycled(date, text); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	}))
	return row
}

// projectRow builds a project's row: its name (bold, struck through when done)
// plus Add-task / Rename / Close actions.
func (w *mainWindow) projectRow(p store.TreeProject) *qt.QWidget {
	row, layout := newRowWidget()
	layout.AddWidget2(nodeLabel(p.Name, p.Done, true).QWidget, 1)
	layout.AddWidget(w.textButton("+ Task", "Add a task to this project (today's plan)", func() { w.addProjectTask(p.ID) }))
	layout.AddWidget(w.textButton("Rename", "Rename this project", func() { w.renameProject(p.ID, p.Name) }))
	layout.AddWidget(w.textButton("Close", "Close (archive) this project", func() { w.closeProject(p.ID) }))
	return row
}

// taskRow builds a task's row: its text plus the Done/Not-done selector, the
// next-day / next-week / backlog defer buttons (all acting on the task's own
// day), a "+ Sub" button to add a nested subtask, and Delete.
func (w *mainWindow) taskRow(task store.TreeTask) *qt.QWidget {
	row, layout := newRowWidget()

	display, truncated := elideText(task.Text, 100)
	label := taskLabel(display, task.State, task.Done)
	if truncated {
		label.SetToolTip(task.Text)
	}
	layout.AddWidget2(label.QWidget, 1)

	date, index := task.Date, task.Index
	selector := newStateSelector(task.State)
	selector.onChanged(func(state store.ItemState) {
		w.runTaskAction(date, index, func(d time.Time, idx int) error {
			return w.app.store.SetPlanItemState(d, idx, state)
		})
	})
	layout.AddWidget(selector.widget)
	layout.AddWidget(w.taskActionButton(postponeIcon(), "Postpone to the next day", date, index,
		w.app.store.PostponeToNextDay))
	layout.AddWidget(w.taskActionButton(standardIcon(qt.QStyle__SP_ArrowUp), "Postpone to next week", date, index,
		w.app.store.PostponePlanItem))
	layout.AddWidget(w.taskActionButton(backlogIcon(), "Move to the cross-week backlog", date, index,
		func(d time.Time, idx int) error {
			if err := w.app.store.MoveToBacklog(d, idx); err != nil {
				return err
			}
			w.app.notifyBacklogMove(task.Text)
			return nil
		}))
	layout.AddWidget(w.textButton("+ Sub", "Add a subtask", func() { w.addSubtask(date, index) }))
	layout.AddWidget(w.taskActionButton(standardIcon(qt.QStyle__SP_TrashIcon),
		"Delete (moves to the recycle bin)", date, index, w.app.store.DeleteTask))
	return row
}

// newRowWidget makes the container + tight horizontal layout used by every tree
// row.
func newRowWidget() (*qt.QWidget, *qt.QHBoxLayout) {
	row := qt.NewQWidget2()
	layout := qt.NewQHBoxLayout(row)
	layout.SetContentsMargins(2, 1, 2, 1)
	return row, layout
}

// nodeLabel builds a rich-text label for a project name, bolded when bold and
// struck through + dimmed when done.
func nodeLabel(name string, done, bold bool) *qt.QLabel {
	text := html.EscapeString(name)
	if bold {
		text = "<b>" + text + "</b>"
	}
	if done {
		text = `<span style="color:#888888"><s>` + text + `</s></span>`
	}
	label := qt.NewQLabel3(text)
	label.SetTextFormat(qt.RichText)
	return label
}

// taskLabel styles a task's text: struck through + dimmed when done (a leaf's
// own checkbox state, or a parent's rolled-up children), postponed dimmed,
// and plain otherwise.
func taskLabel(display string, state store.ItemState, done bool) *qt.QLabel {
	text := html.EscapeString(display)
	switch {
	case done:
		text = fmt.Sprintf(`<s style="color:#888888">%s</s>`, text)
	case state == store.StatePostponed:
		text = fmt.Sprintf(`<span style="color:#888888">%s</span>`, text)
	}
	label := qt.NewQLabel3(text)
	label.SetTextFormat(qt.RichText)
	return label
}

// textButton makes a flat auto-raised text tool button for a node action.
func (w *mainWindow) textButton(text, tip string, handler func()) *qt.QWidget {
	btn := qt.NewQToolButton2()
	btn.SetText(text)
	btn.SetToolButtonStyle(qt.ToolButtonTextOnly)
	btn.SetToolTip(tip)
	btn.SetAutoRaise(true)
	btn.OnClicked(handler)
	return btn.QWidget
}

// taskActionButton makes an icon button that applies action to the plan item
// at index on date.
func (w *mainWindow) taskActionButton(icon *qt.QIcon, tip string, date time.Time, index int, action taskFunc) *qt.QWidget {
	btn := qt.NewQToolButton2()
	btn.SetIcon(icon)
	btn.SetToolButtonStyle(qt.ToolButtonIconOnly)
	btn.SetToolTip(tip)
	btn.SetAccessibleName(tip)
	btn.SetAutoRaise(true)
	btn.OnClicked(func() { w.runTaskAction(date, index, action) })
	return btn.QWidget
}

// elideText truncates s to maxRunes runes, appending "…" when it truncates.
func elideText(s string, maxRunes int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s, false
	}
	return string(runes[:maxRunes]) + "…", true
}

// --- node-key helpers ---

func taskKeyOf(task store.TreeTask) string {
	return "t:" + task.Date.Format(time.DateOnly) + ":" + strconv.Itoa(task.Index)
}

func decodeTaskKey(key string) (date time.Time, index int, ok bool) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[0] != "t" {
		return time.Time{}, 0, false
	}
	d, err := time.ParseInLocation(time.DateOnly, parts[1], time.Local)
	if err != nil {
		return time.Time{}, 0, false
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, 0, false
	}
	return d, idx, true
}

func keyOf(item *qt.QTreeWidgetItem) string {
	if item == nil {
		return ""
	}
	return item.Data(0, nodeKeyRole).ToString()
}

// currentTask returns the selected tree row's day and plan index when it is a
// task, for the item keyboard shortcuts.
func (w *mainWindow) currentTask() (date time.Time, index int, ok bool) {
	return decodeTaskKey(keyOf(w.tree.CurrentItem()))
}

// currentTaskText returns the currently selected task row's cached display
// text (cosmetic uses only, e.g. a notification message), or "" when the
// selection is not a task.
func (w *mainWindow) currentTaskText() string {
	item := w.tree.CurrentItem()
	if item == nil {
		return ""
	}
	return item.Data(0, taskTextRole).ToString()
}

// --- expand-state tracking ---

func (w *mainWindow) expandedOr(key string, def bool) bool {
	if v, ok := w.expanded[key]; ok {
		return v
	}
	return def
}

func (w *mainWindow) setExpanded(item *qt.QTreeWidgetItem, expanded bool) {
	if key := keyOf(item); key != "" {
		w.expanded[key] = expanded
	}
}

// --- CRUD row actions ---

// addProjectTask prompts for a name and adds it as a new top-level task on
// the viewed day's plan, tagged to projectID.
func (w *mainWindow) addProjectTask(projectID string) {
	if name, ok := w.promptText("New Task", "Task (added to the viewed day's plan):", ""); ok {
		if err := w.app.store.AddTaggedTask(w.viewedDate, name, projectID); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	}
}

// addSubtask prompts for a name and adds it as a new subtask nested under the
// plan item at index on date.
func (w *mainWindow) addSubtask(date time.Time, index int) {
	if name, ok := w.promptText("New Subtask", "Subtask:", ""); ok {
		if err := w.app.store.AddSubtask(date, index, name); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	}
}

func (w *mainWindow) renameProject(id, current string) {
	if name, ok := w.promptText("Rename Project", "Project name:", current); ok {
		if err := w.app.store.RenameProject(id, name); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	}
}

func (w *mainWindow) closeProject(id string) {
	if err := w.app.store.SetProjectStatus(id, store.StatusClosed); err != nil {
		w.app.reportError(err)
	}
	w.scheduleRefresh()
}

// --- drag & drop ---

// onDrop applies a drag: a task dropped on another task nests it as that
// task's subtask; dropped on a project (or Unfiled) it is re-homed to the top
// level and (re)tagged. We rebuild the tree ourselves and never call the base
// handler, so Qt does not also move the items.
func (w *mainWindow) onDrop(event *qt.QDropEvent) {
	// The custom per-row widgets make the drop event's own position unreliable
	// (it arrives in the row widget's local coordinates, so ItemAt always maps
	// it to the top row). Resolve the target from the real cursor position
	// mapped into the tree's viewport instead.
	_ = event
	global := qt.NewQPointF2(qt.QCursor_Pos())
	local := w.tree.Viewport().MapFromGlobal(global).ToPoint()
	src := w.dragSource()
	target := w.tree.ItemAt(local)
	slog.Debug("tree drop", "src", keyOf(src), "x", local.X(), "y", local.Y(), "target", keyOf(target))
	w.applyDrop(src, target)
}

// dragSource returns the item being dragged: the current item, or the first
// selected item as a fallback when the current pointer is stale.
func (w *mainWindow) dragSource() *qt.QTreeWidgetItem {
	if it := w.tree.CurrentItem(); keyOf(it) != "" {
		return it
	}
	if sel := w.tree.SelectedItems(); len(sel) > 0 {
		return sel[0]
	}
	return nil
}

// applyDrop dispatches a task drop: onto another task it nests as a subtask;
// onto a project or Unfiled it re-homes to the top level. Projects are not
// draggable (no reparenting), so any other source key is ignored.
func (w *mainWindow) applyDrop(src, target *qt.QTreeWidgetItem) {
	srcKey := keyOf(src)
	if !strings.HasPrefix(srcKey, "t:") {
		return
	}
	date, index, ok := decodeTaskKey(srcKey)
	if !ok {
		return
	}
	targetKey := keyOf(target)
	switch {
	case strings.HasPrefix(targetKey, "t:"):
		targetDate, targetIndex, ok := decodeTaskKey(targetKey)
		if !ok || !sameDay(date, targetDate) {
			return // MakeSubtask is same-day only
		}
		w.runTaskAction(date, index, func(d time.Time, idx int) error {
			return w.app.store.MakeSubtask(d, idx, targetIndex)
		})
	case strings.HasPrefix(targetKey, "p:"):
		projectID := strings.TrimPrefix(targetKey, "p:")
		w.runTaskAction(date, index, func(d time.Time, idx int) error {
			return w.app.store.MoveTaskToProject(d, idx, projectID)
		})
	case targetKey == "u:":
		w.runTaskAction(date, index, func(d time.Time, idx int) error {
			return w.app.store.MoveTaskToProject(d, idx, "")
		})
	}
}
