package auth

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

// GetUsersByIDs returns the users that still exist, as GetUserByID would read
// each one, and leaves out IDs it can't find.
func TestGetUsersByIDs(t *testing.T) {
	for name, open := range map[string]func(*testing.T) Store{
		"memory":   func(*testing.T) Store { return NewMemoryStore() },
		"postgres": postgresClaimStore,
	} {
		t.Run(name, func(t *testing.T) {
			store := open(t)
			ctx := context.Background()
			ada, err := store.CreateUser(ctx, "ada@example.invalid", "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			sam, err := store.CreateUser(ctx, "sam@example.invalid", "")
			if err != nil {
				t.Fatal(err)
			}
			none, err := store.GetUsersByIDs(ctx, nil)
			if err != nil || none == nil || len(none) != 0 {
				t.Fatalf("no IDs: %v, %v; want an empty list", none, err)
			}
			got, err := store.GetUsersByIDs(ctx, []int64{sam.ID, 999999, ada.ID})
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
			var want []User
			for _, id := range []int64{ada.ID, sam.ID} {
				user, err := store.GetUserByID(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, user)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("GetUsersByIDs = %+v\nwant %+v", got, want)
			}
		})
	}
}
