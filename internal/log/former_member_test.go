package log_test

import (
	"context"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
)

func TestMemoryCleanupAccountTombstonesSharedLogs(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name               string
		deleteLogs, notes  bool
		wantKept, wantNote bool
	}{
		{name: "default keeps logs and notes", wantKept: true, wantNote: true},
		{name: "delete notes", notes: true, wantKept: true},
		{name: "delete logs", deleteLogs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := chorelog.NewMemoryStore()
			const ann, ben = 1, 2
			mine, _ := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 10, ChoreID: 1, UserID: ben, Note: "night feed", IdempotencyActorID: ben})
			mine2, _ := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 10, ChoreID: 1, UserID: ben})
			elsewhere, _ := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 20, ChoreID: 2, UserID: ben})
			forAnn, _ := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 10, ChoreID: 1, UserID: ann, IdempotencyActorID: ben})

			d := lifecycle.NewDeletion(ben, nil)
			d.DeleteLogs, d.DeleteNotes = tc.deleteLogs, tc.notes
			store.CleanupAccount(d)

			got, err := store.GetLog(ctx, mine.ID)
			if (err == nil) != tc.wantKept {
				t.Fatalf("log kept = %v, want %v", err == nil, tc.wantKept)
			}
			if a, _ := store.GetLog(ctx, forAnn.ID); a.UserID != ann || a.IdempotencyActorID != 0 {
				t.Fatalf("Ann's log = user %d actor %d", a.UserID, a.IdempotencyActorID)
			}
			if !tc.wantKept {
				return
			}
			second, _ := store.GetLog(ctx, mine2.ID)
			other, _ := store.GetLog(ctx, elsewhere.ID)
			if got.UserID >= 0 || second.UserID != got.UserID || other.UserID >= 0 || other.UserID == got.UserID {
				t.Fatalf("tombstones = %d, %d, %d; want one negative ID per household", got.UserID, second.UserID, other.UserID)
			}
			if (got.Note != "") != tc.wantNote {
				t.Fatalf("note = %q", got.Note)
			}
		})
	}
}
