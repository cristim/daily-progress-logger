package mobilecore

// recurringTaskJSON is the wire form of a recurring template.
type recurringTaskJSON struct {
	Text    string `json:"text"`    // display text (project tag stripped)
	Project string `json:"project"` // project ID, "" if untagged
	Raw     string `json:"raw"`     // raw stored line (use for RemoveRecurring)
}

// RecurringJSON returns all recurring templates as JSON.
// Each element has text (display), project (project ID, may be ""), and raw
// (the raw stored line — pass raw back to RemoveRecurring to delete a template).
func (c *Core) RecurringJSON() (string, error) {
	tasks, err := c.store.RecurringTasks()
	if err != nil {
		return "", err
	}
	out := make([]recurringTaskJSON, len(tasks))
	for i, t := range tasks {
		out[i] = recurringTaskJSON{Text: t.Text, Project: t.Project, Raw: t.Raw}
	}
	return toJSON(out)
}

// AddRecurring stores text as a recurring template. text must include a
// recurrence keyword (@daily, @weekly @mon @09:00, etc.).
// Returns an error when text carries no valid recurrence tag.
func (c *Core) AddRecurring(text string) error {
	return c.store.AddRecurring(text)
}

// RemoveRecurring deletes the first recurring template whose raw text matches
// rawText (as returned by RecurringJSON's "raw" field).
func (c *Core) RemoveRecurring(rawText string) error {
	return c.store.RemoveRecurring(rawText)
}
