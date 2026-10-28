package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

func packRequest(t *testing.T, h *ChoreHandler, user auth.User, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/chores/packs", strings.NewReader(body))
	r = r.WithContext(middleware.WithUser(r.Context(), user))
	w := httptest.NewRecorder()
	h.AddPacks(w, r)
	return w
}

func TestPacksListsTheCatalog(t *testing.T) {
	h := NewChoreHandler(chore.NewService(chore.NewMemoryStore()))
	w := httptest.NewRecorder()
	h.Packs(w, httptest.NewRequest(http.MethodGet, "/api/packs", nil))
	var got struct {
		Packs []map[string]any `json:"packs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
		t.Fatalf("status %d, %v: %s", w.Code, err, w.Body)
	}
	if len(got.Packs) != len(chore.Packs) || got.Packs[0]["key"] != "baby" || got.Packs[0]["nameLabel"] != "Baby's name" || got.Packs[0]["title"] != "Baby" {
		t.Fatalf("packs = %v", got.Packs)
	}
	if _, ok := got.Packs[0]["Activities"]; ok {
		t.Fatal("the catalog exposes pack internals")
	}
	if _, ok := got.Packs[3]["nameLabel"]; ok {
		t.Fatalf("plants takes names: %v", got.Packs[3])
	}
}

func TestAddPacksSeedsTheCurrentHousehold(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	h := NewChoreHandler(svc)
	hid := int64(7)
	user := auth.User{ID: 1, HouseholdID: &hid}

	w := packRequest(t, h, user, `{"packs":[{"key":"baby","names":["Ada"]},{"key":"plants"}]}`)
	var got struct {
		Added []chore.Chore `json:"added"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
		t.Fatalf("status %d, %v: %s", w.Code, err, w.Body)
	}
	// Ada's own activities are custom tasks; Water Plants keeps its template.
	if len(got.Added) != 4 || got.Added[0].Name != "Feed Ada" || got.Added[0].HouseholdID != hid || got.Added[0].IsPredefined || got.Added[0].PredefinedKey != "" ||
		got.Added[3].Name != "Water Plants" || !got.Added[3].IsPredefined || got.Added[3].PredefinedKey != "Water Plants" {
		t.Fatalf("added = %+v", got.Added)
	}
	if strings.Contains(w.Body.String(), "defaultName") || strings.Contains(w.Body.String(), "DefaultName") {
		t.Fatalf("answer exposes the default name: %s", w.Body)
	}
	list, _ := svc.ListChores(t.Context(), hid)
	if len(list) != 4 {
		t.Fatalf("household has %d activities", len(list))
	}

	// Picking the same packs again adds nothing.
	w = packRequest(t, h, user, `{"packs":[{"key":"plants"}]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"added":[]`) {
		t.Fatalf("again: %d %s", w.Code, w.Body)
	}
}

func TestAddPacksRefusals(t *testing.T) {
	h := NewChoreHandler(chore.NewService(chore.NewMemoryStore()))
	hid := int64(7)
	member := auth.User{ID: 1, HouseholdID: &hid}
	for _, tc := range []struct {
		user auth.User
		body string
		code int
		want string
	}{
		{auth.User{ID: 1}, `{"packs":[{"key":"plants"}]}`, http.StatusUnauthorized, "no household"},
		{member, `{"packs":`, http.StatusBadRequest, "invalid request body"},
		{member, `{"packs":[]}`, http.StatusBadRequest, "Choose at least one pack"},
		{member, `{"packs":[{"key":"cat","names":["Mongo","mongo"]}]}`, http.StatusBadRequest, `\"mongo\" is used twice`},
		{member, `{"packs":[{"key":"home","names":["Kitchen"]}]}`, http.StatusBadRequest, "Home chores doesn't take names"},
	} {
		w := packRequest(t, h, tc.user, tc.body)
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("%s: %d %s, want %d %q", tc.body, w.Code, w.Body, tc.code, tc.want)
		}
	}
}
