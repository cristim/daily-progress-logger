package store

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func date(s string) time.Time {
	t, err := time.ParseInLocation(dateLayout, s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseItemLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		line       string
		want       Item
		wantRender string // defaults to line
		wantErr    string
	}{
		{name: "todo", line: "- [ ] write tests", want: Item{Text: "write tests", State: StateTodo}},
		{name: "done lowercase", line: "- [x] ship it", want: Item{Text: "ship it", State: StateDone}},
		{name: "done uppercase", line: "- [X] ship it", want: Item{Text: "ship it", State: StateDone}, wantRender: "- [x] ship it"},
		{name: "postponed", line: "- [>] refactor", want: Item{Text: "refactor", State: StatePostponed}},
		{name: "plain bullet", line: "- no checkbox", wantErr: "not a checkbox bullet"},
		{name: "unknown marker", line: "- [?] what", wantErr: "unknown checkbox marker"},
		{name: "empty text", line: "- [ ] ", wantErr: "empty item text"},
		{name: "whitespace text", line: "- [ ]  ", wantErr: "empty item text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseItemLine(tt.line)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			wantRender := tt.wantRender
			if wantRender == "" {
				wantRender = tt.line
			}
			assert.Equal(t, wantRender, got.render(), "render should round-trip")
		})
	}
}

// TestItemRenderUnknownStateNeverEmitsNUL verifies that render() falls back
// to the todo marker for any state outside the known set (e.g. -1, which
// CheckedId() returns when a UI selector has nothing checked), instead of
// formatting the zero byte as a "%c" verb and writing a NUL into the file.
func TestItemRenderUnknownStateNeverEmitsNUL(t *testing.T) {
	t.Parallel()
	for _, state := range []ItemState{-1, ItemState(99)} {
		t.Run(fmt.Sprintf("state_%d", state), func(t *testing.T) {
			t.Parallel()
			got := Item{Text: "some goal", State: state}.render()
			assert.NotContains(t, got, "\x00", "render must never emit a NUL byte")
			assert.Equal(t, "- [ ] some goal", got, "unknown state should fall back to the todo marker")
		})
	}
}

func TestWeekID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		date      string
		want      string
		wantStart string
		wantEnd   string
	}{
		{date: "2026-07-07", want: "2026-W28", wantStart: "2026-07-06", wantEnd: "2026-07-12"},
		{date: "2026-01-01", want: "2026-W01", wantStart: "2025-12-29", wantEnd: "2026-01-04"},
		{date: "2027-01-01", want: "2026-W53", wantStart: "2026-12-28", wantEnd: "2027-01-03"},
	}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			t.Parallel()
			w := WeekOf(date(tt.date))
			assert.Equal(t, tt.want, w.String())
			assert.Equal(t, tt.wantStart, w.Start().Format(dateLayout))
			assert.Equal(t, tt.wantEnd, w.End().Format(dateLayout))
		})
	}
}

func TestWeekIDBefore(t *testing.T) {
	t.Parallel()
	assert.True(t, WeekID{2025, 52}.Before(WeekID{2026, 1}))
	assert.True(t, WeekID{2026, 27}.Before(WeekID{2026, 28}))
	assert.False(t, WeekID{2026, 28}.Before(WeekID{2026, 28}))
	assert.False(t, WeekID{2026, 29}.Before(WeekID{2026, 28}))
}

func TestDailyRoundTrip(t *testing.T) {
	t.Parallel()
	d := &Daily{
		Date:        date("2026-07-07"),
		MorningDone: true,
		EveningDone: true,
		Plan: []Item{
			{Text: "fix parser", State: StateDone},
			{Text: "write docs", State: StateTodo},
			{Text: "big refactor", State: StatePostponed},
		},
		Done: []string{"fix parser", "helped Ana debug prod issue"},
	}
	rendered := d.render()
	parsed, err := parseDaily(rendered)
	require.NoError(t, err)
	assert.Equal(t, d, parsed)
	assert.Contains(t, rendered, "week: 2026-W28")
	assert.Contains(t, rendered, "day: Tuesday")
	assert.Contains(t, rendered, "# Tuesday, 7 July 2026")
}

func TestDailyRoundTripEmpty(t *testing.T) {
	t.Parallel()
	d := &Daily{Date: date("2026-07-07")}
	parsed, err := parseDaily(d.render())
	require.NoError(t, err)
	assert.Equal(t, d, parsed)
}

func TestParseDailyErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "no frontmatter", content: "# hello\n", wantErr: "missing frontmatter opening"},
		{name: "unclosed frontmatter", content: "---\ndate: 2026-07-07\n", wantErr: "missing frontmatter closing"},
		{name: "missing date", content: "---\nmorning_done: true\n---\n", wantErr: `missing required frontmatter key "date"`},
		{name: "bad date", content: "---\ndate: tomorrow\n---\n", wantErr: "invalid date"},
		{name: "unknown key", content: "---\ndate: 2026-07-07\nmood: great\n---\n", wantErr: `unknown frontmatter key "mood"`},
		{name: "bad bool", content: "---\ndate: 2026-07-07\nmorning_done: yes\n---\n", wantErr: "invalid boolean"},
		{name: "unknown section", content: "---\ndate: 2026-07-07\n---\n\n## Notes\n", wantErr: `unknown section "Notes"`},
		{name: "stray text", content: "---\ndate: 2026-07-07\n---\n\nhello\n", wantErr: "unexpected content outside sections"},
		{name: "bad plan bullet", content: "---\ndate: 2026-07-07\n---\n\n## Plan\n\n- loose bullet\n", wantErr: "not a checkbox bullet"},
		{name: "bad done line", content: "---\ndate: 2026-07-07\n---\n\n## Done\n\nplain text\n", wantErr: "expected a bullet in Done section"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseDaily(tt.content)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestBacklogRoundTrip(t *testing.T) {
	t.Parallel()
	b := &Backlog{
		Current:  []string{"update runbooks", "clean up alerts"},
		NextWeek: []string{"quarterly report"},
	}
	parsed, err := parseBacklog(b.render())
	require.NoError(t, err)
	assert.Equal(t, b, parsed)

	empty := &Backlog{}
	parsed, err = parseBacklog(empty.render())
	require.NoError(t, err)
	assert.Equal(t, empty, parsed)
}

func TestBacklogOps(t *testing.T) {
	t.Parallel()
	b := &Backlog{Current: []string{"a"}, NextWeek: []string{"b", "a"}}
	b.addCurrent("a") // duplicate ignored
	assert.Equal(t, []string{"a"}, b.Current)
	b.rollOver()
	assert.Equal(t, []string{"a", "b"}, b.Current)
	assert.Empty(t, b.NextWeek)
	b.removeCurrent("a")
	assert.Equal(t, []string{"b"}, b.Current)
}

func TestWeeklyRenderAndMeta(t *testing.T) {
	t.Parallel()
	week := WeekID{Year: 2026, Week: 28}
	dailies := []*Daily{
		{
			Date: date("2026-07-06"),
			Plan: []Item{
				{Text: "deploy service", State: StateDone},
				{Text: "flaky test", State: StateTodo},
			},
			Done: []string{"unplanned incident response"},
		},
		{
			Date: date("2026-07-07"),
			Plan: []Item{
				{Text: "flaky test", State: StateTodo}, // dup deduped in summary
				{Text: "big refactor", State: StatePostponed},
			},
		},
	}
	meta := weeklyMeta{Reviewed: true, Dropped: []string{"old idea"}}
	content := renderWeekly(week, dailies, meta)

	assert.Contains(t, content, "# Week 28, 2026 (6 Jul - 12 Jul)")
	assert.Contains(t, content, "### Monday, 6 July")
	assert.Contains(t, content, "- deploy service")
	assert.Contains(t, content, "- unplanned incident response")
	assert.Contains(t, content, "## Not completed\n\n- flaky test")
	assert.Equal(t, 1, strings.Count(content, "- flaky test"), "duplicate open items should be deduplicated")
	assert.Contains(t, content, "## Postponed\n\n- big refactor")
	assert.Contains(t, content, "## Dropped at review\n\n- old idea")

	parsed, err := parseWeeklyMeta(content)
	require.NoError(t, err)
	assert.Equal(t, meta, parsed)
}

// TestWeeklyMetaNormalizesPostponedPlanGoal verifies that a week-plan goal
// hand-edited to carry the postponed marker ("- [>]") is normalized to
// StateTodo on parse, since the weekly-plan dialog's goal selector only
// offers two states (planned / done): leaving the state as postponed would
// round-trip into a selector with no checked button (CheckedId() == -1),
// which previously wrote a NUL byte via Item.render() and corrupted the
// weekly file into a permanent parse-error loop.
func TestWeeklyMetaNormalizesPostponedPlanGoal(t *testing.T) {
	t.Parallel()
	content := "---\n" +
		"week: 2026-W28\n" +
		"start: 2026-07-06\n" +
		"end: 2026-07-12\n" +
		"reviewed: false\n" +
		"summarized: false\n" +
		"planned: true\n" +
		"---\n\n" +
		"## Week plan\n\n" +
		"- [>] hand-edited postponed goal\n" +
		"- [ ] ordinary goal\n"

	meta, err := parseWeeklyMeta(content)
	require.NoError(t, err)
	require.Len(t, meta.Plan, 2)
	assert.Equal(t, Item{Text: "hand-edited postponed goal", State: StateTodo}, meta.Plan[0],
		"a stray postponed marker on a week-plan goal must normalize to todo")
	assert.Equal(t, Item{Text: "ordinary goal", State: StateTodo}, meta.Plan[1])

	// Re-rendering must produce clean, parseable markdown with no NUL bytes.
	week := WeekID{Year: 2026, Week: 28}
	rendered := renderWeekly(week, nil, meta)
	assert.NotContains(t, rendered, "\x00")
	assert.Contains(t, rendered, "- [ ] hand-edited postponed goal")

	reparsed, err := parseWeeklyMeta(rendered)
	require.NoError(t, err)
	assert.Equal(t, meta, reparsed, "re-parsing the re-rendered content must be stable")
}

// TestWeeklyRenderDedupDoneAndPlan verifies that when a checked plan item and
// a Done-section bullet carry the same text, the weekly markdown emits that
// text exactly once.
func TestWeeklyRenderDedupDoneAndPlan(t *testing.T) {
	t.Parallel()
	week := WeekID{Year: 2026, Week: 28}
	dailies := []*Daily{
		{
			Date: date("2026-07-06"), // Monday
			Plan: []Item{
				{Text: "review Ana's PR", State: StateDone},
			},
			// User also typed the same accomplishment in the free-text evening field.
			Done: []string{"review Ana's PR", "fixed deploy pipeline"},
		},
	}
	content := renderWeekly(week, dailies, weeklyMeta{})

	assert.Equal(t, 1, strings.Count(content, "review Ana's PR"),
		"same-text checked item and Done bullet must appear exactly once in Done section")
	assert.Contains(t, content, "- fixed deploy pipeline")
}

// TestDoneByDay verifies ordering, deduplication, and empty-day omission.
func TestDoneByDay(t *testing.T) {
	t.Parallel()

	t.Run("checked_plan_before_done_bullets", func(t *testing.T) {
		t.Parallel()
		dailies := []*Daily{
			{
				Date: date("2026-07-06"),
				Plan: []Item{
					{Text: "ship feature", State: StateDone},
				},
				Done: []string{"helped on-call"},
			},
		}
		got := DoneByDay(dailies)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"ship feature", "helped on-call"}, got[0].Items,
			"checked plan items must come before Done bullets")
	})

	t.Run("dedup_same_text_in_plan_and_done", func(t *testing.T) {
		t.Parallel()
		dailies := []*Daily{
			{
				Date: date("2026-07-06"),
				Plan: []Item{
					{Text: "review Ana's PR", State: StateDone},
				},
				Done: []string{"review Ana's PR"},
			},
		}
		got := DoneByDay(dailies)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"review Ana's PR"}, got[0].Items,
			"same text in checked plan and Done must appear exactly once")
	})

	t.Run("dedup_case_and_whitespace", func(t *testing.T) {
		t.Parallel()
		dailies := []*Daily{
			{
				Date: date("2026-07-06"),
				Plan: []Item{
					{Text: "Fix flaky test", State: StateDone},
				},
				Done: []string{"fix  flaky   test"},
			},
		}
		got := DoneByDay(dailies)
		require.Len(t, got, 1)
		assert.Len(t, got[0].Items, 1, "normalized duplicate must appear only once")
		assert.Equal(t, "Fix flaky test", got[0].Items[0], "original (plan) text must be preserved")
	})

	t.Run("empty_day_omitted", func(t *testing.T) {
		t.Parallel()
		dailies := []*Daily{
			{
				Date: date("2026-07-06"),
				Plan: []Item{
					{Text: "todo item", State: StateTodo},
				},
				Done: nil,
			},
		}
		got := DoneByDay(dailies)
		assert.Empty(t, got, "day with no done items must be omitted from result")
	})

	t.Run("multiple_days_order_preserved", func(t *testing.T) {
		t.Parallel()
		dailies := []*Daily{
			{Date: date("2026-07-06"), Plan: []Item{{Text: "mon task", State: StateDone}}},
			{Date: date("2026-07-07"), Plan: []Item{{Text: "tue task", State: StateDone}}},
		}
		got := DoneByDay(dailies)
		require.Len(t, got, 2)
		assert.Equal(t, date("2026-07-06"), got[0].Date)
		assert.Equal(t, date("2026-07-07"), got[1].Date)
	})
}
