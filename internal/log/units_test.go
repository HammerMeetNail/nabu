package log_test

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"testing"
)

func TestEntryUnitSnapshotAndLegacyReplay(t *testing.T) {
	ctx := context.Background()
	hs := household.NewMemoryStore()
	hh, err := hs.CreateHousehold(ctx, "Home", "H", 1)
	if err != nil {
		t.Fatal(err)
	}
	cs := chore.NewMemoryStore()
	c, err := cs.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Meds", HasVolumeML: true, MetricType: chore.MetricAmount, MetricUnit: "mg"})
	if err != nil {
		t.Fatal(err)
	}
	store := chorelog.NewMemoryStore()
	svc := chorelog.NewService(store).WithAccess(cs, hs)
	amount := 5
	in := chorelog.CreateInput{ActorID: 1, UserID: 1, HouseholdID: hh.ID, ChoreID: c.ID, VolumeML: &amount, IdempotencyKey: "legacy-unit"}
	saved, _, err := svc.LogChoreIdempotent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if saved.MetricUnit != "mg" {
		t.Fatalf("snapshot=%q", saved.MetricUnit)
	}
	unit := "g"
	err = svc.UpdateLog(ctx, saved.ID, hh.ID, nil, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, chorelog.Patch{ActorID: 1, Fields: chorelog.LogFields{"metricUnit": true}, MetricUnit: &unit})
	if err != nil {
		t.Fatal(err)
	}
	replay, created, err := svc.LogChoreIdempotent(ctx, in)
	if err != nil || created || replay.MetricUnit != "g" {
		t.Fatalf("replay=%+v created=%v err=%v", replay, created, err)
	}
	err = svc.UpdateLog(ctx, saved.ID, hh.ID, nil, "note", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, chorelog.Patch{ActorID: 1, Fields: chorelog.LogFields{"note": true}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetLog(ctx, saved.ID)
	if err != nil || updated.MetricUnit != "g" || updated.VolumeML == nil || *updated.VolumeML != 5 {
		t.Fatalf("note patch=%+v err=%v", updated, err)
	}
	legacy := chorelog.ChoreLog{HouseholdID: hh.ID, ChoreID: c.ID, UserID: 1}
	legacy, err = store.CreateLog(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.UpdateLog(ctx, legacy.ID, hh.ID, nil, "", nil, nil, &amount, nil, nil, nil, nil, nil, nil, nil, chorelog.Patch{ActorID: 1, Fields: chorelog.LogFields{"volumeML": true}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err = store.GetLog(ctx, legacy.ID)
	if err != nil || updated.MetricUnit != "mg" {
		t.Fatalf("legacy patch=%+v err=%v", updated, err)
	}
}

func TestImplicitSnapshotDoesNotOverwriteExplicitUnitEdit(t *testing.T) {
	ctx := context.Background()
	store := chorelog.NewMemoryStore()
	original, err := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, ChoreID: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Both edits read the blank snapshot. The explicit unit edit commits first.
	explicit, implicit := original, original
	explicit.MetricUnit = "g"
	implicit.MetricUnit = "mg"
	amount := 5
	implicit.VolumeML = &amount
	if err = store.UpdateLog(ctx, explicit, chorelog.LogFields{"metricUnit": true}); err != nil {
		t.Fatal(err)
	}
	if err = store.UpdateLog(ctx, implicit, chorelog.LogFields{"volumeML": true, "snapshotMetricUnit": true}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetLog(ctx, original.ID)
	if err != nil || got.MetricUnit != "g" || got.VolumeML == nil || *got.VolumeML != 5 {
		t.Fatalf("lost concurrent edit: %+v %v", got, err)
	}
}
