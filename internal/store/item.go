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

// Item is a single entry in a day's plan checklist. Depth is its nesting
// level (0 = top-level task, 1 = subtask of the preceding depth-0 task, etc.),
// derived from 2-space indentation on the stored markdown bullet.
type Item struct {
	Text  string
	State ItemState
	Depth int
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
// "- [>] text" (postponed), optionally preceded by leading spaces indicating
// its nesting Depth (2 spaces per level).
func parseItemLine(line string) (Item, error) {
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	rest := line[spaces:]
	const prefixLen = len("- [?] ")
	if len(rest) < prefixLen || !strings.HasPrefix(rest, "- [") || rest[4] != ']' || rest[5] != ' ' {
		return Item{}, fmt.Errorf("not a checkbox bullet: %q", line)
	}
	state, ok := markers[rest[3]]
	if !ok {
		return Item{}, fmt.Errorf("unknown checkbox marker %q in %q", rest[3], line)
	}
	text := strings.TrimSpace(rest[prefixLen:])
	if text == "" {
		return Item{}, fmt.Errorf("empty item text in %q", line)
	}
	return Item{Text: text, State: state, Depth: spaces / 2}, nil
}

func (i Item) render() string {
	marker, ok := stateMarkers[i.State]
	if !ok {
		// An unknown state (e.g. -1 from a UI selector with nothing checked)
		// must never fall through to the zero byte stateMarkers[i.State]
		// would otherwise yield: a NUL byte written into a markdown file
		// corrupts it and sends parsing into a permanent error loop. Fall
		// back to the todo marker instead.
		marker = ' '
	}
	return fmt.Sprintf("%s- [%c] %s", strings.Repeat("  ", i.Depth), marker, i.Text)
}
