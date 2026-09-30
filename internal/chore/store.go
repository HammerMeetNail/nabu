package chore

import (
	"context"
	"regexp"
	"time"
)

// quietTimeRE matches 24h "HH:MM" (00:00–23:59).
var quietTimeRE = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ValidQuietTime reports whether s is a well-formed "HH:MM" time-of-day
// (empty is handled by callers; a quiet window only exists when both bounds
// are provided).
func ValidQuietTime(s string) bool {
	return quietTimeRE.MatchString(s)
}

// QuietHoursComplete reports whether the (start, end) pair is a usable quiet
// window: both bounds set and well-formed.
func QuietHoursComplete(start, end string) bool {
	return start != "" && end != "" && ValidQuietTime(start) && ValidQuietTime(end)
}

type Chore struct {
	ID                  int64     `json:"id"`
	HouseholdID         int64     `json:"householdId"`
	Name                string    `json:"name"`
	Icon                string    `json:"icon"`
	Color               string    `json:"color"`
	SortOrder           int       `json:"sortOrder"`
	Category            string    `json:"category"`
	IsPredefined        bool      `json:"isPredefined"`
	PredefinedKey       string    `json:"predefinedKey"`
	CreatedBy           *int64    `json:"createdBy"`
	CreatedAt           time.Time `json:"createdAt"`
	IndicatorLabels     []string  `json:"indicatorLabels"`
	IndicatorDefaults   []string  `json:"indicatorDefaults"`
	HasVolumeML         bool      `json:"hasVolumeML"`
	FollowUpEnabled     bool      `json:"followUpEnabled"`
	LastFollowUpMinutes int       `json:"lastFollowUpMinutes"`
	HasRating           bool      `json:"hasRating"`
	// MetricType is the explicit per-chore metric configuration (Phase 3):
	// "none" | "amount" | "rating" | "duration". It generalizes the older
	// HasVolumeML/HasRating booleans, which are kept in sync (see SyncMetricFlags)
	// for backward compatibility with the stats/log layers.
	MetricType string `json:"metricType"`
	// MetricUnit is the display unit label for an "amount" metric (e.g. "mL",
	// "oz", "g", "min"). Empty for non-amount metrics.
	MetricUnit string `json:"metricUnit"`
	// Subjects is an optional list of subject tags for this chore (e.g. twin
	// names), used to distinguish which subject a log is about (Phase 5.5).
	Subjects []string `json:"subjects"`
	// Visibility controls who can see the chore. "household" (default) is
	// visible to all household members; "admins" is visible only to owners and
	// admins (private household tasks).
	Visibility string `json:"visibility"`
	// QuietHoursStart/QuietHoursEnd ("HH:MM", empty = unset) define the
	// household-level quiet window for this chore: schedule reminders are
	// suppressed for every member whose local time falls inside the window.
	QuietHoursStart string `json:"quietHoursStart,omitempty"`
	QuietHoursEnd   string `json:"quietHoursEnd,omitempty"`
}

// Metric type constants for Chore.MetricType.
const (
	MetricNone     = "none"
	MetricAmount   = "amount"
	MetricRating   = "rating"
	MetricDuration = "duration"
)

// Visibility constants for Chore.Visibility.
const (
	VisibilityHousehold = "household"
	VisibilityAdmins    = "admins"
)

// ValidVisibility reports whether v is a recognized visibility value.
func ValidVisibility(v string) bool {
	return v == VisibilityHousehold || v == VisibilityAdmins
}

// ValidMetricType reports whether t is a recognized metric type.
func ValidMetricType(t string) bool {
	switch t {
	case MetricNone, MetricAmount, MetricRating, MetricDuration:
		return true
	}
	return false
}

// NormalizeVisibility fills in the default visibility when unset and validates it.
func (c *Chore) NormalizeVisibility() {
	if c.Visibility == "" {
		c.Visibility = VisibilityHousehold
	}
	if !ValidVisibility(c.Visibility) {
		c.Visibility = VisibilityHousehold
	}
}

// NormalizeMetric fills in a default metric configuration, deriving it from the
// legacy HasVolumeML/HasRating flags when MetricType is unset, then re-syncs the
// legacy flags so both representations agree. Call before persisting a chore.
func (c *Chore) NormalizeMetric() {
	if c.MetricType == "" || !ValidMetricType(c.MetricType) {
		switch {
		case c.HasVolumeML:
			c.MetricType = MetricAmount
			if c.MetricUnit == "" {
				c.MetricUnit = "mL"
			}
		case c.HasRating:
			c.MetricType = MetricRating
		default:
			c.MetricType = MetricNone
		}
	}
	switch c.MetricType {
	case MetricAmount:
		if c.MetricUnit == "" {
			c.MetricUnit = "mL"
		}
	default:
		c.MetricUnit = ""
	}
	// Keep the legacy flags in lockstep so downstream readers stay correct.
	c.HasVolumeML = c.MetricType == MetricAmount
	c.HasRating = c.MetricType == MetricRating
}

type Store interface {
	CreateChore(ctx context.Context, chore Chore) (Chore, error)
	GetChore(ctx context.Context, id int64) (Chore, error)
	ListChores(ctx context.Context, householdID int64) ([]Chore, error)
	UpdateChore(ctx context.Context, chore Chore) error
	DeleteChore(ctx context.Context, id int64) error
	ReorderChores(ctx context.Context, householdID int64, choreIDs []int64) error
	SeedPredefinedChores(ctx context.Context, householdID int64) error
}
