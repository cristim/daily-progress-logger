package mobilecore

// BacklogJSON returns the backlog as JSON with "current" and "next_week" arrays.
func (c *Core) BacklogJSON() (string, error) {
	b, err := c.store.LoadBacklog()
	if err != nil {
		return "", err
	}
	type backlogJSON struct {
		Current  []string `json:"current"`
		NextWeek []string `json:"next_week"`
	}
	out := backlogJSON{
		Current:  b.Current,
		NextWeek: b.NextWeek,
	}
	if out.Current == nil {
		out.Current = []string{}
	}
	if out.NextWeek == nil {
		out.NextWeek = []string{}
	}
	return toJSON(out)
}

// AdoptFromBacklog adds text to today's plan and removes it from the backlog.
// date is "YYYY-MM-DD".
func (c *Core) AdoptFromBacklog(date, text string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	return c.store.AdoptFromBacklog(d, text)
}

// MoveBacklogItem moves text between backlog sections.
// toNextWeek=true moves from Current to Next week; false moves back.
func (c *Core) MoveBacklogItem(text string, toNextWeek bool) error {
	return c.store.MoveBacklogItem(text, toNextWeek)
}
