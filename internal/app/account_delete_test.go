package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func registerAs(t *testing.T, srv http.Handler, email string) string {
	t.Helper()
	rec := srvRequest(srv, http.MethodPost, "/api/auth/register",
		fmt.Sprintf(`{"email":%q,"password":"password123"}`, email), "tok", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "nabu_session" {
			return c.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

// Deleting an account must not erase the household's shared history: by
// default the departing member's logs stay, attributed to "Former member".
func TestDeleteAccountKeepsSharedLogsAsFormerMember(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		wantLogs int
	}{
		{"default", `{"confirm":"DELETE"}`, 2},
		{"delete my logs", `{"confirm":"DELETE","deleteLogs":true}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestSrv(t)
			if closer, ok := srv.(io.Closer); ok {
				t.Cleanup(func() { _ = closer.Close() })
			}
			ann := registerAs(t, srv, "ann@example.com")
			if rec := srvRequest(srv, http.MethodPost, "/api/household", `{"name":"Home"}`, "tok", ann); rec.Code >= 300 {
				t.Fatalf("create household: %d %s", rec.Code, rec.Body.String())
			}
			srvRequest(srv, http.MethodPost, "/api/chores/seed-defaults", "", "tok", ann)
			var hh struct {
				Household struct {
					InviteCode string `json:"inviteCode"`
				} `json:"household"`
			}
			_ = json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/household", "", "", ann).Body.Bytes(), &hh)
			var chores struct {
				Chores []struct {
					ID int64 `json:"id"`
				} `json:"chores"`
			}
			_ = json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/chores", "", "", ann).Body.Bytes(), &chores)
			if hh.Household.InviteCode == "" || len(chores.Chores) == 0 {
				t.Fatal("household setup failed")
			}
			ben := registerAs(t, srv, "ben@example.com")
			if rec := srvRequest(srv, http.MethodPost, "/api/household/join", fmt.Sprintf(`{"inviteCode":%q}`, hh.Household.InviteCode), "tok", ben); rec.Code >= 300 {
				t.Fatalf("join: %d %s", rec.Code, rec.Body.String())
			}
			log := fmt.Sprintf(`{"choreId":%d,"note":"night feed"}`, chores.Chores[0].ID)
			for _, s := range []string{ann, ben} {
				if rec := srvRequest(srv, http.MethodPost, "/api/logs", log, "tok", s); rec.Code >= 300 {
					t.Fatalf("log: %d %s", rec.Code, rec.Body.String())
				}
			}
			// "Download my data" returns only Ben's own logs, with names.
			export := srvRequest(srv, http.MethodGet, "/api/me/logs/export", "", "", ben)
			if export.Code != http.StatusOK {
				t.Fatalf("export: %d %s", export.Code, export.Body.String())
			}
			rows := strings.Split(strings.TrimSpace(export.Body.String()), "\n")
			if len(rows) != 2 || !strings.HasPrefix(rows[1], "Home,") || !strings.Contains(rows[1], "night feed") {
				t.Fatalf("personal export = %q", export.Body.String())
			}
			if rec := srvRequest(srv, http.MethodDelete, "/api/me", tc.body, "tok", ben); rec.Code != http.StatusOK {
				t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
			}

			var today struct {
				Logs []struct {
					UserID int64 `json:"userId"`
				} `json:"logs"`
			}
			_ = json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/logs/today", "", "", ann).Body.Bytes(), &today)
			if len(today.Logs) != tc.wantLogs {
				t.Fatalf("logs after delete = %d, want %d", len(today.Logs), tc.wantLogs)
			}
			var household struct {
				HistoricalMembers []struct {
					UserID      int64  `json:"userId"`
					DisplayName string `json:"displayName"`
				} `json:"historicalMembers"`
			}
			_ = json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/household", "", "", ann).Body.Bytes(), &household)
			if tc.wantLogs == 1 {
				if len(household.HistoricalMembers) != 0 {
					t.Fatalf("historical members = %+v, want none", household.HistoricalMembers)
				}
				return
			}
			if len(household.HistoricalMembers) != 1 || household.HistoricalMembers[0].DisplayName != "Former member" || household.HistoricalMembers[0].UserID >= 0 {
				t.Fatalf("historical members = %+v", household.HistoricalMembers)
			}
			found := false
			for _, l := range today.Logs {
				found = found || l.UserID == household.HistoricalMembers[0].UserID
			}
			if !found {
				t.Fatal("retained log is not attributed to the former member")
			}
		})
	}
}
