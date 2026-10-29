package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/apikey"
	"github.com/HammerMeetNail/nabu/internal/apns"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/daynote"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

// noMyData is every personal data source but logs, holding nothing.
type noMyData struct{}

func (noMyData) ListUserSessions(context.Context, int64, string) ([]auth.SessionInfo, error) {
	return nil, nil
}
func (noMyData) GetPreferences(context.Context, int64) (userprefs.Preferences, error) {
	return userprefs.Preferences{}, nil
}
func (noMyData) GetNotificationPreferences(context.Context, int64) (notification.ReminderPreference, error) {
	return notification.ReminderPreference{}, nil
}
func (noMyData) ListPage(context.Context, int64, string) (notification.Page, error) {
	return notification.Page{}, nil
}
func (noMyData) GetChoreReminderPrefs(context.Context, int64) ([]reminder.ChoreReminderPref, error) {
	return nil, nil
}
func (noMyData) ListRange(context.Context, int64, time.Time, time.Time) ([]daynote.DayNote, error) {
	return nil, nil
}
func (noMyData) GetSubscriptions(context.Context, int64) ([]push.Subscription, error) {
	return nil, nil
}
func (noMyData) DevicesForUser(context.Context, int64) ([]apns.Device, error) { return nil, nil }
func (noMyData) List(context.Context, auth.User) ([]apikey.Key, error)        { return nil, nil }

// A personal export has no date range to narrow, so it holds more than a
// household export's 10,000 rows, and data past its own limit is sent to
// support rather than told to choose dates.
func TestPersonalExportsHoldYearsOfLogs(t *testing.T) {
	logs := logsvc.NewMemoryStore()
	add := func(n int) {
		for range n {
			if _, err := logs.CreateLog(context.Background(), logsvc.ChoreLog{HouseholdID: 1, UserID: 1, ChoreID: 1, CompletedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
	}
	none := noMyData{}
	myData := NewMyDataHandler(MyDataSources{Sessions: none, Households: household.NewMemoryStore(), Chores: chore.NewMemoryStore(),
		Preferences: none, Notifications: none, Reminders: none, Logs: logsvc.NewService(logs), DayNotes: none,
		WebPush: none, Devices: none, APIKeys: none})
	exports := map[string]http.HandlerFunc{"JSON": myData.Export, "CSV": NewLogHandler(logsvc.NewService(logs)).ExportMine}
	get := func(export http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/me/export", nil)
		r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: 1}))
		w := httptest.NewRecorder()
		export(w, r)
		return w
	}

	add(exportRowLimit + 1)
	for name, export := range exports {
		if w := get(export); w.Code != http.StatusOK {
			t.Fatalf("%s export of %d logs: %d %s", name, exportRowLimit+1, w.Code, w.Body)
		}
	}
	add(personalExportRowLimit - exportRowLimit)
	for name, export := range exports {
		w := get(export)
		if body := w.Body.String(); w.Code != http.StatusRequestEntityTooLarge || w.Header().Get("Content-Disposition") != "" ||
			!strings.Contains(body, "contact support") || strings.Contains(body, "date range") {
			t.Fatalf("%s export past the personal limit: %d %s", name, w.Code, body)
		}
	}
}
