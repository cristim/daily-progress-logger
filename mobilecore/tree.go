package mobilecore

import "log/slog"

// TreeJSON returns the Projects/tasks tree for date ("YYYY-MM-DD") as JSON
// (see store.ProjectTree). TreeTask.Index in the returned JSON is the stable
// plan-file index to use for all subsequent task action calls.
//
// Recurring tasks due on date are materialised before building the tree,
// exactly as the Qt desktop does in materializeViewedDate (mainwindow.go:240).
// Past dates are not mutated (store.MaterializeRecurring is a no-op for them).
func (c *Core) TreeJSON(date string) (string, error) {
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
	return toJSON(tree)
}
