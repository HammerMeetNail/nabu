package log_test

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/migrations"
	"testing"
)

func TestMedicationMigrationAndLegacyUnitSnapshots(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES(1,'Home','UNITS')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'units@example.invalid','','Test')`,
		`INSERT INTO chores(id,household_id,name,predefined_key,is_predefined) VALUES(1,1,'Cat Meds','Cat Meds',true),(2,1,'Baby Meds',NULL,false)`,
		`INSERT INTO chore_logs(id,household_id,user_id,chore_id,completed_at) VALUES(1,1,1,1,now())`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	migration, err := migrations.Assets.ReadFile("051_medication_amounts.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = db.ExecContext(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM chores WHERE household_id=1 AND predefined_key IN ('Cat Meds','Baby Meds') AND metric_unit='mg' AND has_volume_ml`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("meds count=%d err=%v", count, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE chore_logs SET volume_ml=5 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE chores SET metric_unit='g' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var unit string
	if err = db.QueryRowContext(ctx, `SELECT metric_unit FROM chore_logs WHERE id=1`).Scan(&unit); err != nil || unit != "mg" {
		t.Fatalf("legacy update unit=%q err=%v", unit, err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO chore_logs(id,household_id,user_id,chore_id,completed_at,indicator_volumes) VALUES(2,1,1,1,now(),'{"dose":12}')`); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT metric_unit FROM chore_logs WHERE id=2`).Scan(&unit); err != nil || unit != "g" {
		t.Fatalf("legacy insert unit=%q err=%v", unit, err)
	}
	// Replay the additive migration against an indicator-only legacy row.
	if _, err = db.ExecContext(ctx, `ALTER TABLE chore_logs DISABLE TRIGGER chore_logs_snapshot_unit; UPDATE chore_logs SET metric_unit='' WHERE id=2; ALTER TABLE chore_logs ENABLE TRIGGER chore_logs_snapshot_unit`); err != nil {
		t.Fatal(err)
	}
	backfill, err := migrations.Assets.ReadFile("050_log_metric_unit.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, string(backfill)); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT metric_unit FROM chore_logs WHERE id=2`).Scan(&unit); err != nil || unit != "g" {
		t.Fatalf("indicator backfill unit=%q err=%v", unit, err)
	}

}
