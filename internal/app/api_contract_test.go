package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

var updateContract = flag.Bool("update-contract", false, "rewrite api/contract from the server's answers")

// contractDir holds the API contract: one file per request the iOS app
// makes, with the server's answer. APIContractTests in ios/NabuTests decodes
// every answer with the app's own models, so a change to these files is an
// API change the app must handle, and CI runs the iOS tests for it.
var contractDir = filepath.Join("..", "..", "api", "contract")

// The values in an answer that change from run to run, replaced with fixed
// ones so that the files change only when the API does.
var (
	contractTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)
	contractDate      = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
)

const (
	contractFixedTimestamp         = "2026-01-02T03:04:05Z"
	contractFixedFractionTimestamp = "2026-01-02T03:04:05.123456Z"
	contractFixedDate              = "2026-01-02"
)

type contractExchange struct {
	Request  contractRequest `json:"request"`
	Status   int             `json:"status"`
	Response any             `json:"response,omitempty"`
}

type contractRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   any    `json:"body,omitempty"`
}

type contractRecorder struct {
	t         *testing.T
	srv       http.Handler
	names     []string
	exchanges map[string]*contractExchange
	// secrets maps a value the server generated at random (an invite code)
	// to the fixed text the files show instead.
	secrets map[string]string
}

// call sends one request the way the iOS app does, checks its status and
// records the exchange as name. It returns the answer decoded.
func (c *contractRecorder) call(name, session, method, path, body string, status int) map[string]any {
	c.t.Helper()
	rec := c.send(session, method, path, body)
	if rec.Code != status {
		c.t.Fatalf("%s: %s %s = %d, want %d: %s", name, method, path, rec.Code, status, rec.Body.String())
	}
	return c.record(name, method, path, body, rec)
}

// setup sends a request the contract does not keep, such as a second log
// that is there to be deleted.
func (c *contractRecorder) setup(session, method, path, body string, status int) map[string]any {
	c.t.Helper()
	rec := c.send(session, method, path, body)
	if rec.Code != status {
		c.t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, status, rec.Body.String())
	}
	answer, _ := decodeContractJSON(c.t, path, rec.Body.Bytes()).(map[string]any)
	return answer
}

func (c *contractRecorder) send(session, method, path, body string) *httptest.ResponseRecorder {
	csrf := ""
	if method != http.MethodGet {
		csrf = "contract-csrf"
	}
	return srvRequest(c.srv, method, path, body, csrf, session)
}

func (c *contractRecorder) record(name, method, path, body string, rec *httptest.ResponseRecorder) map[string]any {
	c.t.Helper()
	if _, dup := c.exchanges[name]; dup {
		c.t.Fatalf("%s recorded twice", name)
	}
	ex := &contractExchange{Request: contractRequest{Method: method, Path: path}, Status: rec.Code}
	if body != "" {
		ex.Request.Body = decodeContractJSON(c.t, name, []byte(body))
	}
	if rec.Body.Len() > 0 {
		ex.Response = decodeContractJSON(c.t, name, rec.Body.Bytes())
		c.collectSecrets(ex.Response)
	}
	c.names = append(c.names, name)
	c.exchanges[name] = ex
	answer, _ := ex.Response.(map[string]any)
	return answer
}

// signIn sends a request that starts a session and returns the session. It
// records the exchange as name unless name is empty.
func (c *contractRecorder) signIn(name, method, path, body string, status int) string {
	c.t.Helper()
	rec := c.send("", method, path, body)
	if rec.Code != status {
		c.t.Fatalf("%s: %s %s = %d, want %d: %s", name, method, path, rec.Code, status, rec.Body.String())
	}
	if name != "" {
		c.record(name, method, path, body, rec)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "nabu_session" {
			return cookie.Value
		}
	}
	c.t.Fatalf("%s: no session cookie", name)
	return ""
}

func decodeContractJSON(t *testing.T, name string, data []byte) any {
	t.Helper()
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s: not JSON: %v: %s", name, err, data)
	}
	return v
}

// collectSecrets notes the invite codes in an answer, numbering them in the
// order they first appear.
func (c *contractRecorder) collectSecrets(v any) {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			code, ok := x[k].(string)
			if ok && (k == "code" || k == "inviteCode") && code != "" {
				if _, seen := c.secrets[code]; !seen {
					c.secrets[code] = fmt.Sprintf("INVITE%04d", len(c.secrets)+1)
				}
				continue
			}
			c.collectSecrets(x[k])
		}
	case []any:
		for _, val := range x {
			c.collectSecrets(val)
		}
	}
}

func (c *contractRecorder) normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = c.normalize(val)
		}
	case []any:
		for i, val := range x {
			x[i] = c.normalize(val)
		}
	case string:
		return c.normalizeString(x)
	}
	return v
}

func (c *contractRecorder) normalizeString(s string) string {
	for secret, fixed := range c.secrets {
		s = strings.ReplaceAll(s, secret, fixed)
	}
	if m := contractTimestamp.FindStringSubmatch(s); m != nil {
		if m[1] != "" {
			return contractFixedFractionTimestamp
		}
		return contractFixedTimestamp
	}
	return contractDate.ReplaceAllString(s, contractFixedDate)
}

// files renders every exchange as the file it is kept in.
func (c *contractRecorder) files() map[string][]byte {
	c.t.Helper()
	out := make(map[string][]byte, len(c.names))
	for _, name := range c.names {
		ex := c.exchanges[name]
		ex.Request.Path = c.normalizeString(ex.Request.Path)
		ex.Request.Body = c.normalize(ex.Request.Body)
		ex.Response = c.normalize(ex.Response)
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(ex); err != nil {
			c.t.Fatalf("%s: %v", name, err)
		}
		out[name+".json"] = buf.Bytes()
	}
	return out
}

func contractInt(t *testing.T, v any, path ...string) int64 {
	t.Helper()
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("no %s in %v", strings.Join(path, "."), v)
		}
		v = m[key]
	}
	n, ok := v.(json.Number)
	if !ok {
		t.Fatalf("%s is %v, not a number", strings.Join(path, "."), v)
	}
	i, err := n.Int64()
	if err != nil {
		t.Fatalf("%s: %v", strings.Join(path, "."), err)
	}
	return i
}

func contractString(t *testing.T, v any, path ...string) string {
	t.Helper()
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("no %s in %v", strings.Join(path, "."), v)
		}
		v = m[key]
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s is %v, not a string", strings.Join(path, "."), v)
	}
	return s
}

// contractChore finds the household's activity called name.
func contractChore(t *testing.T, chores map[string]any, name string) int64 {
	t.Helper()
	list, _ := chores["chores"].([]any)
	for _, ch := range list {
		if contractString(t, ch, "name") == name {
			return contractInt(t, ch, "id")
		}
	}
	t.Fatalf("no activity %q", name)
	return 0
}

// TestAPIContract makes the requests the iOS app makes, in the order a
// household's life brings them, and compares the answers with api/contract.
// After an intended API change, rewrite the files with
//
//	go test ./internal/app -run '^TestAPIContract$' -update-contract
//
// and commit them: CI then runs the iOS tests against the new answers.
func TestAPIContract(t *testing.T) {
	srv := newTestSrv(t)
	if closer, ok := srv.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	c := &contractRecorder{t: t, srv: srv, exchanges: map[string]*contractExchange{}, secrets: map[string]string{}}

	// Logs go at a fixed hour of the day before, so the answers do not depend
	// on when the test runs.
	day := time.Now().AddDate(0, 0, -1)
	date := day.Format(time.DateOnly)
	at := time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, time.Local).UTC().Format(time.RFC3339)
	nextWeek := time.Now().AddDate(0, 0, 7).Format(time.DateOnly)
	const password = "contract password"

	owner := c.signIn("auth-register", http.MethodPost, "/api/auth/register",
		`{"email":"owner@example.com","password":"`+password+`"}`, http.StatusCreated)
	c.call("me", owner, http.MethodGet, "/api/me", "", http.StatusOK)
	c.call("household-create", owner, http.MethodPost, "/api/household", `{"name":"Contract Home","initials":"CH"}`, http.StatusCreated)
	c.call("packs", owner, http.MethodGet, "/api/packs", "", http.StatusOK)
	c.call("chores-add-packs", owner, http.MethodPost, "/api/chores/packs",
		`{"packs":[{"key":"baby","names":["Ada"]},{"key":"home"}]}`, http.StatusOK)
	c.call("chores-seed-defaults", owner, http.MethodPost, "/api/chores/seed-defaults", "", http.StatusOK)
	chores := c.call("chores", owner, http.MethodGet, "/api/chores", "", http.StatusOK)
	feed := contractChore(t, chores, "Feed Ada")
	trash := contractChore(t, chores, "Take Out Trash")

	created := c.call("chore-create", owner, http.MethodPost, "/api/chores",
		`{"name":"Water Fern","icon":"🌿","color":"#22C55E","category":"plants","indicatorLabels":["Misted"],"indicatorDefaults":[],"followUpEnabled":false,"metricType":"none","visibility":"household"}`,
		http.StatusCreated)
	fern := contractInt(t, created, "chore", "id")
	c.call("chore-update", owner, http.MethodPatch, fmt.Sprintf("/api/chores/%d", fern), `{"name":"Water the Fern"}`, http.StatusOK)
	c.call("chore-delete", owner, http.MethodDelete, fmt.Sprintf("/api/chores/%d", fern), "", http.StatusOK)
	c.call("chore-restore-default", owner, http.MethodPost, fmt.Sprintf("/api/chores/%d/restore-default", trash), "", http.StatusOK)

	logged := c.call("log-create", owner, http.MethodPost, "/api/logs",
		fmt.Sprintf(`{"choreId":%d,"note":"","indicators":[],"date":%q,"hour":9,"completedAt":%q,"volumeML":120,"metricUnit":"mL","idempotencyKey":"00000000-0000-4000-8000-000000000001"}`, feed, date, at),
		http.StatusCreated)
	feedLog := contractInt(t, logged, "log", "id")
	logged = c.setup(owner, http.MethodPost, "/api/logs",
		fmt.Sprintf(`{"choreId":%d,"note":"","indicators":[],"date":%q,"hour":9,"completedAt":%q,"idempotencyKey":"00000000-0000-4000-8000-000000000002"}`, trash, date, at),
		http.StatusCreated)
	trashLog := contractInt(t, logged, "log", "id")
	c.call("log-update", owner, http.MethodPatch, fmt.Sprintf("/api/logs/%d", feedLog), `{"note":"Burped after"}`, http.StatusOK)
	c.call("log-delete", owner, http.MethodDelete, fmt.Sprintf("/api/logs/%d", trashLog), "", http.StatusOK)
	c.call("logs-today", owner, http.MethodGet, "/api/logs/today?date="+date, "", http.StatusOK)
	c.call("logs-latest-per-chore", owner, http.MethodGet, "/api/logs/latest-per-chore", "", http.StatusOK)
	c.call("logs-recent-amounts", owner, http.MethodGet, fmt.Sprintf("/api/logs/recent-amounts?choreId=%d&unit=mL", feed), "", http.StatusOK)
	c.call("logs-history", owner, http.MethodGet, "/api/logs/history", "", http.StatusOK)
	c.call("logs-history-search", owner, http.MethodGet, "/api/logs/history?q=Burped", "", http.StatusOK)

	// The schedule starts next week, so no reminder comes due during the test.
	scheduled := c.call("schedule-create", owner, http.MethodPost, "/api/schedules",
		fmt.Sprintf(`{"choreId":%d,"frequencyType":"daily","timePeriod":"morning","specificTime":"08:00","daysOfWeek":[],"startDate":%q,"isActive":true}`, feed, nextWeek),
		http.StatusCreated)
	schedule := contractInt(t, scheduled, "schedule", "id")
	c.call("schedules", owner, http.MethodGet, "/api/schedules", "", http.StatusOK)
	c.call("schedule-update", owner, http.MethodPatch, fmt.Sprintf("/api/schedules/%d", schedule), `{"specificTime":"08:30"}`, http.StatusOK)
	c.call("chore-reminder-pref-update", owner, http.MethodPatch, fmt.Sprintf("/api/chore-reminder-prefs/%d", feed),
		`{"enabled":true,"leadMinutes":15}`, http.StatusOK)
	c.call("chore-reminder-prefs", owner, http.MethodGet, "/api/chore-reminder-prefs", "", http.StatusOK)
	c.call("schedule-delete", owner, http.MethodDelete, fmt.Sprintf("/api/schedules/%d", schedule), "", http.StatusOK)

	c.call("day-note-set", owner, http.MethodPut, "/api/day-notes/"+date, `{"note":"Quiet day"}`, http.StatusOK)
	c.call("day-notes", owner, http.MethodGet, "/api/day-notes", "", http.StatusOK)
	c.call("preferences", owner, http.MethodGet, "/api/preferences", "", http.StatusOK)
	c.call("preferences-update", owner, http.MethodPatch, "/api/preferences", `{"volumeUnit":"oz"}`, http.StatusOK)
	c.call("notification-preferences", owner, http.MethodGet, "/api/notification-preferences", "", http.StatusOK)
	c.call("notification-preferences-update", owner, http.MethodPatch, "/api/notification-preferences",
		`{"pushEnabled":true,"defaultReminderLeadMinutes":10}`, http.StatusOK)
	c.call("apns-register", owner, http.MethodPost, "/api/mobile/apns/register",
		`{"token":"`+strings.Repeat("ab", 32)+`","environment":"production","bundleId":"com.nabu.app","deviceName":"iOS"}`, http.StatusOK)

	// A second member joins by invite.
	invited := c.call("invite-create", owner, http.MethodPost, "/api/household/invites", "", http.StatusCreated)
	code := contractString(t, invited, "invite", "code")
	c.call("invite-preview", "", http.MethodGet, "/api/household/join/preview?code="+code, "", http.StatusOK)
	member := c.signIn("auth-register-member", http.MethodPost, "/api/auth/register",
		`{"email":"member@example.com","password":"`+password+`"}`, http.StatusCreated)
	joined := c.call("household-join", member, http.MethodPost, "/api/household/join", `{"inviteCode":"`+code+`"}`, http.StatusOK)
	memberID := contractInt(t, c.call("me-member", member, http.MethodGet, "/api/me", "", http.StatusOK), "user", "id")
	household := contractInt(t, joined, "household", "id")

	// The join reaches the owner as a notification, which a worker fans out.
	var notifications map[string]any
	for deadline := time.Now().Add(10 * time.Second); ; {
		notifications = c.setup(owner, http.MethodGet, "/api/notifications", "", http.StatusOK)
		if list, _ := notifications["notifications"].([]any); len(list) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	notifications = c.call("notifications", owner, http.MethodGet, "/api/notifications", "", http.StatusOK)
	list, _ := notifications["notifications"].([]any)
	if len(list) != 1 {
		t.Fatalf("owner has %d notifications, want the join's", len(list))
	}
	notification := contractInt(t, list[0], "id")
	c.call("notification-read", owner, http.MethodPost, fmt.Sprintf("/api/notifications/%d/read", notification), "", http.StatusOK)
	c.call("notifications-read-all", owner, http.MethodPost, "/api/notifications/read-all", "", http.StatusOK)
	c.call("notification-delete", owner, http.MethodDelete, fmt.Sprintf("/api/notifications/%d", notification), "", http.StatusOK)
	c.call("notifications-clear", owner, http.MethodDelete, "/api/notifications", "", http.StatusOK)

	pending := c.call("invite-create-pending", owner, http.MethodPost, "/api/household/invites", "", http.StatusCreated)
	c.call("household", owner, http.MethodGet, "/api/household", "", http.StatusOK)
	c.call("household-update", owner, http.MethodPatch, "/api/household", `{"name":"Contract House","initials":"CH"}`, http.StatusOK)
	c.call("invite-delete", owner, http.MethodDelete, fmt.Sprintf("/api/household/invites/%d", contractInt(t, pending, "invite", "id")), "", http.StatusOK)
	c.call("households", owner, http.MethodGet, "/api/households", "", http.StatusOK)
	c.call("household-activate", owner, http.MethodPost, fmt.Sprintf("/api/households/%d/activate", household), "", http.StatusOK)

	c.call("member-role", owner, http.MethodPatch, fmt.Sprintf("/api/household/members/%d", memberID), `{"role":"admin"}`, http.StatusOK)
	// Suspending a member ends their sessions; signed in again, they learn
	// which household disabled them.
	c.call("member-access", owner, http.MethodPatch, fmt.Sprintf("/api/household/members/%d/access", memberID), `{"suspended":true}`, http.StatusOK)
	c.call("error-signed-out", member, http.MethodGet, "/api/household", "", http.StatusUnauthorized)
	member = c.signIn("", http.MethodPost, "/api/auth/login", `{"email":"member@example.com","password":"`+password+`"}`, http.StatusOK)
	c.call("error-access-suspended", member, http.MethodGet, "/api/household", "", http.StatusForbidden)
	c.setup(owner, http.MethodPatch, fmt.Sprintf("/api/household/members/%d/access", memberID), `{"suspended":false}`, http.StatusOK)
	c.call("error-invalid", owner, http.MethodPost, "/api/chores", `{"name":""}`, http.StatusBadRequest)

	// The owner hands the household over and leaves; the new owner removes
	// a third member.
	c.call("household-transfer", owner, http.MethodPost, "/api/household/transfer", fmt.Sprintf(`{"newOwnerId":%d}`, memberID), http.StatusOK)
	c.call("household-leave", owner, http.MethodPost, "/api/household/leave", "", http.StatusOK)
	third := c.setup(member, http.MethodPost, "/api/household/invites", "", http.StatusCreated)
	other := c.signIn("", http.MethodPost, "/api/auth/register",
		`{"email":"third@example.com","password":"`+password+`"}`, http.StatusCreated)
	c.setup(other, http.MethodPost, "/api/household/join", `{"inviteCode":"`+contractString(t, third, "invite", "code")+`"}`, http.StatusOK)
	otherID := contractInt(t, c.setup(other, http.MethodGet, "/api/me", "", http.StatusOK), "user", "id")
	c.call("member-remove", member, http.MethodDelete, fmt.Sprintf("/api/household/members/%d", otherID), "", http.StatusOK)

	c.call("auth-verification-resend", owner, http.MethodPost, "/api/auth/email/verification/resend", "", http.StatusOK)
	c.call("auth-password-change", owner, http.MethodPost, "/api/auth/password",
		`{"current_password":"`+password+`","new_password":"contract password 2"}`, http.StatusOK)
	c.call("auth-logout", owner, http.MethodPost, "/api/auth/logout", "", http.StatusOK)
	c.signIn("auth-login", http.MethodPost, "/api/auth/login", `{"email":"owner@example.com","password":"contract password 2"}`, http.StatusOK)
	c.call("auth-magic-link", "", http.MethodPost, "/api/auth/magic-link/request", `{"email":"owner@example.com"}`, http.StatusOK)
	c.call("auth-password-forgot", "", http.MethodPost, "/api/auth/password/forgot", `{"email":"owner@example.com"}`, http.StatusOK)

	compareContract(t, c.files())
}

// compareContract checks the files against api/contract, or rewrites it.
func compareContract(t *testing.T, files map[string][]byte) {
	t.Helper()
	existing, err := filepath.Glob(filepath.Join(contractDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if *updateContract {
		if err := os.MkdirAll(contractDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, path := range existing {
			if _, keep := files[filepath.Base(path)]; !keep {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(contractDir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	var problems []string
	for _, path := range existing {
		if _, ok := files[filepath.Base(path)]; !ok {
			problems = append(problems, filepath.Base(path)+": no request makes it any more")
		}
	}
	for name, data := range files {
		got, err := os.ReadFile(filepath.Join(contractDir, name))
		switch {
		case os.IsNotExist(err):
			problems = append(problems, name+": missing")
		case err != nil:
			t.Fatal(err)
		case !bytes.Equal(got, data):
			problems = append(problems, name+": the server's answer changed")
		}
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		t.Fatalf("api/contract is out of date:\n  %s\n"+
			"If the change is intended, run go test ./internal/app -run '^TestAPIContract$' -update-contract, "+
			"check that the iOS app still decodes the new answers (APIContractTests), and commit the files.",
			strings.Join(problems, "\n  "))
	}
}
