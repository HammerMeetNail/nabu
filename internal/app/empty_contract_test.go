package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// nullableResponseKeys lists JSON object keys whose value is a single
// optional object or scalar (a Go pointer), where null is part of the
// contract. Arrays are never in this list: an API response must encode an
// empty list as [] so clients can decode it into a non-optional array (the
// iOS Stats tab failed on recap.byCategory: null for every new household).
var nullableResponseKeys = map[string]bool{
	"topPerformer": true,
	"avgVolume":    true,
	"createdBy":    true,
}

// TestEmptyHouseholdResponsesHaveNoNullArrays fetches every read endpoint the
// clients call for a brand-new household (with and without seeded chores and
// before any log exists) and fails on any null that is not an allowlisted
// optional object.
func TestEmptyHouseholdResponsesHaveNoNullArrays(t *testing.T) {
	srv := newTestSrv(t)
	if closer, ok := srv.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	session := srvRegister(t, srv)
	if rec := srvRequest(srv, http.MethodPost, "/api/household", `{"name":"Empty"}`, "tok", session); rec.Code >= 300 {
		t.Fatalf("create household: %d %s", rec.Code, rec.Body.String())
	}

	endpoints := []string{
		"/api/me",
		"/api/household",
		"/api/households",
		"/api/chores",
		"/api/preferences",
		"/api/notifications",
		"/api/logs/today",
		"/api/logs/week",
		"/api/logs/month",
		"/api/logs/history",
		"/api/logs/latest-per-chore",
		"/api/chore-reminder-prefs",
		"/api/schedules",
		"/api/schedules/for-date?date=2026-10-13",
		"/api/day-notes?start=2026-10-01&end=2026-10-31",
		"/api/stats/leaderboard",
		"/api/stats/streaks",
		"/api/stats/heatmap",
		"/api/stats/breakdown",
		"/api/stats/recap",
		"/api/stats/overview",
		"/api/stats/busy-hours",
		"/api/stats/top-chores",
		"/api/stats/chores",
		"/api/stats/feeding-gaps",
	}

	check := func(phase string, paths []string) {
		for _, path := range paths {
			rec := srvRequest(srv, http.MethodGet, path, "", "", session)
			if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("%s %s: route missing (%d)", phase, path, rec.Code)
				continue
			}
			if rec.Code != http.StatusOK {
				continue
			}
			var body any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Errorf("%s %s: invalid JSON: %v", phase, path, err)
				continue
			}
			if nulls := findNulls(body, "$"); len(nulls) > 0 {
				t.Errorf("%s %s: null values at %s", phase, path, strings.Join(nulls, ", "))
			}
		}
	}
	check("unseeded", endpoints)

	if rec := srvRequest(srv, http.MethodPost, "/api/chores/seed-defaults", "", "tok", session); rec.Code >= 300 {
		t.Fatalf("seed defaults: %d %s", rec.Code, rec.Body.String())
	}
	var chores struct {
		Chores []struct {
			ID int64 `json:"id"`
		} `json:"chores"`
	}
	rec := srvRequest(srv, http.MethodGet, "/api/chores", "", "", session)
	if err := json.Unmarshal(rec.Body.Bytes(), &chores); err != nil || len(chores.Chores) == 0 {
		t.Fatalf("list seeded chores: %v %s", err, rec.Body.String())
	}
	id := chores.Chores[0].ID
	seeded := append([]string{}, endpoints...)
	seeded = append(seeded, fmt.Sprintf("/api/logs/recent-amounts?choreId=%d", id))
	for _, suffix := range []string{"", "/time-series", "/summary"} {
		seeded = append(seeded, fmt.Sprintf("/api/stats/chores/%d%s", id, suffix))
	}
	check("seeded", seeded)
}

func findNulls(v any, path string) []string {
	var out []string
	switch val := v.(type) {
	case nil:
		out = append(out, path)
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if val[k] == nil && nullableResponseKeys[k] {
				continue
			}
			out = append(out, findNulls(val[k], path+"."+k)...)
		}
	case []any:
		for i, item := range val {
			out = append(out, findNulls(item, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}
