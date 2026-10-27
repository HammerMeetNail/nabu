package chore_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func names(chores []chore.Chore) []string {
	out := []string{}
	for _, c := range chores {
		out = append(out, c.Name)
	}
	return out
}

func TestPlanPacksNamesActivitiesForEachBabyAndPet(t *testing.T) {
	// Picked out of order: activities still follow the packs' own order.
	planned, err := chore.PlanPacks([]chore.PackPick{
		{Key: "cat", Names: []string{" Mongo ", "Roger"}},
		{Key: "plants"},
		{Key: "baby", Names: []string{"Ada"}},
		{Key: "dog"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Feed Ada", "Change Ada", "Ada's Bath", "Feed Mongo", "Feed Roger", "Clean Litter Box", "Walk Dog", "Feed Dog", "Water Plants"}
	if got := names(planned); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
	byName := map[string]chore.Chore{}
	for i, c := range planned {
		byName[c.Name] = c
		if !c.IsPredefined || c.SortOrder != i || c.DefaultName != c.Name || c.Visibility != chore.VisibilityHousehold || !c.FollowUpEnabled {
			t.Fatalf("%s = %+v", c.Name, c)
		}
	}
	feed := byName["Feed Ada"]
	if feed.PredefinedKey != "Feed Baby" || feed.MetricType != chore.MetricAmount || feed.MetricUnit != "mL" || !reflect.DeepEqual(feed.IndicatorLabels, []string{"🍼 formula", "🤱 breast"}) {
		t.Fatalf("Feed Ada = %+v", feed)
	}
	if byName["Feed Roger"].PredefinedKey != "Feed Cats" || byName["Ada's Bath"].PredefinedKey != "Baby Bath" || byName["Feed Dog"].Icon != "🐶" {
		t.Fatalf("templates = %+v", byName)
	}
}

func TestPlanPacksMedsFollowTheOtherPacks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		picks []chore.PackPick
		meds  []string
	}{
		{"for you when no baby or pet", []chore.PackPick{{Key: "meds"}, {Key: "home"}}, []string{"Meds"}},
		{"named for a single baby or pet", []chore.PackPick{{Key: "meds"}, {Key: "baby", Names: []string{"Ada"}}, {Key: "cat", Names: []string{"Mongo", "Roger"}}, {Key: "dog"}},
			[]string{"Ada's Meds", "Cat Meds", "Dog Meds"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planned, err := chore.PlanPacks(tc.picks)
			if err != nil {
				t.Fatal(err)
			}
			var meds []string
			for _, c := range planned {
				if strings.HasSuffix(c.PredefinedKey, "Meds") {
					meds = append(meds, c.Name)
					if c.MetricType != chore.MetricAmount || c.MetricUnit != "mg" {
						t.Fatalf("%s = %+v", c.Name, c)
					}
				}
			}
			if !reflect.DeepEqual(meds, tc.meds) {
				t.Fatalf("meds = %q, want %q", meds, tc.meds)
			}
		})
	}
}

func TestPlanPacksCoversEveryPackWithoutSharingTemplates(t *testing.T) {
	var picks []chore.PackPick
	for _, p := range chore.Packs {
		pick := chore.PackPick{Key: p.Key}
		if p.NameLabel != "" {
			pick.Names = []string{"Synthetic " + p.Key}
		}
		picks = append(picks, pick)
	}
	planned, err := chore.PlanPacks(picks)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) < len(chore.Packs) {
		t.Fatalf("planned %d activities for %d packs", len(planned), len(chore.Packs))
	}
	for _, c := range planned {
		if c.Name == "" || c.Icon == "" || c.PredefinedKey == "" || len([]rune(c.Name)) > 60 {
			t.Fatalf("activity %+v", c)
		}
	}
	// Changing a planned activity must not change the templates.
	planned[0].IndicatorLabels[0] = "changed"
	again, _ := chore.PlanPacks(picks)
	if again[0].IndicatorLabels[0] == "changed" {
		t.Fatal("planned activities share the template's labels")
	}
}

func TestPlanPacksRefusesWhatTheUserMustFix(t *testing.T) {
	long := strings.Repeat("n", chore.MaxPackNameRunes+1)
	for _, tc := range []struct {
		picks []chore.PackPick
		want  string
	}{
		{nil, "Choose at least one pack"},
		{[]chore.PackPick{{Key: "goldfish"}}, `Unknown pack "goldfish"`},
		{[]chore.PackPick{{Key: "home"}, {Key: "home"}}, "Each pack can be chosen once"},
		{[]chore.PackPick{{Key: "plants", Names: []string{"Fern"}}}, "Plants doesn't take names"},
		{[]chore.PackPick{{Key: "cat", Names: []string{"a", "b", "c", "d", "e", "f", "g"}}}, "Add up to 6 names for Cats"},
		{[]chore.PackPick{{Key: "dog", Names: []string{"  "}}}, "Names must be 1 to 30 characters"},
		{[]chore.PackPick{{Key: "dog", Names: []string{long}}}, "Names must be 1 to 30 characters"},
		{[]chore.PackPick{{Key: "dog", Names: []string{"Rex\u0007"}}}, "Names must be 1 to 30 characters"},
		{[]chore.PackPick{{Key: "baby", Names: []string{"Max"}}, {Key: "dog", Names: []string{"max"}}}, `"max" is used twice; give each one a different name`},
	} {
		_, err := chore.PlanPacks(tc.picks)
		var packErr *chore.PackError
		if !errors.As(err, &packErr) || packErr.Message != tc.want {
			t.Errorf("PlanPacks(%+v) = %v, want %q", tc.picks, err, tc.want)
		}
	}
}

// packStores runs a test on the memory store and, with TEST_DATABASE_URL, on
// PostgreSQL. Both have household 1 and user 1.
func packStores(t *testing.T, test func(t *testing.T, svc *chore.Service)) {
	t.Run("memory", func(t *testing.T) { test(t, chore.NewService(chore.NewMemoryStore())) })
	t.Run("postgres", func(t *testing.T) {
		db := testdb.New(t)
		if err := database.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO households(id,name,invite_code) VALUES(1,'Synthetic home','SYNTH1'); INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'one@example.invalid','','Synthetic')`); err != nil {
			t.Fatal(err)
		}
		test(t, chore.NewService(chore.NewPostgresStore(db)))
	})
}

func TestAddPacksKeepsWhatTheHouseholdHas(t *testing.T) {
	packStores(t, func(t *testing.T, svc *chore.Service) {
		ctx := context.Background()
		picks := []chore.PackPick{{Key: "cat", Names: []string{"Mongo"}}, {Key: "meds"}}
		added, err := svc.AddPacks(ctx, 1, picks)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(added); !reflect.DeepEqual(got, []string{"Feed Mongo", "Clean Litter Box", "Mongo's Meds"}) {
			t.Fatalf("added %q", got)
		}
		// The same picks again add nothing, and seed-defaults leaves a pack
		// household alone.
		if again, err := svc.AddPacks(ctx, 1, picks); err != nil || len(again) != 0 {
			t.Fatalf("again = %q, %v", names(again), err)
		}
		if err := svc.SeedDefaultChores(ctx, 1); err != nil {
			t.Fatal(err)
		}
		list, _ := svc.ListChores(ctx, 1)
		if len(list) != 3 {
			t.Fatalf("after seed-defaults: %q", names(list))
		}

		// A renamed shared activity still counts, and a taken name is skipped
		// whatever its case. A new cat is added after the existing activities.
		litter := list[1]
		if err := svc.UpdateChore(ctx, litter.ID, 1, "Scoop litter", litter.Icon, litter.Color, litter.Category, nil, nil, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateChore(ctx, 1, 1, "feed luna", "🐈", "#123456", "", nil, nil, nil, "", "", nil); err != nil {
			t.Fatal(err)
		}
		before, _ := svc.ListChores(ctx, 1)
		next := 0
		for _, c := range before {
			next = max(next, c.SortOrder+1)
		}
		added, err = svc.AddPacks(ctx, 1, []chore.PackPick{{Key: "cat", Names: []string{"Luna", "Nova"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(added) != 1 || added[0].Name != "Feed Nova" || added[0].SortOrder != next || added[0].ID == 0 {
			t.Fatalf("added %+v", added)
		}

		// Restore default returns a named activity to its pack name.
		mongo := list[0]
		if err := svc.UpdateChore(ctx, mongo.ID, 1, "Dinner", "🍣", mongo.Color, mongo.Category, nil, nil, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := svc.RestoreDefaultChore(ctx, mongo.ID, 1); err != nil {
			t.Fatal(err)
		}
		restored, err := svc.GetChore(ctx, mongo.ID)
		if err != nil || restored.Name != "Feed Mongo" || restored.Icon != "🐱" || restored.PredefinedKey != "Feed Cats" {
			t.Fatalf("restored = %+v, %v", restored, err)
		}
	})
}

func TestSeedDefaultsFillsOnlyAHouseholdWithoutDefaults(t *testing.T) {
	packStores(t, func(t *testing.T, svc *chore.Service) {
		ctx := context.Background()
		if _, err := svc.CreateChore(ctx, 1, 1, "Synthetic custom", "📝", "#123456", "", nil, nil, nil, "", "", nil); err != nil {
			t.Fatal(err)
		}
		if err := svc.SeedDefaultChores(ctx, 1); err != nil {
			t.Fatal(err)
		}
		list, _ := svc.ListChores(ctx, 1)
		if len(list) != len(chore.PredefinedChores)+1 {
			t.Fatalf("seeded %d activities", len(list))
		}
		// A pack added to a household from before packs brings only what it lacks.
		added, err := svc.AddPacks(ctx, 1, []chore.PackPick{{Key: "dog", Names: []string{"Rex"}}, {Key: "plants"}})
		if err != nil || !reflect.DeepEqual(names(added), []string{"Walk Rex", "Feed Rex"}) {
			t.Fatalf("added %q, %v", names(added), err)
		}
	})
}
