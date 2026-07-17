// Package main provides dpl, a command-line companion for the Daily Progress
// Logger. It reads and writes the same markdown files as the GUI app, using
// the pure-Go internal/store package (no Qt, no cgo).
//
// Build:  CGO_ENABLED=0 go build ./cmd/dpl
// Usage:  dpl help
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/store"
)

const (
	dateFormat = "2006-01-02"

	usageText = `dpl — Daily Progress Logger CLI

Usage: dpl [--data-dir PATH] [--date YYYY-MM-DD] <subcommand> [flags] [args]

The CLI and GUI share the same data directory and files.  All file writes are
atomic, so running both concurrently is safe.

Global flags:
  --data-dir PATH    Override data directory (default: from config)
  --date YYYY-MM-DD  Target date (default: today in local time)

Subcommands:
  list [--json]
        Show the day's plan, 1-based numbered.  --json emits a JSON array.
  add [--project SLUG] [--parent N] <text...>
        Add a task.  --project tags it to a project; --parent N adds it as a
        subtask of item N.  Flags must precede text.  Combining --parent and
        --project is an error (subtasks inherit the parent's project).
  done <n>
        Mark item N done ([x]).
  undone <n>
        Mark item N to-do ([ ]).
  edit <n> <text...>
        Replace item N's text (project tag is preserved unless new text already
        ends with a known #tag).
  rm <n>
        Delete item N (moved to recycle bin, recoverable via the GUI).
  postpone <n> [--week]
        Move item N to tomorrow's plan.  --week marks it postponed ([>]) and
        queues it in next week's backlog instead.
  backlog <n>
        Move item N out of today's plan into the current backlog.
  backlog list
        Show the backlog (Current and Next week sections).
  projects
        List all projects (id, name, status).
  project add <name...>
        Create a new project and print its generated id.
  recur list
        List recurring task templates.
  help
        Show this help text.

Index notes:
  Item numbers are 1-based, matching the output of 'dpl list'.  Numbers shift
  after add/rm, so re-check the list before chaining commands.

Build note:
  CGO_ENABLED=0 go build ./cmd/dpl   (pure Go, no Qt dependency)
`
)

// exitError signals a non-zero exit code.  A zero msg means the error was
// already written elsewhere.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

// usageErr returns an exitError for a bad-argument condition (exit 2).
func usageErr(msg string) error { return &exitError{code: 2, msg: msg} }

// printUsage writes the usage text to w, ignoring any write error since usage
// is only ever printed to stdout/stderr where write failures are unrecoverable.
func printUsage(w io.Writer) { _, _ = fmt.Fprint(w, usageText) }

// subHandler is the signature shared by all subcommand functions.
type subHandler func(st *store.Store, date time.Time, args []string, w io.Writer) error

// subcommands maps each subcommand name to its handler, enabling a simple
// O(1) dispatch in run() without a sprawling switch.
var subcommands = map[string]subHandler{
	"list":     cmdList,
	"add":      cmdAdd,
	"done":     cmdDone,
	"undone":   cmdUndone,
	"edit":     cmdEdit,
	"rm":       cmdRm,
	"postpone": cmdPostpone,
	"backlog":  cmdBacklog,
	"projects": func(st *store.Store, _ time.Time, _ []string, w io.Writer) error {
		return cmdProjects(st, w)
	},
	"project": func(st *store.Store, _ time.Time, args []string, w io.Writer) error {
		return cmdProject(st, args, w)
	},
	"recur": func(st *store.Store, _ time.Time, args []string, w io.Writer) error {
		return cmdRecur(st, args, w)
	},
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var ec *exitError
		if errors.As(err, &ec) {
			if ec.msg != "" {
				fmt.Fprintln(os.Stderr, "dpl:", ec.msg)
			}
			os.Exit(ec.code)
		}
		fmt.Fprintln(os.Stderr, "dpl:", err)
		os.Exit(1)
	}
}

// run is the entry point used by main and by tests.
func run(args []string, w, errW io.Writer) error {
	fs := flag.NewFlagSet("dpl", flag.ContinueOnError)
	fs.SetOutput(errW)
	dataDir := fs.String("data-dir", "", "override data directory (default: from config)")
	dateStr := fs.String("date", "", "target date YYYY-MM-DD (default: today)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(w)
			return nil
		}
		return usageErr(err.Error())
	}

	subcmdArgs := fs.Args()
	if len(subcmdArgs) == 0 || subcmdArgs[0] == "help" {
		printUsage(w)
		return nil
	}

	subcommand := subcmdArgs[0]
	rest := subcmdArgs[1:]

	handler, ok := subcommands[subcommand]
	if !ok {
		_, _ = fmt.Fprintf(errW, "dpl: unknown subcommand %q\n\n%s", subcommand, usageText)
		return &exitError{code: 2}
	}

	// Resolve date (midnight local).
	var date time.Time
	if *dateStr == "" {
		now := time.Now()
		date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	} else {
		var err error
		date, err = time.ParseInLocation(dateFormat, *dateStr, time.Local)
		if err != nil {
			return fmt.Errorf("invalid --date %q: %w", *dateStr, err)
		}
	}

	dir := *dataDir
	if dir == "" {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		dir = cfg.DataDir
	}
	st, err := store.New(dir)
	if err != nil {
		return fmt.Errorf("opening store at %s: %w", dir, err)
	}
	return handler(st, date, rest, w)
}

// loadPlan returns the flat plan slice for date; nil when the file does not
// exist yet (not an error).
func loadPlan(st *store.Store, date time.Time) ([]store.Item, error) {
	d, _, err := st.LoadDaily(date)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, nil
	}
	return d.Plan, nil
}

// stateGlyph maps an item state to its checkbox display string.
func stateGlyph(s store.ItemState) string {
	switch s {
	case store.StateTodo:
		return "[ ]"
	case store.StateDone:
		return "[x]"
	case store.StatePostponed:
		return "[>]"
	}
	return "[ ]"
}

// stateName returns the JSON-friendly name of an item state.
func stateName(s store.ItemState) string {
	switch s {
	case store.StateTodo:
		return "todo"
	case store.StateDone:
		return "done"
	case store.StatePostponed:
		return "postponed"
	}
	return "todo"
}

// printPlan writes the plan in human-readable numbered form.
func printPlan(w io.Writer, items []store.Item, date time.Time) {
	if len(items) == 0 {
		_, _ = fmt.Fprintf(w, "No tasks for %s.\n", date.Format(dateFormat))
		return
	}
	for i, item := range items {
		indent := strings.Repeat("  ", item.Depth)
		_, _ = fmt.Fprintf(w, "%3d %s%s %s\n", i+1, indent, stateGlyph(item.State), item.Text)
	}
}

// afterMutation reloads and prints the day's plan to confirm the change.
func afterMutation(st *store.Store, date time.Time, w io.Writer) error {
	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("reloading plan after mutation: %w", err)
	}
	printPlan(w, items, date)
	return nil
}

// resolveIndex converts a 1-based user number to a 0-based plan index.
func resolveIndex(items []store.Item, n int) (int, error) {
	if n < 1 || n > len(items) {
		return 0, fmt.Errorf("item %d out of range (plan has %d items, numbered 1-%d)", n, len(items), len(items))
	}
	return n - 1, nil
}

// parseN parses a positive integer from a string (1-based item number).
func parseN(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("expected a positive item number, got %q", s)
	}
	return n, nil
}

// extractProjectID returns the project ID embedded in text as a trailing
// #slug or @slug that matches a known project ID, or "" when none.
func extractProjectID(text string, known map[string]bool) string {
	trimmed := strings.TrimRight(text, " \t")
	space := strings.LastIndexByte(trimmed, ' ')
	last := trimmed[space+1:] // whole string when there is no space
	var candidate string
	switch {
	case strings.HasPrefix(last, "#"):
		candidate = last[1:]
	case strings.HasPrefix(last, "@"):
		candidate = last[1:]
	}
	if candidate != "" && known[candidate] {
		return candidate
	}
	return ""
}

// jsonItem is the JSON representation of one plan entry.
type jsonItem struct {
	Number  int    `json:"number"`
	Text    string `json:"text"`  // display text with project tag stripped
	State   string `json:"state"` // "todo", "done", or "postponed"
	Depth   int    `json:"depth"`
	Project string `json:"project"` // project id, or ""
}

// printPlanJSON writes the plan as a pretty-printed JSON array.
func printPlanJSON(w io.Writer, items []store.Item, st *store.Store) error {
	known, err := st.KnownProjectIDs()
	if err != nil {
		return fmt.Errorf("loading projects for JSON output: %w", err)
	}
	out := make([]jsonItem, len(items))
	for i, item := range items {
		project := ""
		if item.Depth == 0 {
			project = extractProjectID(item.Text, known)
		}
		out[i] = jsonItem{
			Number:  i + 1,
			Text:    store.DisplayText(item, known),
			State:   stateName(item.State),
			Depth:   item.Depth,
			Project: project,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// cmdList implements: dpl list [--json].
func cmdList(st *store.Store, date time.Time, args []string, w io.Writer) error {
	fs := flag.NewFlagSet("dpl list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "output as a JSON array")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(w)
			return nil
		}
		return usageErr("list: " + err.Error())
	}
	if fs.NArg() > 0 {
		return usageErr("list: unexpected arguments: " + strings.Join(fs.Args(), " "))
	}

	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("loading plan for %s: %w", date.Format(dateFormat), err)
	}
	if *jsonOut {
		return printPlanJSON(w, items, st)
	}
	printPlan(w, items, date)
	return nil
}

// cmdAdd implements: dpl add [--project SLUG] [--parent N] <text...>.
//
// --parent N and --project SLUG cannot be combined: subtasks always inherit
// their depth-0 ancestor's project tag and do not carry one themselves.
func cmdAdd(st *store.Store, date time.Time, args []string, w io.Writer) error {
	fs := flag.NewFlagSet("dpl add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	projectSlug := fs.String("project", "", "project id to tag the new task with")
	parentN := fs.Int("parent", 0, "add as a subtask of item N (1-based)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(w)
			return nil
		}
		return usageErr("add: " + err.Error())
	}
	if fs.NArg() == 0 {
		return usageErr("add: requires <text...>")
	}
	if *parentN > 0 && *projectSlug != "" {
		return usageErr("add: --parent and --project cannot both be specified (subtasks inherit their depth-0 ancestor's project)")
	}
	if err := doAdd(st, date, *parentN, *projectSlug, strings.Join(fs.Args(), " ")); err != nil {
		return err
	}
	return afterMutation(st, date, w)
}

// doAdd performs the store write for cmdAdd once flags are validated.
func doAdd(st *store.Store, date time.Time, parentN int, projectSlug, text string) error {
	switch {
	case parentN > 0:
		items, err := loadPlan(st, date)
		if err != nil {
			return fmt.Errorf("loading plan: %w", err)
		}
		idx, err := resolveIndex(items, parentN)
		if err != nil {
			return err
		}
		if err := st.AddSubtask(date, idx, text); err != nil {
			return fmt.Errorf("adding subtask: %w", err)
		}
	case projectSlug != "":
		// Validate the project slug before calling the store so the error
		// message distinguishes a typo from a genuine I/O failure.
		known, err := st.KnownProjectIDs()
		if err != nil {
			return fmt.Errorf("loading projects: %w", err)
		}
		if !known[projectSlug] {
			return fmt.Errorf("project %q not found (run 'dpl projects' to list known ids)", projectSlug)
		}
		if err := st.AddTaggedTask(date, text, projectSlug); err != nil {
			return fmt.Errorf("adding tagged task: %w", err)
		}
	default:
		if err := st.AddPlanItem(date, text); err != nil {
			return fmt.Errorf("adding task: %w", err)
		}
	}
	return nil
}

// setItemState is the shared logic for done and undone.
func setItemState(st *store.Store, date time.Time, nStr string, state store.ItemState, w io.Writer) error {
	n, err := parseN(nStr)
	if err != nil {
		return usageErr(err.Error())
	}
	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	idx, err := resolveIndex(items, n)
	if err != nil {
		return err
	}
	if err := st.SetPlanItemState(date, idx, state); err != nil {
		return fmt.Errorf("setting item state: %w", err)
	}
	return afterMutation(st, date, w)
}

// cmdDone implements: dpl done <n>.
func cmdDone(st *store.Store, date time.Time, args []string, w io.Writer) error {
	if len(args) != 1 {
		return usageErr("done: requires exactly one argument <n>")
	}
	return setItemState(st, date, args[0], store.StateDone, w)
}

// cmdUndone implements: dpl undone <n>.
func cmdUndone(st *store.Store, date time.Time, args []string, w io.Writer) error {
	if len(args) != 1 {
		return usageErr("undone: requires exactly one argument <n>")
	}
	return setItemState(st, date, args[0], store.StateTodo, w)
}

// cmdEdit implements: dpl edit <n> <text...>.
func cmdEdit(st *store.Store, date time.Time, args []string, w io.Writer) error {
	if len(args) < 2 {
		return usageErr("edit: requires <n> and <text...>")
	}
	n, err := parseN(args[0])
	if err != nil {
		return usageErr("edit: " + err.Error())
	}
	text := strings.Join(args[1:], " ")

	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	idx, err := resolveIndex(items, n)
	if err != nil {
		return err
	}
	if err := st.EditTaskText(date, idx, text); err != nil {
		return fmt.Errorf("editing task: %w", err)
	}
	return afterMutation(st, date, w)
}

// cmdRm implements: dpl rm <n>.
func cmdRm(st *store.Store, date time.Time, args []string, w io.Writer) error {
	if len(args) != 1 {
		return usageErr("rm: requires exactly one argument <n>")
	}
	n, err := parseN(args[0])
	if err != nil {
		return usageErr("rm: " + err.Error())
	}
	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	idx, err := resolveIndex(items, n)
	if err != nil {
		return err
	}
	deleted := items[idx].Text
	if err := st.DeleteTask(date, idx); err != nil {
		return fmt.Errorf("deleting task: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Deleted %q (moved to recycle bin).\n", deleted)
	return afterMutation(st, date, w)
}

// cmdPostpone implements: dpl postpone <n> [--week].
//
// --week may appear before or after <n> since stdlib flag.Parse stops at
// the first non-flag argument.
func cmdPostpone(st *store.Store, date time.Time, args []string, w io.Writer) error {
	// Separate --week / -week from positional args so the flag can appear
	// anywhere (e.g. "postpone 2 --week" as well as "postpone --week 2").
	week := false
	var positional []string
	for _, a := range args {
		if a == "--week" || a == "-week" {
			week = true
		} else {
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return usageErr("postpone: requires exactly one argument <n>")
	}
	n, err := parseN(positional[0])
	if err != nil {
		return usageErr("postpone: " + err.Error())
	}
	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	idx, err := resolveIndex(items, n)
	if err != nil {
		return err
	}
	if week {
		if err := st.PostponePlanItem(date, idx); err != nil {
			return fmt.Errorf("postponing to next week: %w", err)
		}
		_, _ = fmt.Fprintf(w, "Item %d marked [>] and queued in next week's backlog.\n", n)
	} else {
		if err := st.PostponeToNextDay(date, idx); err != nil {
			return fmt.Errorf("postponing to next day: %w", err)
		}
		next := date.AddDate(0, 0, 1)
		_, _ = fmt.Fprintf(w, "Item %d moved to %s.\n", n, next.Format(dateFormat))
	}
	return afterMutation(st, date, w)
}

// cmdBacklog implements: dpl backlog <n>  AND  dpl backlog list.
func cmdBacklog(st *store.Store, date time.Time, args []string, w io.Writer) error {
	if len(args) == 0 {
		return usageErr("backlog: requires 'list' or <n>")
	}
	if args[0] == "list" {
		if len(args) > 1 {
			return usageErr("backlog list: unexpected arguments")
		}
		return cmdBacklogList(st, w)
	}
	// Move item N to backlog.
	n, err := parseN(args[0])
	if err != nil {
		return usageErr("backlog: " + err.Error())
	}
	items, err := loadPlan(st, date)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	idx, err := resolveIndex(items, n)
	if err != nil {
		return err
	}
	text := items[idx].Text
	if err := st.MoveToBacklog(date, idx); err != nil {
		return fmt.Errorf("moving to backlog: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Moved %q to current backlog.\n", text)
	return afterMutation(st, date, w)
}

// cmdBacklogList prints the backlog (Current and Next week).
func cmdBacklogList(st *store.Store, w io.Writer) error {
	b, err := st.LoadBacklog()
	if err != nil {
		return fmt.Errorf("loading backlog: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Current (%d):\n", len(b.Current))
	for _, text := range b.Current {
		_, _ = fmt.Fprintf(w, "  - %s\n", text)
	}
	_, _ = fmt.Fprintf(w, "\nNext week (%d):\n", len(b.NextWeek))
	for _, text := range b.NextWeek {
		_, _ = fmt.Fprintf(w, "  - %s\n", text)
	}
	return nil
}

// cmdProjects implements: dpl projects.
func cmdProjects(st *store.Store, w io.Writer) error {
	projects, err := st.LoadProjects()
	if err != nil {
		return fmt.Errorf("loading projects: %w", err)
	}
	if len(projects) == 0 {
		_, _ = fmt.Fprintln(w, "No projects.")
		return nil
	}
	for _, p := range projects {
		_, _ = fmt.Fprintf(w, "%-24s %-30s %s\n", p.ID, p.Name, p.Status)
	}
	return nil
}

// cmdProject dispatches project sub-subcommands.
func cmdProject(st *store.Store, args []string, w io.Writer) error {
	if len(args) == 0 {
		return usageErr("project: requires a sub-subcommand (add)")
	}
	switch args[0] {
	case "add":
		return cmdProjectAdd(st, args[1:], w)
	default:
		return usageErr(fmt.Sprintf("project: unknown sub-subcommand %q (try: add)", args[0]))
	}
}

// cmdProjectAdd implements: dpl project add <name...>.
func cmdProjectAdd(st *store.Store, args []string, w io.Writer) error {
	if len(args) == 0 {
		return usageErr("project add: requires <name...>")
	}
	name := strings.Join(args, " ")
	id, err := st.AddProject(name)
	if err != nil {
		return fmt.Errorf("adding project: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Created project %q  id: %s\n", name, id)
	return nil
}

// cmdRecur dispatches recur sub-subcommands.
func cmdRecur(st *store.Store, args []string, w io.Writer) error {
	if len(args) == 0 {
		return usageErr("recur: requires a sub-subcommand (list)")
	}
	switch args[0] {
	case "list":
		return cmdRecurList(st, w)
	default:
		return usageErr(fmt.Sprintf("recur: unknown sub-subcommand %q (try: list)", args[0]))
	}
}

// cmdRecurList implements: dpl recur list.
func cmdRecurList(st *store.Store, w io.Writer) error {
	tasks, err := st.RecurringTasks()
	if err != nil {
		return fmt.Errorf("loading recurring tasks: %w", err)
	}
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(w, "No recurring tasks.")
		return nil
	}
	for i, t := range tasks {
		project := ""
		if t.Project != "" {
			project = " #" + t.Project
		}
		_, _ = fmt.Fprintf(w, "%3d  %s%s  [%s]\n", i+1, t.Text, project, t.Rec.Describe())
	}
	return nil
}
