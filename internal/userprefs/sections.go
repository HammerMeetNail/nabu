package userprefs

// StatsSections lists every stats section in canonical default order.
// This is the single source of truth for section key names.
//
// When you add a new stats section, append it to the END of this list.
// Existing users will see the new section appear (visible by default)
// below their existing sections, per the layout-resolution algorithm.
var StatsSections = []string{
	"overview",
	"baby",
	"activity",
	"busy-hours",
	"leaderboard",
	"top-chores",
	"categories",
	"chores",
	"recap",
}

// IsKnownStatsSection reports whether key is a recognized stats section.
func IsKnownStatsSection(key string) bool {
	for _, s := range StatsSections {
		if s == key {
			return true
		}
	}
	return false
}

// DefaultStatsSectionOrder returns a copy of the canonical order.
func DefaultStatsSectionOrder() []string {
	out := make([]string, len(StatsSections))
	copy(out, StatsSections)
	return out
}
