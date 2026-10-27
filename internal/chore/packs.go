package chore

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Activity packs (improvement plan, Phase 2). A new household seeds only the
// packs it picks, named for the baby and pets it looks after ("Feed Mongo"),
// and any household can add packs later from Manage. PredefinedChores stays
// the full set that POST /api/chores/seed-defaults seeds, which is what
// households created before packs received.
//
// Every pack activity is a predefined template whose name is the activity's
// predefined_key, so a change to a template's fields still reaches existing
// households through an UPDATE migration keyed on predefined_key. A template
// added to a pack needs no migration: households get it with the pack.

// PackActivity is one activity a pack seeds.
type PackActivity struct {
	// Template is the predefined activity's key, which is also its name when
	// the pack is given no names.
	Template string
	// PerName, when set, makes one activity per name given, named with this
	// format ("Feed %s"). Otherwise the pack seeds one shared activity.
	PerName string
}

// Pack is a set of activities offered together during setup and in Manage.
type Pack struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	Icon    string `json:"icon"`
	Summary string `json:"summary"`
	// NameLabel is set for a pack that takes names (a baby, pets): the label
	// for one name field.
	NameLabel  string         `json:"nameLabel,omitempty"`
	Activities []PackActivity `json:"-"`
}

// PackPick is a pack chosen by the user, with the names it applies to.
type PackPick struct {
	Key   string   `json:"key"`
	Names []string `json:"names"`
}

// PackError is a pick the server refuses, with a message the user can act on.
type PackError struct{ Message string }

func (e *PackError) Error() string { return e.Message }

const (
	// MaxPackNames bounds the names given to one pack (twins, a few pets).
	MaxPackNames = 6
	// MaxPackNameRunes matches the limit on an activity's subject tags.
	MaxPackNameRunes = 30
)

// medsPack seeds a medication activity for each pack below that was picked,
// named for its baby or pet when it has exactly one, or "Meds" when none was.
const medsPack = "meds"

var medsFor = []struct{ pack, template string }{
	{"baby", "Baby Meds"},
	{"cat", "Cat Meds"},
	{"dog", "Dog Meds"},
}

// Packs lists the packs in the order they are offered and seeded.
var Packs = []Pack{
	{Key: "baby", Version: 1, Title: "Baby", Icon: "👶", Summary: "Feeds, changes and baths", NameLabel: "Baby's name",
		Activities: []PackActivity{{"Feed Baby", "Feed %s"}, {"Change Baby", "Change %s"}, {"Baby Bath", "%s's Bath"}}},
	{Key: "cat", Version: 1, Title: "Cats", Icon: "🐱", Summary: "Feeding and the litter box", NameLabel: "Cat's name",
		Activities: []PackActivity{{"Feed Cats", "Feed %s"}, {"Clean Litter Box", ""}}},
	{Key: "dog", Version: 1, Title: "Dogs", Icon: "🐶", Summary: "Walks and feeding", NameLabel: "Dog's name",
		Activities: []PackActivity{{"Walk Dog", "Walk %s"}, {"Feed Dog", "Feed %s"}}},
	{Key: "plants", Version: 1, Title: "Plants", Icon: "🌱", Summary: "Watering",
		Activities: []PackActivity{{"Water Plants", ""}}},
	{Key: "home", Version: 1, Title: "Home chores", Icon: "🏠", Summary: "Trash, dishes, vacuuming, laundry and beds",
		Activities: []PackActivity{{"Take Out Trash", ""}, {"Wash Dishes", ""}, {"Vacuum", ""}, {"Laundry", ""}, {"Make Bed", ""}}},
	{Key: medsPack, Version: 1, Title: "Medications", Icon: "💊", Summary: "Doses with amounts, for each baby and pet you add, or for you"},
	{Key: "personal", Version: 1, Title: "Personal", Icon: "📖", Summary: "Reading, movies and workouts",
		Activities: []PackActivity{{"Read Book", ""}, {"Watch Movie", ""}, {"Workout", ""}}},
}

// packOnlyChores are templates that only packs seed; seed-defaults does not.
var packOnlyChores = []Chore{
	{Name: "Feed Dog", Icon: "🐶", Color: "#D97706", Category: "feeding"},
	{Name: "Dog Meds", Icon: "💊", Color: "#A78BFA", Category: "care", HasVolumeML: true, MetricType: MetricAmount, MetricUnit: "mg"},
	{Name: "Meds", Icon: "💊", Color: "#A78BFA", Category: "care", HasVolumeML: true, MetricType: MetricAmount, MetricUnit: "mg"},
	{Name: "Workout", Icon: "🏋️", Color: "#22C55E", Category: "personal", MetricType: MetricDuration},
}

// templateFor returns the predefined activity with this key.
func templateFor(key string) (Chore, bool) {
	for _, list := range [][]Chore{PredefinedChores, packOnlyChores} {
		for _, c := range list {
			if c.Name == key {
				return c, true
			}
		}
	}
	return Chore{}, false
}

func packByKey(key string) (Pack, bool) {
	for _, p := range Packs {
		if p.Key == key {
			return p, true
		}
	}
	return Pack{}, false
}

// cleanPackNames trims each name and checks it, and that no name is used twice
// across the picks, since names become activity names.
func cleanPackNames(picks []PackPick) (map[string][]string, error) {
	if len(picks) == 0 {
		return nil, &PackError{"Choose at least one pack"}
	}
	names := map[string][]string{}
	used := map[string]bool{}
	for _, pick := range picks {
		pack, ok := packByKey(pick.Key)
		if !ok {
			return nil, &PackError{fmt.Sprintf("Unknown pack %q", pick.Key)}
		}
		if _, dup := names[pick.Key]; dup {
			return nil, &PackError{"Each pack can be chosen once"}
		}
		if len(pick.Names) > 0 && pack.NameLabel == "" {
			return nil, &PackError{pack.Title + " doesn't take names"}
		}
		if len(pick.Names) > MaxPackNames {
			return nil, &PackError{fmt.Sprintf("Add up to %d names for %s", MaxPackNames, pack.Title)}
		}
		cleaned := []string{}
		for _, raw := range pick.Names {
			name := strings.TrimSpace(raw)
			if name == "" || utf8.RuneCountInString(name) > MaxPackNameRunes || strings.IndexFunc(name, unicode.IsControl) >= 0 {
				return nil, &PackError{fmt.Sprintf("Names must be 1 to %d characters", MaxPackNameRunes)}
			}
			if used[strings.ToLower(name)] {
				return nil, &PackError{fmt.Sprintf("%q is used twice; give each one a different name", name)}
			}
			used[strings.ToLower(name)] = true
			cleaned = append(cleaned, name)
		}
		names[pick.Key] = cleaned
	}
	return names, nil
}

// PlanPacks returns the activities the picked packs seed, in pack order and
// named for the names given. Its sort orders start at 0. It returns a
// *PackError for a pick the user can correct.
func PlanPacks(picks []PackPick) ([]Chore, error) {
	names, err := cleanPackNames(picks)
	if err != nil {
		return nil, err
	}
	var planned []Chore
	add := func(template, name string) {
		c, ok := templateFor(template)
		if !ok {
			panic("chore: pack template not found: " + template)
		}
		c.Name = name
		c.IsPredefined = true
		c.PredefinedKey = template
		c.DefaultName = name
		c.SortOrder = len(planned)
		c.FollowUpEnabled = true
		c.Visibility = VisibilityHousehold
		c.IndicatorLabels = append([]string{}, c.IndicatorLabels...)
		c.IndicatorDefaults = append([]string{}, c.IndicatorDefaults...)
		c.NormalizeMetric()
		planned = append(planned, c)
	}
	for _, pack := range Packs {
		given, picked := names[pack.Key]
		if !picked {
			continue
		}
		if pack.Key == medsPack {
			anyFor := false
			for _, m := range medsFor {
				owners, ok := names[m.pack]
				if !ok {
					continue
				}
				anyFor = true
				if len(owners) == 1 {
					add(m.template, owners[0]+"'s Meds")
				} else {
					add(m.template, m.template)
				}
			}
			if !anyFor {
				add("Meds", "Meds")
			}
			continue
		}
		for _, a := range pack.Activities {
			if a.PerName == "" || len(given) == 0 {
				add(a.Template, a.Template)
				continue
			}
			for _, name := range given {
				add(a.Template, fmt.Sprintf(a.PerName, name))
			}
		}
	}
	return planned, nil
}
