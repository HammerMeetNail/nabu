package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/config"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestHTTPLogReadsDoNotUseReservedDeliveryPool(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES(1,'Synthetic','invite')`,
		`INSERT INTO users(id,email,password_hash,display_name,active_household_id) VALUES(1,'synthetic@example.invalid','','Synthetic',1)`,
		`INSERT INTO user_households(user_id,household_id,role) VALUES(1,1,'owner')`,
		`INSERT INTO chores(household_id,name,visibility) VALUES(1,'Synthetic','household')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	token := "synthetic-request-token"
	digest := sha256.Sum256([]byte(token))
	if _, err := auth.NewPostgresStore(db).CreateSession(ctx, 1, 0, base64.RawURLEncoding.EncodeToString(digest[:]), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBMaxOpenConns = 10
	h, err := newServerWithDB(cfg, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := h.(*Server)
	defer s.Close()
	if db.Stats().MaxOpenConnections != 2 || s.deliveryDB.Stats().MaxOpenConnections != 8 {
		t.Fatal("pool split exceeded configured process budget")
	}
	var held []*sql.Conn
	defer func() {
		for _, conn := range held {
			_ = conn.Close()
		}
	}()
	for range database.DeliveryConnections {
		conn, err := s.deliveryDB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	for _, path := range []string{"/api/logs/today", "/api/logs/latest-per-chore", "/api/logs/history", "/api/logs/history?q=needle"} {
		requestCtx, requestCancel := context.WithTimeout(ctx, time.Second)
		r := httptest.NewRequest(http.MethodGet, path, nil).WithContext(requestCtx)
		r.AddCookie(&http.Cookie{Name: "nabu_session", Value: token})
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		requestCancel()
		if w.Code != 200 {
			t.Fatalf("%s depends on delivery pool: %d %s", path, w.Code, w.Body.String())
		}
	}
	if s.deliveryDB.Stats().WaitCount != 0 {
		t.Fatal("HTTP log read attempted delivery checkout")
	}
}
