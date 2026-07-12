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
// for Unfiled. taskTextRole additionally caches a task's or project's
// display text/name (used to seed the double-click and context-menu edit
// dialogs, and for cosmetic uses like a notification message; never used to
// resolve a store action, which always goes through the index in
// nodeKeyRole).
const (
	nodeKeyRole  = int(qt.UserRole)
	taskTextRole = int(qt.UserRole) + 1
)

// taskFunc applies a store operation to the plan item at idx on a given day.
type taskFunc func(date time.Time, idx int) error

// buildTree rebuilds the whole tree from the aggregated model, preserving each
// node's expand state (nodes not seen before default to expanded).
func (w *mainWindow) buildTree(model *store.ProjectTree) {
	// Clear drops every row widget, including any currently tracked as the
	// drop-highlighted row; forget it now rather than risk restoring a
	// stylesheet on (or otherwise touching) a widget Qt already destroyed.
	w.dropHighlightRow = nil
	w.dropHighlightStyle = ""
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
	item.SetData(0, taskTextRole, qt.NewQVariant11(p.Name))
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
	restore := w.textButton("Restore", "Restore to its day", func() {
		if err := w.app.store.RestoreTask(date, text); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	})
	del := w.textButton("Delete", "Delete permanently", func() {
		if err := w.app.store.PurgeRecycled(date, text); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	})
	layout.AddWidget(restore)
	layout.AddWidget(del)
	w.hoverReveal(row, restore, del)
	return row
}

// projectRow builds a project's row: a hover drag grip, its name (bold,
// struck through when done), plus Add-task / Rename / Close actions.
// Renaming is also reachable via double-click (see mainwindow.go's
// OnItemDoubleClicked) and the row's right-click context menu.
func (w *mainWindow) projectRow(p store.TreeProject) *qt.QWidget {
	row, layout := newRowWidget()
	layout.AddWidget2(nodeLabel(p.Name, p.Done, true).QWidget, 1)
	addTask := w.textButton("+ Task", "Add a task to this project (today's plan)", func() { w.addProjectTask(p.ID) })
	rename := w.textButton("Rename", "Rename this project", func() { w.renameProject(p.ID, p.Name) })
	closeBtn := w.textButton("Close", "Close (archive) this project", func() { w.closeProject(p.ID) })
	layout.AddWidget(addTask)
	layout.AddWidget(rename)
	layout.AddWidget(closeBtn)
	w.hoverReveal(row, addTask, rename, closeBtn)
	return row
}

// taskRow builds a task's row: a hover drag grip, a Done checkbox, and the
// task's text (stretched). The checkbox is interactive for a leaf task
// (toggling sets its own state) but disabled for a task with children, whose
// Done is rolled up from them and so is not directly togglable. Every other
// action (edit, add subtask, postpone, move to backlog, delete) lives in the
// row's right-click context menu (see showTaskContextMenu) and, for the
// selected row, the app's keyboard shortcuts (see shortcuts.go).
func (w *mainWindow) taskRow(task store.TreeTask) *qt.QWidget {
	row, layout := newRowWidget()
	gripCellW, grip := gripCell()
	layout.AddWidget(gripCellW)

	date, index, text := task.Date, task.Index, task.Text
	checkbox := qt.NewQCheckBox2()
	checkbox.SetToolTip("Done")
	// Block signals while setting the initial state so it does not re-enter
	// SetPlanItemState during construction.
	checkbox.BlockSignals(true)
	checkbox.SetChecked(task.Done)
	checkbox.BlockSignals(false)
	if len(task.Children) > 0 {
		checkbox.SetEnabled(false) // rolled-up from children; not directly togglable
	} else {
		checkbox.OnToggled(func(checked bool) {
			state := store.StateTodo
			if checked {
				state = store.StateDone
			}
			w.runTaskAction(date, index, func(d time.Time, idx int) error {
				return w.app.store.SetPlanItemState(d, idx, state)
			})
		})
	}
	layout.AddWidget(checkbox.QWidget)

	display, truncated := elideText(task.Text, 100)
	label := taskLabel(display, task.State, task.Done)
	if truncated {
		label.SetToolTip(task.Text)
	}
	layout.AddWidget2(label.QWidget, 1)

	// Hover-revealed per-row actions (also in the right-click menu / shortcuts).
	sub := w.textButton("+ Sub", "Add a subtask", func() { w.addSubtask(date, index) })
	nextDay := w.taskActionButton(postponeIcon(), "Postpone to the next day",
		date, index, w.app.store.PostponeToNextDay)
	nextWeek := w.taskActionButton(standardIcon(qt.QStyle__SP_ArrowUp), "Postpone to next week",
		date, index, w.app.store.PostponePlanItem)
	backlog := w.taskActionButton(backlogIcon(), "Move to the cross-week backlog",
		date, index, func(d time.Time, idx int) error {
			if err := w.app.store.MoveToBacklog(d, idx); err != nil {
				return err
			}
			w.app.notifyBacklogMove(text)
			return nil
		})
	del := w.taskActionButton(standardIcon(qt.QStyle__SP_TrashIcon), "Delete (moves to the recycle bin)",
		date, index, w.app.store.DeleteTask)
	for _, b := range []*qt.QWidget{sub, nextDay, nextWeek, backlog, del} {
		layout.AddWidget(b)
	}
	w.hoverReveal(row, grip, sub, nextDay, nextWeek, backlog, del)
	return row
}

// gripCell returns a fixed-width cell holding the drag-handle glyph, plus the
// glyph widget itself (which the caller adds to the row's hover-revealed set).
// The cell reserves the glyph's width so toggling the glyph's visibility never
// shifts the rest of the row. Dragging still originates from the tree view; the
// grip is purely a visual affordance and never captures the drag itself.
func gripCell() (cell, glyph *qt.QWidget) {
	c := qt.NewQWidget2()
	c.SetFixedWidth(16)
	l := qt.NewQHBoxLayout(c)
	l.SetContentsMargins(0, 0, 0, 0)
	g := qt.NewQLabel3("⠿")
	g.SetAlignment(qt.AlignCenter)
	g.SetStyleSheet("color: #999999;")
	g.SetToolTip("Drag to move")
	l.AddWidget(g.QWidget)
	return c, g.QWidget
}

// hoverReveal hides the given widgets and shows them only while container is
// hovered, so a resting row stays uncluttered and its actions (drag grip,
// per-row buttons) appear on mouse-over.
func (w *mainWindow) hoverReveal(container *qt.QWidget, widgets ...*qt.QWidget) {
	setVisible := func(v bool) {
		for _, wd := range widgets {
			wd.SetVisible(v)
		}
	}
	setVisible(false)
	container.OnEnterEvent(func(super func(event *qt.QEnterEvent), event *qt.QEnterEvent) {
		setVisible(true)
		super(event)
	})
	container.OnLeaveEvent(func(super func(event *qt.QEvent), event *qt.QEvent) {
		setVisible(false)
		super(event)
	})
}

// taskActionButton makes a flat icon button that applies action to the plan
// item at index on date (via runTaskAction), for the hover-revealed row
// actions.
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

// editTask prompts to replace the plan item at index on date's text (keeping
// its project tag and depth via EditTaskText), shared by double-click and the
// row's context-menu "Edit…" action.
func (w *mainWindow) editTask(date time.Time, index int, currentText string) {
	if name, ok := w.promptText("Edit Task", "Task:", currentText); ok {
		if err := w.app.store.EditTaskText(date, index, name); err != nil {
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

// --- context menu ---

// showContextMenu resolves the row under pos (a task or a project; any other
// row, including the section headers, gets no menu) and shows its actions.
// Wired from the tree's OnCustomContextMenuRequested in mainwindow.go.
func (w *mainWindow) showContextMenu(pos *qt.QPoint) {
	item := w.tree.ItemAt(pos)
	key := keyOf(item)
	global := w.tree.Viewport().MapToGlobalWithQPoint(pos)
	switch {
	case strings.HasPrefix(key, "t:"):
		w.showTaskContextMenu(item, key, global)
	case strings.HasPrefix(key, "p:"):
		w.showProjectContextMenu(item, strings.TrimPrefix(key, "p:"), global)
	}
}

// showTaskContextMenu builds the right-click menu for a task row, holding
// every action the simplified row no longer shows as its own button. The
// task's (date, index) and cached display text are resolved fresh from key
// and item at click time.
func (w *mainWindow) showTaskContextMenu(item *qt.QTreeWidgetItem, key string, global *qt.QPoint) {
	date, index, ok := decodeTaskKey(key)
	if !ok {
		return
	}
	text := item.Data(0, taskTextRole).ToString()

	menu := qt.NewQMenu2()
	addMenuAction(menu, "Edit…", func() { w.editTask(date, index, text) })
	addMenuAction(menu, "Add subtask…", func() { w.addSubtask(date, index) })
	addMenuAction(menu, "Postpone to next day", func() {
		w.runTaskAction(date, index, w.app.store.PostponeToNextDay)
	})
	addMenuAction(menu, "Postpone to next week", func() {
		w.runTaskAction(date, index, w.app.store.PostponePlanItem)
	})
	addMenuAction(menu, "Move to backlog", func() {
		w.runTaskAction(date, index, func(d time.Time, idx int) error {
			if err := w.app.store.MoveToBacklog(d, idx); err != nil {
				return err
			}
			w.app.notifyBacklogMove(text)
			return nil
		})
	})
	menu.AddSeparator()
	addMenuAction(menu, "Delete", func() { w.runTaskAction(date, index, w.app.store.DeleteTask) })
	menu.ExecWithPos(global)
}

// showProjectContextMenu mirrors the project row's own + Task / Rename /
// Close buttons, for parity with the task row's right-click menu.
func (w *mainWindow) showProjectContextMenu(item *qt.QTreeWidgetItem, id string, global *qt.QPoint) {
	name := item.Data(0, taskTextRole).ToString()
	menu := qt.NewQMenu2()
	addMenuAction(menu, "+ Task", func() { w.addProjectTask(id) })
	addMenuAction(menu, "Rename…", func() { w.renameProject(id, name) })
	addMenuAction(menu, "Close", func() { w.closeProject(id) })
	menu.ExecWithPos(global)
}

// --- drag & drop ---

// onDrop applies a drag: dropped ONTO a task it nests as that task's
// subtask, ONTO a project (or Unfiled) it is re-homed to the top level and
// (re)tagged, and dropped BETWEEN two tasks it is reordered to sit at that
// position (see applyDrop). We rebuild the tree ourselves and never call the
// base handler, so Qt does not also move the items.
func (w *mainWindow) onDrop(event *qt.QDropEvent) {
	_ = event
	local := w.dragPoint()
	src := w.dragSource()
	target := w.tree.ItemAt(local)
	zone := dropOnto
	if target != nil {
		zone = w.resolveDropZone(target, local)
	}
	slog.Debug("tree drop", "src", keyOf(src), "x", local.X(), "y", local.Y(), "target", keyOf(target), "zone", zone)
	w.applyDrop(src, target, zone)
	w.clearDropIndicator()
}

// dragPoint resolves the current drag/drop cursor position into the tree's
// viewport. The custom per-row widgets make the event's own position
// unreliable (it arrives in the row widget's local coordinates, so ItemAt
// always maps it to the top row), so the real cursor position is mapped from
// global screen coordinates instead. Shared by onDrop and the drag-move
// indicator (updateDropIndicator), which resolve the same target the same
// way.
func (w *mainWindow) dragPoint() *qt.QPoint {
	global := qt.NewQPointF2(qt.QCursor_Pos())
	return w.tree.Viewport().MapFromGlobal(global).ToPoint()
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

// --- drop indicator ---

// dropZone identifies where within a target row's rect a drag point falls,
// splitting the row into thirds: the top third means BETWEEN above the
// target, the bottom third BETWEEN below it, and the middle third ONTO it.
type dropZone int

const (
	dropOnto dropZone = iota
	dropAbove
	dropBelow
)

// resolveDropZone maps pt (already in viewport coordinates) to a dropZone
// against target's row. Shared by the live indicator (updateDropIndicator)
// and onDrop/applyDrop, so the drop always lands exactly where the indicator
// showed it would.
func (w *mainWindow) resolveDropZone(target *qt.QTreeWidgetItem, pt *qt.QPoint) dropZone {
	rect := w.tree.VisualItemRect(target)
	third := rect.Height() / 3
	relY := pt.Y() - rect.Y()
	switch {
	case relY < third:
		return dropAbove
	case relY >= rect.Height()-third:
		return dropBelow
	default:
		return dropOnto
	}
}

// updateDropIndicator recomputes and redraws the live drag indicator for pt
// (viewport coordinates): no target clears it, otherwise it renders as an
// onto-highlight or a between-line per resolveDropZone. Called from
// OnDragMoveEvent in mainwindow.go on every drag-move.
func (w *mainWindow) updateDropIndicator(pt *qt.QPoint) {
	target := w.tree.ItemAt(pt)
	if target == nil {
		w.clearDropIndicator()
		return
	}
	switch w.resolveDropZone(target, pt) {
	case dropAbove:
		w.showBetweenIndicator(target, false)
	case dropBelow:
		w.showBetweenIndicator(target, true)
	case dropOnto:
		w.showOntoIndicator(target)
	}
}

// showOntoIndicator highlights target's row (an accent background) to signal
// an ONTO drop, hiding any between-line and restoring a previously
// highlighted row first.
func (w *mainWindow) showOntoIndicator(target *qt.QTreeWidgetItem) {
	w.dropLine.Hide()
	row := w.tree.ItemWidget(target, 0)
	if row == w.dropHighlightRow {
		return
	}
	w.restoreDropHighlight()
	if row == nil {
		return
	}
	w.dropHighlightRow = row
	w.dropHighlightStyle = row.StyleSheet()
	row.SetStyleSheet("background-color: rgba(61, 126, 255, 60);")
}

// showBetweenIndicator shows the reusable drop line at the gap above (below
// == false) or below (below == true) target's row, indented to target's own
// depth so the user sees the level the drop will land at, restoring any
// onto-highlighted row first.
func (w *mainWindow) showBetweenIndicator(target *qt.QTreeWidgetItem, below bool) {
	w.restoreDropHighlight()
	rect := w.tree.VisualItemRect(target)
	y := rect.Y()
	if below {
		y = rect.Y() + rect.Height()
	}
	viewportWidth := w.tree.Viewport().Width()
	w.dropLine.SetGeometry(rect.X(), y-1, viewportWidth-rect.X(), 2)
	w.dropLine.Show()
	w.dropLine.Raise()
}

// restoreDropHighlight restores the currently onto-highlighted row (if any)
// to its stylesheet from before the highlight, then forgets it.
func (w *mainWindow) restoreDropHighlight() {
	if w.dropHighlightRow == nil {
		return
	}
	w.dropHighlightRow.SetStyleSheet(w.dropHighlightStyle)
	w.dropHighlightRow = nil
	w.dropHighlightStyle = ""
}

// clearDropIndicator hides the between-line and restores any onto-highlighted
// row, leaving no visible trace of the indicator. Called on drag-leave and
// after every drop so the indicator never leaks past the drag's end.
func (w *mainWindow) clearDropIndicator() {
	w.dropLine.Hide()
	w.restoreDropHighlight()
}

// applyDrop dispatches a task drop: onto another task it nests as a subtask;
// onto a project or Unfiled it re-homes to the top level. Projects are not
// draggable (no reparenting), so any other source key is ignored.
func (w *mainWindow) applyDrop(src, target *qt.QTreeWidgetItem, zone dropZone) {
	srcKey := keyOf(src)
	if !strings.HasPrefix(srcKey, "t:") {
		return // projects don't reorder here
	}
	date, index, ok := decodeTaskKey(srcKey)
	if !ok {
		return
	}
	targetKey := keyOf(target)

	// BETWEEN a task target reorders as its sibling at that position; BETWEEN
	// on a project/Unfiled header (or any other non-task target) falls
	// through to the ONTO handling below, appending into that container.
	if zone != dropOnto && strings.HasPrefix(targetKey, "t:") {
		targetDate, targetIndex, ok := decodeTaskKey(targetKey)
		if !ok || !sameDay(date, targetDate) {
			return // same viewed day only
		}
		w.runTaskAction(date, index, func(d time.Time, idx int) error {
			return w.app.store.ReorderTask(d, idx, targetIndex, zone == dropBelow)
		})
		return
	}

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
