package log

import (
	"context"
	"fmt"
	"sort"
)

type readAccessKey struct{}

type readAccess struct {
	householdID int64
	viewerID    int64
	choreIDs    []int64
	visible     map[int64]struct{}
}

// WithReadAccess binds an immutable, previously resolved permission set to a
// collection read. A nil or empty set permits nothing. Absence of this context
// value is reserved for explicit internal operations without an HTTP viewer.
// PostgreSQL also rechecks membership and visibility in the data query itself.
func WithReadAccess(ctx context.Context, householdID, viewerID int64, visible map[int64]struct{}) context.Context {
	a := readAccess{householdID: householdID, viewerID: viewerID, visible: make(map[int64]struct{}, len(visible)), choreIDs: []int64{}}
	for id := range visible {
		a.visible[id] = struct{}{}
		a.choreIDs = append(a.choreIDs, id)
	}
	sort.Slice(a.choreIDs, func(i, j int) bool { return a.choreIDs[i] < a.choreIDs[j] })
	return context.WithValue(ctx, readAccessKey{}, a)
}

func readAllowed(ctx context.Context, householdID, choreID int64) bool {
	a, scoped := ctx.Value(readAccessKey{}).(readAccess)
	if !scoped {
		return true
	}
	_, visible := a.visible[choreID]
	return a.householdID == householdID && a.viewerID > 0 && visible
}

// readAccessSQL accepts only column expressions written by this package, never
// request data. start is the first unused positional argument number.
func readAccessSQL(ctx context.Context, householdID int64, choreColumn, householdColumn string, start int) (string, []any) {
	a, scoped := ctx.Value(readAccessKey{}).(readAccess)
	if !scoped {
		return "", nil
	}
	if a.householdID != householdID || a.viewerID <= 0 || len(a.choreIDs) == 0 {
		return " AND FALSE", nil
	}
	return fmt.Sprintf(` AND %s = ANY($%d::bigint[]) AND EXISTS (
		SELECT 1 FROM chores access_chore JOIN user_households access_member
		ON access_member.household_id = access_chore.household_id
		WHERE access_chore.id = %s AND access_chore.household_id = %s
		AND access_member.user_id = $%d
		AND (access_chore.visibility = 'household' OR access_member.role IN ('owner','admin'))
	)`, choreColumn, start, choreColumn, householdColumn, start+1), []any{a.choreIDs, a.viewerID}
}
