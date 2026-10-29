package handlers

import (
	"bytes"
	"context"
	"errors"
	"log"
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
// concurrency-bound.
type ExportGate struct {
	mu     sync.Mutex
	active map[int64]bool
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

// run serves next in one of the two slots, if the key holds neither.
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
