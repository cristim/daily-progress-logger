package mobilecore

import (
	"log/slog"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// TreeJSON returns the Projects/tasks tree for date ("YYYY-MM-DD") as JSON
// matching projectTreeDTO (see dto.go for the complete schema).
// TreeTask.index in the returned JSON is the stable plan-file index to use for
// all subsequent task action calls on that date.
//
// Recurring tasks due on date are materialised before building the tree,
// exactly as the Qt desktop does in materializeViewedDate (mainwindow.go:240).
// Past dates are not mutated (store.MaterializeRecurring is a no-op for them).
func (c *Core) TreeJSON(date string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	d, err := parseDate(date)
	if err != nil {
		return "", err
	}
	// Materialise any recurring occurrences due on date so they appear in the
	// tree. A failure here is non-fatal: the tree is still returned minus any
	// un-materialised recurring tasks; the error is logged for diagnostics.
	if _, err := c.store.MaterializeRecurring(d); err != nil {
		slog.Warn("mobilecore: materialize recurring failed; tree may miss recurring tasks",
			"date", date, "error", err)
	}
	tree, err := c.store.BuildProjectTree(d)
	if err != nil {
		return "", err
	}
	return toJSON(mapProjectTree(tree))
}

// mapTreeTask converts a store.TreeTask (internal struct, no json tags) to the
// stable taskDTO wire type (snake_case keys, state as string, date as YYYY-MM-DD,
// empty children as [] not null).
func mapTreeTask(t store.TreeTask) taskDTO {
	children := make([]taskDTO, len(t.Children))
	for i, ch := range t.Children {
		children[i] = mapTreeTask(ch)
	}
	dateStr := ""
	if !t.Date.IsZero() {
		dateStr = t.Date.Format(dateLayout)
	}
	return taskDTO{
		Index:    t.Index,
		Depth:    t.Depth,
		Text:     t.Text,
		State:    stateString(t.State),
		Date:     dateStr,
		Done:     t.Done,
		Project:  t.Project,
		Children: children,
	}
}

// mapTreeTasks converts a []store.TreeTask slice, normalising nil to [].
func mapTreeTasks(tasks []store.TreeTask) []taskDTO {
	out := make([]taskDTO, len(tasks))
	for i, t := range tasks {
		out[i] = mapTreeTask(t)
	}
	return out
}

// mapProjectTree converts a *store.ProjectTree (internal struct) to the stable
// projectTreeDTO wire type.  All slice fields are always [], never null.
func mapProjectTree(tree *store.ProjectTree) projectTreeDTO {
	projects := make([]projectDTO, 0, len(tree.Projects))
	for _, p := range tree.Projects {
		tasks := mapTreeTasks(p.Tasks)
		projects = append(projects, projectDTO{
			ID:    p.ID,
			Name:  p.Name,
			Done:  p.Done,
			Tasks: tasks,
		})
	}

	recurring := make([]recurringTemplateDTO, len(tree.Recurring))
	for i, r := range tree.Recurring {
		recurring[i] = recurringTemplateDTO{
			Text:     r.Text,
			Project:  r.Project,
			Describe: r.Rec.Describe(),
			Kind:     int(r.Rec.Kind),
			Weekday:  int(r.Rec.Weekday),
			MonthDay: r.Rec.MonthDay,
			Hour:     r.Rec.Hour,
			Minute:   r.Rec.Minute,
			Raw:      r.Raw,
		}
	}

	return projectTreeDTO{
		Projects:  projects,
		Unfiled:   mapTreeTasks(tree.Unfiled),
		Recycled:  mapTreeTasks(tree.Recycled),
		Recurring: recurring,
	}
}
