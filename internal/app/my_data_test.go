package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func login(t *testing.T, srv http.Handler, email string) string {
	t.Helper()
	rec := srvRequest(srv, http.MethodPost, "/api/auth/login", fmt.Sprintf(`{"email":%q,"password":"password123"}`, email), "tok", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "nabu_session" {
			return c.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

func mustOK(t *testing.T, rec interface{ Result() *http.Response }, what string) {
	t.Helper()
	if code := rec.Result().StatusCode; code >= 300 {
		body, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("%s: %d %s", what, code, body)
	}
}

type myDataFile struct {
	About   string `json:"about"`
	Account struct {
		Email       string `json:"email"`
		HasPassword bool   `json:"hasPassword"`
	} `json:"account"`
	Households []struct {
		Name      string `json:"name"`
		Role      string `json:"role"`
		Suspended bool   `json:"suspended"`
	} `json:"households"`
	Preferences struct {
		VolumeUnit string `json:"volumeUnit"`
	} `json:"preferences"`
	Logs []struct {
		Note      string `json:"note"`
		Household string `json:"household"`
		Activity  string `json:"activity"`
	} `json:"logs"`
	DayNotes []struct {
		Household string `json:"household"`
		Date      string `json:"date"`
		Note      string `json:"note"`
	} `json:"dayNotes"`
	SignIns []struct {
		Current bool `json:"current"`
	} `json:"signIns"`
	Devices []struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Service string `json:"service"`
	} `json:"devices"`
	APIKeys []struct {
		Name string `json:"name"`
	} `json:"apiKeys"`
}

// GET /api/me/export holds what Nabu keeps about the user, and none of the
// tokens, hashes or push endpoints that act as credentials.
func TestMyDataExportHoldsTheUsersDataAndNoCredentials(t *testing.T) {
	srv := newTestSrv(t)
	if closer, ok := srv.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	ann := registerAs(t, srv, "ann@example.com")
	mustOK(t, srvRequest(srv, http.MethodPost, "/api/household", `{"name":"Home"}`, "tok", ann), "create household")
	mustOK(t, srvRequest(srv, http.MethodPost, "/api/chores/seed-defaults", "", "tok", ann), "seed")
	var chores struct {
		Chores []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"chores"`
	}
	_ = json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/chores", "", "", ann).Body.Bytes(), &chores)
	if len(chores.Chores) == 0 {
		t.Fatal("no chores")
	}
	mustOK(t, srvRequest(srv, http.MethodPost, "/api/logs", fmt.Sprintf(`{"choreId":%d,"note":"fed at noon"}`, chores.Chores[0].ID), "tok", ann), "log")
	mustOK(t, srvRequest(srv, http.MethodPut, "/api/day-notes/2026-10-01", `{"note":"vet visit"}`, "tok", ann), "day note")
	mustOK(t, srvRequest(srv, http.MethodPatch, "/api/preferences", `{"volumeUnit":"oz"}`, "tok", ann), "preferences")
	const endpoint = "https://web.push.apple.com/QGhbW-SECRET-CAPABILITY-PATH"
	mustOK(t, srvRequest(srv, http.MethodPost, "/api/push/subscribe", `{"bindingId":"binding-0123456789abcdef0123456789abcdef",
 "subscription":{"endpoint":"`+endpoint+`","keys":{"p256dh":"P256DH-SECRET","auth":"AUTH-SECRET"}}}`, "tok", ann), "push subscribe")
	apnsDevice := strings.Repeat("ab", 32) // a fake 64-hex device token
	mustOK(t, srvRequest(srv, http.MethodPost, "/api/mobile/apns/register", `{"token":"`+apnsDevice+`","environment":"sandbox",
 "bundleId":"com.nabu.app","deviceName":"Ann's iPhone"}`, "tok", ann), "apns register")
	keyRec := srvRequest(srv, http.MethodPost, "/api/api-keys", `{"name":"Shortcuts"}`, "tok", ann)
	mustOK(t, keyRec, "api key")
	var issued struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(keyRec.Body.Bytes(), &issued)
	other := login(t, srv, "ann@example.com")

	rec := srvRequest(srv, http.MethodGet, "/api/me/export", "", "", other)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), `filename="nabu-my-data.json"`) {
		t.Fatalf("export: %d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	for _, secret := range []string{ann, other, issued.Token, "SECRET-CAPABILITY-PATH", "P256DH-SECRET", "AUTH-SECRET", apnsDevice, "binding-0123"} {
		if secret == "" || strings.Contains(body, secret) {
			t.Fatalf("the export holds a credential %q:\n%s", secret, body)
		}
	}
	var file myDataFile
	if err := json.Unmarshal(rec.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Account.Email != "ann@example.com" || !file.Account.HasPassword || file.About == "" {
		t.Fatalf("account = %+v", file.Account)
	}
	if len(file.Households) != 1 || file.Households[0].Name != "Home" || file.Households[0].Role != "owner" || file.Households[0].Suspended {
		t.Fatalf("households = %+v", file.Households)
	}
	if file.Preferences.VolumeUnit != "oz" {
		t.Fatalf("preferences = %+v", file.Preferences)
	}
	if len(file.Logs) != 1 || file.Logs[0].Note != "fed at noon" || file.Logs[0].Household != "Home" || file.Logs[0].Activity != chores.Chores[0].Name {
		t.Fatalf("logs = %+v", file.Logs)
	}
	if len(file.DayNotes) != 1 || file.DayNotes[0].Date != "2026-10-01" || file.DayNotes[0].Note != "vet visit" || file.DayNotes[0].Household != "Home" {
		t.Fatalf("day notes = %+v", file.DayNotes)
	}
	current := 0
	for _, s := range file.SignIns {
		if s.Current {
			current++
		}
	}
	if len(file.SignIns) != 2 || current != 1 {
		t.Fatalf("sign-ins = %+v", file.SignIns)
	}
	if len(file.Devices) != 2 || file.Devices[0].Kind != "web push" || file.Devices[0].Service != "web.push.apple.com" ||
		file.Devices[1].Kind != "iPhone" || file.Devices[1].Name != "Ann's iPhone" {
		t.Fatalf("devices = %+v", file.Devices)
	}
	if len(file.APIKeys) != 1 || file.APIKeys[0].Name != "Shortcuts" {
		t.Fatalf("api keys = %+v", file.APIKeys)
	}
	if rec := srvRequest(srv, http.MethodGet, "/api/me/export", "", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("export without a session: %d", rec.Code)
	}
}

// Someone who left every household, perhaps before deleting their account,
// can still download their data. The CSV used to answer 401 "no household".
func TestPersonalExportsWorkWithoutAHousehold(t *testing.T) {
	srv := newTestSrv(t)
	if closer, ok := srv.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	bea := registerAs(t, srv, "bea@example.com")
	for _, path := range []string{"/api/me/export", "/api/me/logs/export"} {
		if rec := srvRequest(srv, http.MethodGet, path, "", "", bea); rec.Code != http.StatusOK {
			t.Fatalf("%s without a household: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	var file myDataFile
	if err := json.Unmarshal(srvRequest(srv, http.MethodGet, "/api/me/export", "", "", bea).Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Households == nil || len(file.Households) != 0 || file.Logs == nil || file.Devices == nil {
		t.Fatalf("empty lists must be [], not null: %+v", file)
	}
}
