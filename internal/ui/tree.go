package ui

import (
	"fmt"
	"html"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// nodeKeyRole is the item-data role holding each tree node's identity string:
// "p:<projectID>", "s:<storyID>", "t:<date>:<text>", or "u:" for Unfiled.
const nodeKeyRole = int(qt.UserRole)

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
}

func (w *mainWindow) addProjectNode(p store.TreeProject) {
	key := "p:" + p.ID
	item := qt.NewQTreeWidgetItem()
	item.SetData(0, nodeKeyRole, qt.NewQVariant11(key))
	w.tree.AddTopLevelItem(item)
	w.tree.SetItemWidget(item, 0, w.projectRow(p))
	for _, st := range p.Stories {
		w.addStoryNode(item, st)
	}
	item.SetExpanded(w.expandedOr(key, true))
}

func (w *mainWindow) addStoryNode(parent *qt.QTreeWidgetItem, st store.TreeStory) {
	key := "s:" + st.ID
	item := qt.NewQTreeWidgetItem6(parent)
	item.SetData(0, nodeKeyRole, qt.NewQVariant11(key))
	w.tree.SetItemWidget(item, 0, w.storyRow(st))
	for _, task := range st.Tasks {
		w.addTaskNode(item, task)
	}
	item.SetExpanded(w.expandedOr(key, true))
}

func (w *mainWindow) addTaskNode(parent *qt.QTreeWidgetItem, task store.TreeTask) {
	item := qt.NewQTreeWidgetItem6(parent)
	item.SetData(0, nodeKeyRole, qt.NewQVariant11(taskKeyOf(task)))
	w.tree.SetItemWidget(item, 0, w.taskRow(task))
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

// projectRow builds a project's row: its name (bold, struck through when done)
// plus Add-story / Rename / Close actions.
func (w *mainWindow) projectRow(p store.TreeProject) *qt.QWidget {
	row, layout := newRowWidget()
	layout.AddWidget2(nodeLabel(p.Name, p.Done, true).QWidget, 1)
	layout.AddWidget(w.textButton("+ Story", "Add a story to this project", func() { w.addStory(p.ID) }))
	layout.AddWidget(w.textButton("Rename", "Rename this project", func() { w.renameProject(p.ID, p.Name) }))
	layout.AddWidget(w.textButton("Close", "Close (archive) this project", func() { w.closeProject(p.ID) }))
	return row
}

// storyRow builds a story's row: its name (struck through when done) plus
// Add-task / Rename / Close actions.
func (w *mainWindow) storyRow(st store.TreeStory) *qt.QWidget {
	row, layout := newRowWidget()
	layout.AddWidget2(nodeLabel(st.Name, st.Done, false).QWidget, 1)
	layout.AddWidget(w.textButton("+ Task", "Add a task to this story (today's plan)", func() { w.addTask(st.ID) }))
	layout.AddWidget(w.textButton("Rename", "Rename this story", func() { w.renameStory(st.ID, st.Name) }))
	layout.AddWidget(w.textButton("Close", "Close (archive) this story", func() { w.closeStory(st.ID) }))
	return row
}

// taskRow builds a task's row: its text plus the Done/Not-done selector and the
// next-day / next-week / backlog defer buttons, all acting on the task's own day.
func (w *mainWindow) taskRow(task store.TreeTask) *qt.QWidget {
	row, layout := newRowWidget()

	display, truncated := elideText(task.Text, 100)
	label := taskLabel(display, task.State)
	if truncated {
		label.SetToolTip(task.Text)
	}
	layout.AddWidget2(label.QWidget, 1)

	date, text := task.Date, task.Text
	selector := newStateSelector(task.State)
	selector.onChanged(func(state store.ItemState) {
		w.runTaskAction(date, text, func(d time.Time, idx int) error {
			return w.app.store.SetPlanItemState(d, idx, state)
		})
	})
	layout.AddWidget(selector.widget)
	layout.AddWidget(w.taskActionButton(postponeIcon(), "Postpone to the next day", date, text,
		w.app.store.PostponeToNextDay))
	layout.AddWidget(w.taskActionButton(standardIcon(qt.QStyle__SP_ArrowUp), "Postpone to next week", date, text,
		w.app.store.PostponePlanItem))
	layout.AddWidget(w.taskActionButton(backlogIcon(), "Move to the cross-week backlog", date, text,
		func(d time.Time, idx int) error {
			if err := w.app.store.MoveToBacklog(d, idx); err != nil {
				return err
			}
			w.app.notifyBacklogMove(text)
			return nil
		}))
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

// nodeLabel builds a rich-text label for a project/story name, bolded when bold
// and struck through + dimmed when done.
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

// taskLabel styles a task's text by state: done struck through and dimmed,
// postponed dimmed, todo plain.
func taskLabel(display string, state store.ItemState) *qt.QLabel {
	text := html.EscapeString(display)
	switch state {
	case store.StateDone:
		text = fmt.Sprintf(`<s style="color:#888888">%s</s>`, text)
	case store.StatePostponed:
		text = fmt.Sprintf(`<span style="color:#888888">%s</span>`, text)
	case store.StateTodo:
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

// taskActionButton makes an icon button that resolves the task freshly (by day +
// text) and applies action.
func (w *mainWindow) taskActionButton(icon *qt.QIcon, tip string, date time.Time, text string, action taskFunc) *qt.QWidget {
	btn := qt.NewQToolButton2()
	btn.SetIcon(icon)
	btn.SetToolButtonStyle(qt.ToolButtonIconOnly)
	btn.SetToolTip(tip)
	btn.SetAccessibleName(tip)
	btn.SetAutoRaise(true)
	btn.OnClicked(func() { w.runTaskAction(date, text, action) })
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
	return "t:" + task.Date.Format(time.DateOnly) + ":" + task.Text
}

func decodeTaskKey(key string) (date time.Time, text string, ok bool) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[0] != "t" {
		return time.Time{}, "", false
	}
	d, err := time.ParseInLocation(time.DateOnly, parts[1], time.Local)
	if err != nil {
		return time.Time{}, "", false
	}
	return d, parts[2], true
}

func keyOf(item *qt.QTreeWidgetItem) string {
	if item == nil {
		return ""
	}
	return item.Data(0, nodeKeyRole).ToString()
}

// currentTask returns the selected tree row when it is a task, for the item
// keyboard shortcuts.
func (w *mainWindow) currentTask() (time.Time, string, bool) {
	return decodeTaskKey(keyOf(w.tree.CurrentItem()))
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

func (w *mainWindow) addStory(projectID string) {
	if name, ok := w.promptText("New Story", "Story name:", ""); ok {
		if _, err := w.app.store.AddStory(projectID, name); err != nil {
			w.app.reportError(err)
		}
		w.refresh()
	}
}

func (w *mainWindow) addTask(storyID string) {
	if name, ok := w.promptText("New Task", "Task (added to the viewed day's plan):", ""); ok {
		if err := w.app.store.AddTaggedTask(w.viewedDate, name, storyID); err != nil {
			w.app.reportError(err)
		}
		w.refresh()
	}
}

func (w *mainWindow) renameProject(id, current string) {
	if name, ok := w.promptText("Rename Project", "Project name:", current); ok {
		if err := w.app.store.RenameProject(id, name); err != nil {
			w.app.reportError(err)
		}
		w.refresh()
	}
}

func (w *mainWindow) renameStory(id, current string) {
	if name, ok := w.promptText("Rename Story", "Story name:", current); ok {
		if err := w.app.store.RenameStory(id, name); err != nil {
			w.app.reportError(err)
		}
		w.refresh()
	}
}

func (w *mainWindow) closeProject(id string) {
	if err := w.app.store.SetProjectStatus(id, store.StatusClosed); err != nil {
		w.app.reportError(err)
	}
	w.refresh()
}

func (w *mainWindow) closeStory(id string) {
	if err := w.app.store.SetStoryStatus(id, store.StatusClosed); err != nil {
		w.app.reportError(err)
	}
	w.refresh()
}

// --- drag & drop ---

// onDrop applies a drag: a task dropped on a story is re-tagged (on Unfiled it is
// untagged); a story dropped on a project is reparented. We rebuild the tree
// ourselves and never call the base handler, so Qt does not also move the items.
func (w *mainWindow) onDrop(event *qt.QDropEvent) {
	w.applyDrop(w.tree.CurrentItem(), w.tree.ItemAt(event.Pos()))
}

func (w *mainWindow) applyDrop(src, target *qt.QTreeWidgetItem) {
	srcKey := keyOf(src)
	switch {
	case strings.HasPrefix(srcKey, "t:"):
		w.dropTask(srcKey, target)
	case strings.HasPrefix(srcKey, "s:"):
		w.dropStory(strings.TrimPrefix(srcKey, "s:"), target)
	}
}

func (w *mainWindow) dropTask(srcKey string, target *qt.QTreeWidgetItem) {
	date, text, ok := decodeTaskKey(srcKey)
	if !ok {
		return
	}
	storyID, unfiled, ok := storyTarget(target)
	if !ok {
		return
	}
	w.runTaskAction(date, text, func(d time.Time, idx int) error {
		if unfiled {
			return w.app.store.UnassignTaskStory(d, idx)
		}
		return w.app.store.AssignTaskStory(d, idx, storyID)
	})
}

func (w *mainWindow) dropStory(storyID string, target *qt.QTreeWidgetItem) {
	projectID, ok := projectTarget(target)
	if !ok {
		return
	}
	if err := w.app.store.MoveStory(storyID, projectID); err != nil {
		w.app.reportError(err)
	}
	w.scheduleRefresh()
}

// storyTarget resolves the story a task was dropped onto: a story node directly,
// a task's parent story, or the Unfiled node (unfiled=true).
func storyTarget(target *qt.QTreeWidgetItem) (storyID string, unfiled, ok bool) {
	key := keyOf(target)
	switch {
	case strings.HasPrefix(key, "s:"):
		return strings.TrimPrefix(key, "s:"), false, true
	case key == "u:":
		return "", true, true
	case strings.HasPrefix(key, "t:"):
		return storyTarget(target.Parent())
	}
	return "", false, false
}

// projectTarget resolves the project a story was dropped onto: a project node
// directly, or the parent project of a story/task node.
func projectTarget(target *qt.QTreeWidgetItem) (string, bool) {
	key := keyOf(target)
	switch {
	case strings.HasPrefix(key, "p:"):
		return strings.TrimPrefix(key, "p:"), true
	case strings.HasPrefix(key, "s:"), strings.HasPrefix(key, "t:"):
		return projectTarget(target.Parent())
	}
	return "", false
}
