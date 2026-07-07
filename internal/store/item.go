package store

import (
	"fmt"
	"strings"
)

// ItemState is the lifecycle state of a plan item.
type ItemState int

const (
	// StateTodo is a planned item not yet completed.
	StateTodo ItemState = iota
	// StateDone is a completed item.
	StateDone
	// StatePostponed is an item pushed to next week's backlog.
	StatePostponed
)

// Item is a single entry in a day's plan checklist.
type Item struct {
	Text  string
	State ItemState
}

// markers maps checkbox markers to states; the reverse of stateMarkers.
var markers = map[byte]ItemState{
	' ': StateTodo,
	'x': StateDone,
	'X': StateDone,
	'>': StatePostponed,
}

var stateMarkers = map[ItemState]byte{
	StateTodo:      ' ',
	StateDone:      'x',
	StatePostponed: '>',
}

// parseItemLine parses a checkbox bullet like "- [ ] text", "- [x] text" or
// "- [>] text" (postponed).
func parseItemLine(line string) (Item, error) {
	const prefixLen = len("- [?] ")
	if len(line) < prefixLen || !strings.HasPrefix(line, "- [") || line[4] != ']' || line[5] != ' ' {
		return Item{}, fmt.Errorf("not a checkbox bullet: %q", line)
	}
	state, ok := markers[line[3]]
	if !ok {
		return Item{}, fmt.Errorf("unknown checkbox marker %q in %q", line[3], line)
	}
	text := strings.TrimSpace(line[prefixLen:])
	if text == "" {
		return Item{}, fmt.Errorf("empty item text in %q", line)
	}
	return Item{Text: text, State: state}, nil
}

func (i Item) render() string {
	return fmt.Sprintf("- [%c] %s", stateMarkers[i.State], i.Text)
}
