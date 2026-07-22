package mobilecore

// DailyPromptJSON returns the user's daily prompt as JSON.
// The schema is dailyPromptDTO (see dto.go): a single "text" field, "" when
// unset. The prompt is stored in <dataDir>/daily-prompt.md, which syncs to
// Drive like all other data files.
func (c *Core) DailyPromptJSON() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, err := c.store.DailyPrompt()
	if err != nil {
		return "", codeStoreErr(err)
	}
	return toJSON(dailyPromptDTO{Text: p})
}

// SetDailyPrompt replaces the daily prompt with text. Passing "" (or a
// whitespace-only string) clears it back to unset.
func (c *Core) SetDailyPrompt(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return codeStoreErr(c.store.SetDailyPrompt(text))
}
