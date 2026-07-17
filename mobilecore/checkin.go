package mobilecore

import (
	"encoding/json"
	"fmt"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// MorningCandidatesJSON returns the carry-over items to offer at the morning
// check-in as JSON. The result is an array of objects:
//
//	[{"text":"...", "from_backlog": false}, ...]
//
// NOTE: text may include the raw project tag (e.g. "#work") — this matches the
// Qt desktop behaviour and is intentional.  If the host strips tags for display,
// it should use the same tag-stripping logic as the tree view.
//
// Pass the adopted subset back to ApplyMorning.
func (c *Core) MorningCandidatesJSON(date string) (string, error) {
	d, err := parseDate(date)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	candidates, err := c.store.MorningCandidates(d)
	if err != nil {
		return "", err
	}
	type candidateJSON struct {
		Text        string `json:"text"`
		FromBacklog bool   `json:"from_backlog"`
	}
	out := make([]candidateJSON, len(candidates))
	for i, cand := range candidates {
		out[i] = candidateJSON{Text: cand.Text, FromBacklog: cand.FromBacklog}
	}
	return toJSON(out)
}

// morningDecisionInput is the JSON payload for ApplyMorning.
type morningDecisionInput struct {
	// NewItems are additional items not in the carry-over list.
	NewItems []string `json:"new_items"`
	// Adopted are the carry-over candidates the user chose to include today.
	Adopted []struct {
		Text        string `json:"text"`
		FromBacklog bool   `json:"from_backlog"`
	} `json:"adopted"`
}

// ApplyMorning records the morning check-in. decisionsJSON must be a JSON
// object with optional "new_items" ([]string) and "adopted" (array of
// {text, from_backlog} objects from MorningCandidatesJSON). Example:
//
//	{"new_items":["write tests"],"adopted":[{"text":"review PR","from_backlog":false}]}
func (c *Core) ApplyMorning(date, decisionsJSON string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var input morningDecisionInput
	if decisionsJSON != "" && decisionsJSON != "{}" {
		if err := json.Unmarshal([]byte(decisionsJSON), &input); err != nil {
			return fmt.Errorf("parsing morning decisions: %w", err)
		}
	}
	adopted := make([]store.Candidate, len(input.Adopted))
	for i, a := range input.Adopted {
		adopted[i] = store.Candidate{Text: a.Text, FromBacklog: a.FromBacklog}
	}
	return c.store.ApplyMorning(d, input.NewItems, adopted)
}

// eveningDecisionInput is the JSON payload for ApplyEvening.
type eveningDecisionInput struct {
	// Decisions maps each plan item to its disposition. Actions:
	//   0 = keep as todo, 1 = mark done, 2 = next day, 3 = next week, 4 = backlog
	Decisions []struct {
		Text   string `json:"text"`
		Action int    `json:"action"`
	} `json:"decisions"`
	// ExtraDone are additional done items not in the plan checklist.
	ExtraDone []string `json:"extra_done"`
}

// ApplyEvening records the evening check-in. decisionsJSON is a JSON object:
//
//	{
//	  "decisions": [{"text":"write tests","action":1}, ...],
//	  "extra_done": ["bonus thing I did"]
//	}
//
// Action values: 0=keep todo, 1=done, 2=next day, 3=next week, 4=backlog.
func (c *Core) ApplyEvening(date, decisionsJSON string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var input eveningDecisionInput
	if decisionsJSON != "" && decisionsJSON != "{}" {
		if err := json.Unmarshal([]byte(decisionsJSON), &input); err != nil {
			return fmt.Errorf("parsing evening decisions: %w", err)
		}
	}
	decisions := make([]store.EveningDecision, len(input.Decisions))
	for i, dec := range input.Decisions {
		action, err := parseEveningAction(dec.Action)
		if err != nil {
			return err
		}
		decisions[i] = store.EveningDecision{Text: dec.Text, Action: action}
	}
	return c.store.ApplyEvening(d, decisions, input.ExtraDone)
}

func parseEveningAction(n int) (store.EveningAction, error) {
	switch n {
	case 0:
		return store.EveningActionTodo, nil
	case 1:
		return store.EveningActionDone, nil
	case 2:
		return store.EveningActionNextDay, nil
	case 3:
		return store.EveningActionNextWeek, nil
	case 4:
		return store.EveningActionBacklog, nil
	}
	return 0, fmt.Errorf("unknown evening action %d (0=todo,1=done,2=next_day,3=next_week,4=backlog)", n)
}
