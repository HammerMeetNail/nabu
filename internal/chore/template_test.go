package chore_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/migrations"
)

// An activity's templateKey is its own template or its predefined key, so
// clients find a baby's feeds by one field.
func TestChoreJSONNamesItsTemplate(t *testing.T) {
	for _, tc := range []struct {
		chore              chore.Chore
		template, namedFor string
	}{
		{chore.Chore{Name: "Feed Baby", IsPredefined: true, PredefinedKey: "Feed Baby"}, "Feed Baby", ""},
		{chore.Chore{Name: "Bottles", TemplateKey: "Feed Baby", NamedFor: "Ada"}, "Feed Baby", "Ada"},
		{chore.Chore{Name: "Water Fern"}, "", ""},
	} {
		data, err := json.Marshal(tc.chore)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got["templateKey"] != tc.template || got["namedFor"] != tc.namedFor || got["predefinedKey"] != tc.chore.PredefinedKey {
			t.Fatalf("%s = %s", tc.chore.Name, data)
		}
	}
}

// Packs made a baby's or pet's own activities before they recorded their
// template and whom they were for. The migration finds those by their pack's
// name format and their template's icon, as packs record them now, and leaves
// alone any it can't be sure of.
func TestTemplateMigrationFindsWhatPacksMadeBefore(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO households(id,name,invite_code) VALUES(1,'One','TMPL1'),(2,'Two','TMPL2'),(3,'Three','TMPL3'); INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'one@example.invalid','','Synthetic')`); err != nil {
		t.Fatal(err)
	}
	store := chore.NewPostgresStore(db)
	svc := chore.NewService(store)

	// What packs record now. Household 2's Ada is a cat, so its Ada's Meds
	// are Cat Meds.
	want := map[int64]map[string][2]string{}
	for household, picks := range map[int64][]chore.PackPick{
		1: {{Key: "baby", Names: []string{"Ada"}}, {Key: "cat", Names: []string{"Mongo", "Roger"}}, {Key: "dog", Names: []string{"Rex"}}, {Key: "meds"}, {Key: "home"}},
		2: {{Key: "baby", Names: []string{"Ben", "Cy"}}, {Key: "cat", Names: []string{"Ada"}}, {Key: "meds"}},
	} {
		planned, err := chore.PlanPacks(picks)
		if err != nil {
			t.Fatal(err)
		}
		want[household] = map[string][2]string{}
		for i, c := range planned {
			want[household][c.Name] = [2]string{c.TemplateKey, c.NamedFor}
			planned[i].TemplateKey, planned[i].NamedFor = "", ""
		}
		if _, err := store.AddPackChores(ctx, household, planned); err != nil {
			t.Fatal(err)
		}
	}
	if want[1]["Ada's Meds"] != [2]string{"Baby Meds", "Ada"} || want[2]["Ada's Meds"] != [2]string{"Cat Meds", "Ada"} || want[1]["Walk Rex"] != [2]string{"Walk Dog", "Rex"} {
		t.Fatalf("planned %v", want)
	}

	// What it can't be sure of: an activity a member made, one renamed and
	// one given another icon. Seeded activities have predefined keys.
	if _, err := svc.CreateChore(ctx, 1, 1, "Feed Kid", "🍼", "#123456", "", nil, nil, nil, "", "", nil); err != nil {
		t.Fatal(err)
	}
	want[1]["Feed Kid"] = [2]string{}
	list, _ := svc.ListChores(ctx, 2)
	for _, c := range list {
		switch c.Name {
		case "Change Cy":
			if err := svc.UpdateChore(ctx, c.ID, 2, "Cy's diapers", c.Icon, c.Color, c.Category, nil, nil, nil, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			want[2]["Cy's diapers"] = [2]string{}
		case "Feed Ben":
			if err := svc.UpdateChore(ctx, c.ID, 2, c.Name, "🥛", c.Color, c.Category, nil, nil, nil, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			want[2]["Feed Ben"] = [2]string{}
		}
	}
	if err := svc.SeedDefaultChores(ctx, 3); err != nil {
		t.Fatal(err)
	}
	want[3] = map[string][2]string{}

	migration, err := migrations.Assets.ReadFile("062_chore_template.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := db.ExecContext(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	for household, byName := range want {
		list, err := svc.ListChores(ctx, household)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if got := [2]string{c.TemplateKey, c.NamedFor}; got != byName[c.Name] {
				t.Errorf("household %d %s: template %q for %q, want %q", household, c.Name, got[0], got[1], byName[c.Name])
			}
			if household == 3 && (c.Template() != c.PredefinedKey || c.Template() == "") {
				t.Errorf("seeded %s: template %q", c.Name, c.Template())
			}
		}
	}
}
