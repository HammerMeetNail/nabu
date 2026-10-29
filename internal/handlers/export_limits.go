package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
)

const exportRowLimit = 10000

// A personal export has no date range to narrow, so each of its reads may
// return more rows: 40,000 logs is years of a baby's feeds and changes. The
// byte limit still bounds the file.
const personalExportRowLimit = 40000
const exportByteLimit = 16 << 20
const exportDuration = 20 * time.Second

// ExportGate is shared by every export route in one server. There is no waiting
// queue: a household or user cannot occupy both slots and all allocations are
// concurrency-bound. The two slots bound this instance's memory; with a shared
// lock, one export per household or user also holds across every instance.
type ExportGate struct {
	mu     sync.Mutex
	active map[int64]bool
	lock   ExportLock
}

// ExportLock holds an export's gate key (a household's ID, or a user's negated
// ID) across every instance until release is called. held is false while
// another instance's export holds it.
type ExportLock func(ctx context.Context, key int64) (release func(), held bool, err error)

// ShareAcrossInstances makes one export per household or user hold across
// instances.
func (g *ExportGate) ShareAcrossInstances(lock ExportLock) { g.lock = lock }

// PostgresExportLock holds an export with a session advisory lock on a
// dedicated connection, keyed by exportLockKey. The lock is released when the
// export ends, or with its connection.
func PostgresExportLock(db *sql.DB) ExportLock {
	return func(ctx context.Context, gateKey int64) (func(), bool, error) {
		key := exportLockKey(gateKey)
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, false, err
		}
		var held bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&held); err != nil || !held {
			_ = conn.Close()
			return nil, false, err
		}
		return func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
				// Never return a connection that may still hold the lock to the
				// pool: discarding it ends the lock with its session.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
		}, true, nil
	}
}

// exportLockKey is an export's advisory-lock key, from its gate key. The app's
// other advisory keys are positive constants, or the negated household ID for
// the household's delivery and revocation lock (household.PostgresStore.WithMember),
// so export keys count up from the lowest key: households' from math.MinInt64
// and users' from 2^61 above it. The ranges stay apart while IDs are below 2^61.
func exportLockKey(gateKey int64) int64 {
	if gateKey < 0 {
		return math.MinInt64 + 1<<61 - gateKey
	}
	return math.MinInt64 + gateKey
}

func NewExportGate() *ExportGate { return &ExportGate{active: map[int64]bool{}} }
func (g *ExportGate) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok || user.HouseholdID == nil {
			writeError(w, http.StatusUnauthorized, "no household")
			return
		}
		g.run(w, r, *user.HouseholdID, next)
	}
}

// WrapUser bounds a personal export the same way, keyed by the user instead
// of a household, so someone with no current household (who left every one,
// perhaps before deleting their account) can still download their data. It
// shares the two server slots with household exports.
func (g *ExportGate) WrapUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		// Negative keys can't meet a household's positive ID.
		g.run(w, r, -user.ID, next)
	}
}

// run serves next in one of the two slots, if the key holds neither and, with
// a shared lock, no other instance is exporting for it.
func (g *ExportGate) run(w http.ResponseWriter, r *http.Request, id int64, next http.HandlerFunc) {
	g.mu.Lock()
	busy := len(g.active) >= 2 || g.active[id]
	if !busy {
		g.active[id] = true
	}
	g.mu.Unlock()
	if busy {
		w.Header().Set("Retry-After", "20")
		writeError(w, http.StatusTooManyRequests, "an export is already running; try again shortly")
		return
	}
	defer func() { g.mu.Lock(); delete(g.active, id); g.mu.Unlock() }()
	if g.lock != nil {
		release, held, err := g.lock(r.Context(), id)
		if err != nil {
			writeServerError(w, "export failed", err)
			return
		}
		if !held {
			w.Header().Set("Retry-After", "20")
			writeError(w, http.StatusTooManyRequests, "an export is already running; try again shortly")
			return
		}
		defer release()
	}
	next(w, r)
}

type exportBuffer struct {
	data bytes.Buffer
	ctx  context.Context
}

func (b *exportBuffer) Len() int      { return b.data.Len() }
func (b *exportBuffer) Bytes() []byte { return b.data.Bytes() }
func (b *exportBuffer) Write(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > exportByteLimit-b.Len() {
		return 0, readlimit.ErrExceeded
	}
	return b.data.Write(p)
}

func exportContext(r *http.Request) (*http.Request, context.CancelFunc) {
	return limitedExportContext(r, exportRowLimit)
}

func personalExportContext(r *http.Request) (*http.Request, context.CancelFunc) {
	return limitedExportContext(r, personalExportRowLimit)
}

func limitedExportContext(r *http.Request, rows int) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(r.Context(), exportDuration)
	return r.WithContext(readlimit.With(ctx, rows)), cancel
}

func exportError(w http.ResponseWriter, err error) {
	if errors.Is(err, readlimit.ErrExceeded) {
		writeError(w, http.StatusRequestEntityTooLarge, readlimit.ErrExceeded.Error())
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "export took too long; choose a smaller date range")
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	writeServerError(w, "failed to prepare export", err)
}

// personalExportError is exportError for the user's own data, which has no
// date range to narrow.
func personalExportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, readlimit.ErrExceeded):
		writeError(w, http.StatusRequestEntityTooLarge, "your data is too large for one download; contact support for a copy")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "preparing your data took too long; please retry")
	default:
		exportError(w, err)
	}
}

// Encode the complete bounded file before committing success. Content-Length
// lets clients detect an interrupted body instead of saving a truncated CSV.
func sendExport(w http.ResponseWriter, r *http.Request, b *exportBuffer, filename string) {
	sendFile(w, r, b, filename, "text/csv; charset=utf-8", exportError)
}

// sendFile sends a complete bounded export of any type as a download, or
// reports why it can't with fail.
func sendFile(w http.ResponseWriter, r *http.Request, b *exportBuffer, filename, contentType string, fail func(http.ResponseWriter, error)) {
	if err := r.Context().Err(); err != nil {
		fail(w, err)
		return
	}
	controller := http.NewResponseController(w)
	deadline, _ := r.Context().Deadline()
	if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		fail(w, err)
		return
	}
	// Leave the deadline installed through net/http's final buffered flush.
	// The server sets a fresh deadline when it begins the next request.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	if _, err := w.Write(b.Bytes()); err != nil {
		log.Printf("export: write failed class=%s correlation_id=%s", diagnostics.ErrorClass(err), diagnostics.RequestID())
	}
}

// Both bounds are inclusive calendar dates. Missing values preserve a caller's
// default range; the returned end remains exclusive for store queries.
func exportRange(r *http.Request, start, end time.Time) (time.Time, time.Time, error) {
	for _, name := range []string{"start", "end"} {
		if raw := r.URL.Query().Get(name); raw != "" {
			parsed, err := time.Parse(time.DateOnly, raw)
			if err != nil {
				return start, end, errors.New("invalid export date")
			}
			if name == "start" {
				start = parsed
			} else {
				end = parsed.AddDate(0, 0, 1)
			}
		}
	}
	if !start.Before(end) {
		return start, end, errors.New("start must be on or before end")
	}
	return start, end, nil
}
