package mobilecore

// recurringTaskDTO is the wire form of a recurring template for RecurringJSON.
// This is the management view (add / remove templates).  For the richer display
// view (with schedule description and structured fields) see recurringTemplateDTO
// in dto.go, which is embedded in the TreeJSON response.
type recurringTaskDTO struct {
	Text    string `json:"text"`    // display text (project tag stripped)
	Project string `json:"project"` // project ID, "" if untagged
	Raw     string `json:"raw"`     // raw stored line (pass to RemoveRecurring)
}

// RecurringJSON returns all recurring templates as JSON.
// Each element has text (display), project (project ID, may be ""), and raw
// (the raw stored line — pass raw back to RemoveRecurring to delete a template).
func (c *Core) RecurringJSON() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tasks, err := c.store.RecurringTasks()
	if err != nil {
		return "", err
	}
	out := make([]recurringTaskDTO, len(tasks))
	for i, t := range tasks {
		out[i] = recurringTaskDTO{Text: t.Text, Project: t.Project, Raw: t.Raw}
	}
	return toJSON(out)
}

// AddRecurring stores text as a recurring template. text must include a
// recurrence keyword (@daily, @weekly @mon @09:00, etc.).
// Returns an error when text carries no valid recurrence tag.
func (c *Core) AddRecurring(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store.AddRecurring(text)
}

// RemoveRecurring deletes the first recurring template whose raw text matches
// rawText (as returned by RecurringJSON's "raw" field).
func (c *Core) RemoveRecurring(rawText string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store.RemoveRecurring(rawText)
}
