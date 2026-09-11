// Package lifecycle supplies cleanup metadata for the in-memory stores.
package lifecycle

import "errors"

var ErrDeleted = errors.New("resource no longer available")

// Deletion is assembled and applied synchronously at the account transaction's
// commit point. Domain cleanup methods add dependent IDs before the next store.
type Deletion struct {
	UserID                              int64
	Households, Chores, Schedules, Logs map[int64]bool
}

func NewDeletion(userID int64, households []int64) *Deletion {
	d := &Deletion{UserID: userID, Households: map[int64]bool{}, Chores: map[int64]bool{}, Schedules: map[int64]bool{}, Logs: map[int64]bool{}}
	for _, id := range households {
		d.Households[id] = true
	}
	return d
}

// Tombstones prevent an already-authenticated request from recreating deleted
// data. All access uses the containing store's mutex; this type takes no locks.
type Tombstones struct {
	users, households, chores, schedules, logs map[int64]bool
}

func (t *Tombstones) Mark(d *Deletion) {
	if t.users == nil {
		t.users = map[int64]bool{}
		t.households = map[int64]bool{}
		t.chores = map[int64]bool{}
		t.schedules = map[int64]bool{}
		t.logs = map[int64]bool{}
	}
	t.users[d.UserID] = true
	for id := range d.Households {
		t.households[id] = true
	}
	for id := range d.Chores {
		t.chores[id] = true
	}
	for id := range d.Schedules {
		t.schedules[id] = true
	}
	for id := range d.Logs {
		t.logs[id] = true
	}
}
func (t *Tombstones) Check(userID, householdID, choreID, scheduleID, logID int64) error {
	if (userID != 0 && t.users[userID]) || (householdID != 0 && t.households[householdID]) || (choreID != 0 && t.chores[choreID]) || (scheduleID != 0 && t.schedules[scheduleID]) || (logID != 0 && t.logs[logID]) {
		return ErrDeleted
	}
	return nil
}
func UserID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
