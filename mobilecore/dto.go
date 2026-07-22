// This file is the STABLE wire contract for the iOS and Android host apps.
// Field names, types, and error-code strings here are FROZEN once host code
// ships; never rename a JSON key or remove a field without a major-version bump.
//
// # Conventions across all JSON payloads
//
//   - snake_case JSON keys everywhere.
//   - Task / goal state as a wire string: "todo", "done", or "postponed".
//   - Calendar dates as "YYYY-MM-DD" strings.
//   - Genuine timestamps (e.g. conflict detection time, OAuth token expiry) use
//     RFC 3339 and are explicitly noted in their field doc.
//   - Empty collections serialize as [] never null.
//
// # Error codes
//
// gomobile flattens Go errors to message strings (NSError / Exception), so
// hosts cannot call errors.Is across the boundary.  Every error returned by a
// Core method is prefixed with a stable code token followed by ": " and a
// human-readable detail.  Use ClassifyError to extract the code.
//
//	ErrCodeCASMismatch  - tree is stale; call TreeJSON and retry the action.
//	ErrCodeNotFound     - referenced item (project, backlog entry, …) does not exist.
//	ErrCodeBadInput     - invalid argument (bad state string, bad date, bad choice, …).
//	ErrCodeSyncAuth     - OAuth token is invalid or expired; host must re-authenticate.
//	ErrCodeInternal     - unexpected internal error (bug; please report).
//
// Example Swift usage:
//
//	let err = core.deleteTask(date, index, text)
//	if err?.localizedDescription.hasPrefix("CAS_MISMATCH") == true { refresh() }

package mobilecore

import "strings"

// --- Error codes ---------------------------------------------------------------

// Stable error-code prefixes embedded in every error returned from Core.
// Hosts match these with strings.HasPrefix(err.Error(), ErrCode<X>+": ").
const (
	ErrCodeCASMismatch = "CAS_MISMATCH"
	ErrCodeNotFound    = "NOT_FOUND"
	ErrCodeBadInput    = "BAD_INPUT"
	ErrCodeSyncAuth    = "SYNC_AUTH"
	ErrCodeInternal    = "INTERNAL"
)

// ClassifyError extracts the ErrCode* constant from a Core error message.
// Returns "" when the message does not carry a recognised code (treat as
// ErrCodeInternal in that case).
func ClassifyError(msg string) string {
	for _, code := range []string{
		ErrCodeCASMismatch, ErrCodeNotFound, ErrCodeBadInput,
		ErrCodeSyncAuth, ErrCodeInternal,
	} {
		if strings.HasPrefix(msg, code+": ") {
			return code
		}
	}
	return ""
}

// --- Tree DTOs ----------------------------------------------------------------

// taskDTO is one task in the day's plan (top-level or nested subtask).
//
// Wire shape:
//
//	{
//	  "index":    0,           // int   — plan-file index; pass back verbatim to task action calls
//	  "depth":    0,           // int   — nesting level (0=top-level, 1=subtask, …)
//	  "text":     "Buy milk",  // string — display text, project tag stripped
//	  "state":    "todo",      // string — "todo" | "done" | "postponed"
//	  "date":     "2026-07-20",// string — YYYY-MM-DD of the day this task belongs to
//	  "done":     false,       // bool  — rollup done state (true when all children done)
//	  "project":  "Work",      // string — project display name; OMITTED (not "") when empty
//	  "children": []           // []taskDTO — direct children; always [], never null
//	}
//
// Note: "project" carries omitempty — the key is absent from the JSON object
// when there is no project, rather than present as "". Treat it as optional.
type taskDTO struct {
	Index    int       `json:"index"`
	Depth    int       `json:"depth"`
	Text     string    `json:"text"`
	State    string    `json:"state"`
	Date     string    `json:"date"`
	Done     bool      `json:"done"`
	Project  string    `json:"project,omitempty"`
	Children []taskDTO `json:"children"`
}

// projectDTO is one open project in the day's tree.
//
// Wire shape:
//
//	{
//	  "id":    "abc123",    // string    — stable opaque project identifier
//	  "name":  "Work",      // string    — display name
//	  "done":  false,       // bool      — true when all tasks across all days are done
//	  "tasks": []           // []taskDTO — tasks for the requested day; always [], never null
//	}
type projectDTO struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Done  bool      `json:"done"`
	Tasks []taskDTO `json:"tasks"`
}

// recurringTemplateDTO is one recurring-task template in the tree response.
// Hosts use this for display; use RecurringJSON for the management view
// (add / remove templates).
//
// Wire shape:
//
//	{
//	  "text":      "Standup",        // string — display text (recurrence + project tags stripped)
//	  "project":   "work",           // string — project ID, or "" when untagged
//	  "describe":  "daily 09:00",    // string — human-readable schedule (e.g. "weekly Mon 09:30")
//	  "kind":      0,                // int    — cadence: 0=daily, 1=weekday, 2=weekly, 3=monthly
//	  "weekday":   1,                // int    — 0=Sun…6=Sat; meaningful when kind=2 (weekly)
//	  "month_day": 1,                // int    — 1-31; meaningful when kind=3 (monthly)
//	  "hour":      9,                // int    — time-of-day hour (0-23)
//	  "minute":    0,                // int    — time-of-day minute (0-59)
//	  "raw":       "Standup @daily …"// string — raw stored line; pass to RemoveRecurring to delete
//	}
type recurringTemplateDTO struct {
	Text     string `json:"text"`
	Project  string `json:"project"`
	Describe string `json:"describe"`
	Kind     int    `json:"kind"`
	Weekday  int    `json:"weekday"`
	MonthDay int    `json:"month_day"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Raw      string `json:"raw"`
}

// projectTreeDTO is the root response for TreeJSON.
//
// Wire shape:
//
//	{
//	  "projects":  [...],  // []projectDTO          — open projects with the day's tasks
//	  "unfiled":   [...],  // []taskDTO             — untagged tasks for the day
//	  "recycled":  [...],  // []taskDTO             — deleted tasks (date+project set, index not used)
//	  "recurring": [...]   // []recurringTemplateDTO — recurring-task templates
//	}
//
// All arrays are always [], never null.
type projectTreeDTO struct {
	Projects  []projectDTO           `json:"projects"`
	Unfiled   []taskDTO              `json:"unfiled"`
	Recycled  []taskDTO              `json:"recycled"`
	Recurring []recurringTemplateDTO `json:"recurring"`
}

// --- Sync DTOs ----------------------------------------------------------------

// conflictDTO is one unresolved sync conflict.
//
// Wire shape:
//
//	{
//	  "path":          "daily/2026/07/2026-07-20.md",
//	  "conflict_copy": "daily/2026/07/2026-07-20.conflict-phone.md",
//	  "time":          "2026-07-20T09:00:00Z"  // RFC 3339 — genuine timestamp
//	}
type conflictDTO struct {
	Path         string `json:"path"`
	ConflictCopy string `json:"conflict_copy"`
	Time         string `json:"time"` // RFC 3339 timestamp of conflict detection
}

// syncResultDTO is the response for SyncNow.
//
// Wire shape:
//
//	{
//	  "conflicts": [],   // []conflictDTO — new conflicts detected; [] when none
//	  "token":     "…"  // string        — updated OAuth JSON when the access token was
//	                    //                  refreshed mid-run; OMITTED (not "") when
//	                    //                  the token was not refreshed.
//	                    //                  IMPORTANT: when present, the host must persist
//	                    //                  this back to Keychain / EncryptedSharedPrefs
//	                    //                  immediately so the next sync starts with a
//	                    //                  valid token.
//	}
//
// Note: "token" carries omitempty — the key is absent when the token was not
// refreshed.  Hosts must treat it as optional, not as a fixed empty string.
type syncResultDTO struct {
	Conflicts []conflictDTO `json:"conflicts"`
	Token     string        `json:"token,omitempty"`
}

// --- Daily prompt DTO -----------------------------------------------------------

// dailyPromptDTO is the wire form of the user's daily prompt (the personal
// reminder shown at the top of the morning check-in).
//
// Wire shape for DailyPromptJSON:
//
//	{
//	  "text": "What will move the needle today?"  // string — "" means unset
//	}
type dailyPromptDTO struct {
	Text string `json:"text"`
}

// --- Mobile config DTO --------------------------------------------------------

// mobileConfigDTO holds the mobile-relevant configuration subset.
// Stored as JSON in <dataDir>/mobile-config.json, which syncs to Drive like
// all other data files.  Desktop-specific fields (data_dir, shortcuts,
// login_item_offered, etc.) are intentionally absent; Open's dataDir is the
// authoritative data location for mobile.
//
// Wire shape for ConfigJSON / SetConfig:
//
//	{
//	  "morning_time":    "09:30",     // "HH:MM" — when the morning check-in becomes due
//	  "evening_time":    "17:30",     // "HH:MM" — when the evening check-in becomes due
//	  "summary_day":     "Friday",    // weekday name — when the weekly summary dialog appears
//	  "summary_time":    "17:00",     // "HH:MM" — earliest time to show the summary
//	  "google_client_id":"…",         // OAuth client ID; empty disables sync
//	  "notify_checkins": true         // bool (nullable) — whether to send notification banners
//	}
//
// Missing / empty fields use the defaults documented in DuePromptsJSON.
type mobileConfigDTO struct {
	MorningTime    string `json:"morning_time,omitempty"`
	EveningTime    string `json:"evening_time,omitempty"`
	SummaryDay     string `json:"summary_day,omitempty"`
	SummaryTime    string `json:"summary_time,omitempty"`
	GoogleClientID string `json:"google_client_id,omitempty"`
	NotifyCheckins *bool  `json:"notify_checkins,omitempty"`
}
