package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/operator"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestOperatorHandlersRejectHouseholdOwnerWithoutQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := NewOperatorHandler(operator.NewService(db, 1), audit.NopLogger{})
	for _, tc := range []struct {
		path    string
		method  string
		handler http.HandlerFunc
	}{
		{"/api/operator/v1/summary", "GET", h.Summary},
		{"/api/operator/v1/activity", "GET", h.Activity},
		{"/api/operator/v1/users", "GET", h.Users},
		{"/api/operator/v1/households", "GET", h.Households},
		{"/api/operator/v1/keys", "GET", h.Keys},
		{"/api/operator/v1/keys/123", "DELETE", h.RevokeKey},
	} {
		t.Run(tc.path+tc.method, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			// This user has the highest household role but is not platform owner.
			r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: 2, EmailVerified: true, Role: "owner"}))
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "private-canary") {
				t.Fatalf("household owner reached operator endpoint: %d %q", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("sensitive response was cacheable")
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unauthorized call reached database: %v", err)
	}
}

func TestOperatorInvalidBearerCannotFallBackToOwnerCookie(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := NewOperatorHandler(operator.NewService(db, 1), audit.NopLogger{})
	r := httptest.NewRequest(http.MethodGet, "/api/operator/v1/users", nil)
	r.Header.Set("Authorization", "Bearer invalid")
	r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: 1, EmailVerified: true}))
	w := httptest.NewRecorder()
	h.Users(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer used owner's cookie: %d", w.Code)
	}
}

func TestOperatorHandlersReportAndManageScopedKeys(t *testing.T) {
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var ownerID int64
	if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified)
		VALUES('operator@example.com','hash','Operator',true) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,authenticated_at,auth_version)
		VALUES('operator-session',$1,'operator-hash',$2,$3,$3,0)`, ownerID, time.Now().Add(time.Hour), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO households(name,invite_code) VALUES('Operator household','operator-handler-test')`); err != nil {
		t.Fatal(err)
	}
	h := NewOperatorHandler(operator.NewService(db, ownerID), audit.NopLogger{})
	owner := auth.User{ID: ownerID, EmailVerified: true, SessionHash: "operator-hash"}
	call := func(method, path, body, bearer string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		} else {
			r = r.WithContext(middleware.WithUser(r.Context(), owner))
		}
		if method == http.MethodDelete {
			r.SetPathValue("id", strings.TrimPrefix(path, "/api/operator/v1/keys/"))
		}
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("operator response was cacheable")
		}
		return w
	}
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
		want    string
	}{
		{"/api/operator/v1/summary", h.Summary, `"registeredUsers":1`},
		{"/api/operator/v1/activity?days=30", h.Activity, `"days":`},
		{"/api/operator/v1/users?limit=1", h.Users, `operator@example.com`},
		{"/api/operator/v1/households?limit=1", h.Households, `Operator household`},
	} {
		w := call(http.MethodGet, tc.path, "", "", tc.handler)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/api/operator/v1/activity?days=31", h.Activity},
		{"/api/operator/v1/users?limit=101", h.Users},
		{"/api/operator/v1/households?after=-1", h.Households},
	} {
		if w := call(http.MethodGet, tc.path, "", "", tc.handler); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid report page accepted: %s %d", tc.path, w.Code)
		}
	}
	if w := call(http.MethodPost, "/api/operator/v1/keys", `{"name":"bad","scope":"write","days":7}`, "", h.Keys); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid key settings accepted: %d", w.Code)
	}
	w := call(http.MethodPost, "/api/operator/v1/keys", `{"name":"Summary report","scope":"summary","days":7}`, "", h.Keys)
	if w.Code != http.StatusCreated {
		t.Fatalf("key creation failed: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Key   operator.Key `json:"key"`
		Token string       `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.Token == "" {
		t.Fatalf("missing cleartext key: %+v %v", created, err)
	}
	if w := call(http.MethodGet, "/api/operator/v1/keys", "", "", h.Keys); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Summary report") {
		t.Fatalf("owner could not list keys: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/operator/v1/summary", "", created.Token, h.Summary); w.Code != http.StatusOK {
		t.Fatalf("summary key could not read summary: %d", w.Code)
	}
	if w := call(http.MethodGet, "/api/operator/v1/users", "", created.Token, h.Users); w.Code != http.StatusUnauthorized {
		t.Fatalf("summary key read email report: %d", w.Code)
	}
	if w := call(http.MethodDelete, "/api/operator/v1/keys/"+created.Key.ID, "", "", h.RevokeKey); w.Code != http.StatusNoContent {
		t.Fatalf("key revocation failed: %d", w.Code)
	}
	if w := call(http.MethodGet, "/api/operator/v1/summary", "", created.Token, h.Summary); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key read summary: %d", w.Code)
	}
	w = call(http.MethodPost, "/api/operator/v1/keys", `{"name":"Full report","scope":"full","days":30}`, "", h.Keys)
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &created) != nil {
		t.Fatalf("full key creation failed: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/operator/v1/users", "", created.Token, h.Users); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "operator@example.com") {
		t.Fatalf("full key could not read users: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/operator/v1/keys", "", created.Token, h.Keys); w.Code != http.StatusForbidden {
		t.Fatalf("bearer key managed credentials: %d", w.Code)
	}
	if w := call(http.MethodDelete, "/api/operator/v1/keys/invalid", "", "", h.RevokeKey); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid key ID was accepted: %d", w.Code)
	}
	if w := call(http.MethodPatch, "/api/operator/v1/keys", "", "", h.Keys); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported key method was accepted: %d", w.Code)
	}
}
