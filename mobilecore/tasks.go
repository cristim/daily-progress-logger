package mobilecore

// AddTask adds a task to date's plan. When projectID is non-empty the task is
// tagged to that project.
func (c *Core) AddTask(date, text, projectID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if projectID == "" {
		return c.store.AddPlanItem(d, text)
	}
	return c.store.AddTaggedTask(d, text, projectID)
}

// SetTaskState sets a task's state ("todo", "done", or "postponed") by its
// index in the day's plan. expectedText is the display text (project tag
// stripped) captured when the caller last fetched TreeJSON; the core verifies
// it matches before acting. Returns ErrCASMismatch when the tree is stale.
func (c *Core) SetTaskState(date string, index int, expectedText, state string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	st, err := parseState(state)
	if err != nil {
		return err
	}
	return c.store.SetPlanItemState(d, index, st)
}

// DeleteTask moves the plan item at index to the recycle bin. expectedText
// guards against acting on the wrong item when the tree is stale.
func (c *Core) DeleteTask(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.DeleteTask(d, index)
}

// EditTaskText replaces the display text of the task at index, preserving its
// project tag. expectedText guards against stale-tree mutations.
func (c *Core) EditTaskText(date string, index int, expectedText, newText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.EditTaskText(d, index, newText)
}

// PostponeToNextDay moves the task at index to tomorrow's plan.
// expectedText guards against stale-tree mutations.
func (c *Core) PostponeToNextDay(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.PostponeToNextDay(d, index)
}

// PostponeToNextWeek marks the task at index postponed ("[>]") and queues it
// in the next-week backlog. expectedText guards against stale-tree mutations.
func (c *Core) PostponeToNextWeek(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.PostponePlanItem(d, index)
}

// MoveTaskToBacklog removes the task at index from the day and adds it to the
// backlog's Current list. expectedText guards against stale-tree mutations.
func (c *Core) MoveTaskToBacklog(date string, index int, expectedText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.MoveToBacklog(d, index)
}

// ReorderTask moves the task at srcIndex to sit immediately before (below=false)
// or after (below=true) the task at refIndex. expectedSrcText guards the source
// task; expectedRefText guards the reference task.
func (c *Core) ReorderTask(date string, srcIndex int, expectedSrcText string, refIndex int, expectedRefText string, below bool) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, srcIndex, expectedSrcText); err != nil {
		return err
	}
	if err := c.verifyIndex(d, refIndex, expectedRefText); err != nil {
		return err
	}
	return c.store.ReorderTask(d, srcIndex, refIndex, below)
}

// MoveTaskToProject re-homes the task at index under projectID (empty string
// clears the tag, leaving the task in Unfiled). expectedText guards against
// stale-tree mutations.
func (c *Core) MoveTaskToProject(date string, index int, expectedText, projectID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, index, expectedText); err != nil {
		return err
	}
	return c.store.MoveTaskToProject(d, index, projectID)
}

// AssignTaskProject is a convenience alias for MoveTaskToProject with a
// non-empty projectID.
func (c *Core) AssignTaskProject(date string, index int, expectedText, projectID string) error {
	return c.MoveTaskToProject(date, index, expectedText, projectID)
}

// UnassignTaskProject removes the project tag from the task at index, moving
// it to Unfiled.
func (c *Core) UnassignTaskProject(date string, index int, expectedText string) error {
	return c.MoveTaskToProject(date, index, expectedText, "")
}

// AddSubtask inserts a new todo as the last child of the task at parentIndex.
// expectedParentText guards against stale-tree mutations.
func (c *Core) AddSubtask(date string, parentIndex int, expectedParentText, text string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, parentIndex, expectedParentText); err != nil {
		return err
	}
	return c.store.AddSubtask(d, parentIndex, text)
}

// MakeSubtask nests the task at childIndex as the last child of parentIndex.
// Both expectedChildText and expectedParentText guard against stale-tree mutations.
func (c *Core) MakeSubtask(date string, childIndex int, expectedChildText string, parentIndex int, expectedParentText string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if err := c.verifyIndex(d, childIndex, expectedChildText); err != nil {
		return err
	}
	if err := c.verifyIndex(d, parentIndex, expectedParentText); err != nil {
		return err
	}
	return c.store.MakeSubtask(d, childIndex, parentIndex)
}
