package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	chorelog "github.com/HammerMeetNail/nabu/internal/log"
)

func TestLogPatchPreservesOmittedMetrics(t *testing.T) {
	h, session, authService := setupLogTest(t)
	create := `{"choreId":1,"note":"original","title":"kept title","indicators":["A"],"indicatorVolumes":{"A":120},"volumeML":120,"rating":35,"durationSeconds":90,"subject":"Twin","hour":9,"date":"2026-09-01","completedAt":"2026-09-01T13:15:00Z"}`
	w := httptest.NewRecorder()
	h.Create(w, withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(create)), authService, session))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Log chorelog.ChoreLog `json:"log"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	patch := func(body string, status int) {
		t.Helper()
		r := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/1", strings.NewReader(body)), authService, session)
		r.SetPathValue("id", strconv.FormatInt(result.Log.ID, 10))
		out := httptest.NewRecorder()
		h.Update(out, r)
		if out.Code != status {
			t.Fatalf("patch %s: %d %s", body, out.Code, out.Body.String())
		}
	}
	for _, body := range []string{`{"note":"changed"}`, `{"completedAt":"2026-09-01T14:16:00Z","hour":10}`} {
		patch(body, http.StatusOK)
		got, err := h.service.GetLog(context.Background(), result.Log.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.DurationSeconds == nil || *got.DurationSeconds != 90 || got.VolumeML == nil || *got.VolumeML != 120 || got.Rating == nil || *got.Rating != 35 || got.Title == nil || *got.Title != "kept title" || got.Subject == nil || *got.Subject != "Twin" || len(got.Indicators) != 1 || got.IndicatorVolumes["A"] != 120 {
			t.Fatalf("omitted metrics lost after %s: %+v", body, got)
		}
	}
	patch(`{"durationSeconds":135}`, http.StatusOK)
	got, _ := h.service.GetLog(context.Background(), result.Log.ID)
	if got.DurationSeconds == nil || *got.DurationSeconds != 135 {
		t.Fatal("explicit duration change lost")
	}
	patch(`{"durationSeconds":null,"volumeML":null,"rating":null,"title":null,"subject":null,"indicators":[],"indicatorVolumes":{},"hour":null,"date":null}`, http.StatusOK)
	got, _ = h.service.GetLog(context.Background(), result.Log.ID)
	if got.DurationSeconds != nil || got.VolumeML != nil || got.Rating != nil || got.Title != nil || got.Subject != nil || len(got.Indicators) != 0 || len(got.IndicatorVolumes) != 0 || got.SlotHour != nil || got.LogDate != nil {
		t.Fatalf("explicit clear lost: %+v", got)
	}
	for _, body := range []string{`null`, `{"userId":null}`, `{"completedAt":null}`, `{"completedAt":""}`, `{"durationSeconds":-1}`} {
		patch(body, http.StatusBadRequest)
	}
}
