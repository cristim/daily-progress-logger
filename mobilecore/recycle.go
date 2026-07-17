package mobilecore

import "github.com/cristim/daily-progress-logger/internal/store"

// recycleEntryJSON is the wire form of one recycle-bin entry.
type recycleEntryJSON struct {
	Date  string `json:"date"`  // YYYY-MM-DD of the day the item was deleted from
	Text  string `json:"text"`  // display text (project tag stripped)
	State string `json:"state"` // "todo", "done", or "postponed"
}

// RecycleJSON returns all recycle-bin entries as JSON.
// Use the date + text fields to call RestoreTask or PurgeRecycled.
func (c *Core) RecycleJSON() (string, error) {
	known, err := c.store.KnownProjectIDs()
	if err != nil {
		return "", err
	}
	bin, err := c.store.LoadRecycleBin()
	if err != nil {
		return "", err
	}
	out := make([]recycleEntryJSON, 0, len(bin))
	for _, e := range bin {
		out = append(out, recycleEntryJSON{
			Date:  e.Date.Format(dateLayout),
			Text:  store.DisplayText(e.Item, known),
			State: stateString(e.Item.State),
		})
	}
	return toJSON(out)
}

// RestoreTask returns the recycled task with displayText from date back to
// that day's plan. date is "YYYY-MM-DD". A missing entry is a no-op.
func (c *Core) RestoreTask(date, displayText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	return c.store.RestoreTask(d, displayText)
}

// PurgeRecycled permanently removes the recycled task with displayText from date.
// A missing entry is a no-op.
func (c *Core) PurgeRecycled(date, displayText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	return c.store.PurgeRecycled(d, displayText)
}
