package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/HammerMeetNail/nabu/internal/middleware"
)

// maxDashboardReads bounds one dashboard request. A Stats page needs about ten
// section reads, up to 15 chore charts and the saved widgets' charts; clients
// split anything larger across requests.
const maxDashboardReads = 64

// dashboardParallelism bounds how many reads one request runs at once, so a
// single page load cannot take over the database pool.
const dashboardParallelism = 4

var dashboardChoreRead = regexp.MustCompile(`^chores/([0-9]{1,19})(/time-series|/summary)?$`)

type statsLocationKey struct{}

type dashboardResult struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// Dashboard answers several Stats reads in one request, so a client loads the
// Stats page in one round trip instead of one per chart. Each `read` names a
// Stats endpoint relative to /api/stats/ with its own query, for example
// read=overview&read=chores%3Fperiod%3Dmonth. Every read runs that endpoint's
// handler for this request's user, so authorization, validation and time zone
// match the endpoint exactly, and keeps its own status: one failed chart never
// fails the page. The ETag lets a client that kept the last response send
// If-None-Match and get 304 when nothing changed.
func (h *StatsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	reads := r.URL.Query()["read"]
	if len(reads) == 0 {
		writeError(w, http.StatusBadRequest, "read required")
		return
	}
	if len(reads) > maxDashboardReads {
		writeError(w, http.StatusBadRequest, "too many reads")
		return
	}

	// One time zone lookup for the page instead of one per read.
	ctx := context.WithValue(r.Context(), statsLocationKey{}, h.userLocation(r))
	requestID := w.Header().Get("X-Request-ID")
	results := make([]dashboardResult, len(reads))
	slots := make(chan struct{}, dashboardParallelism)
	var wg sync.WaitGroup
	for i, read := range reads {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			results[i] = h.dashboardRead(ctx, read, requestID)
		}()
	}
	wg.Wait()

	body, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		writeServerError(w, "failed to encode stats dashboard", err)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	// Household data: never kept by a browser or proxy cache. A client keeps
	// its own copy for the current identity and revalidates it explicitly.
	w.Header().Set("Cache-Control", "no-store")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// dashboardRead runs one read through its endpoint's handler.
func (h *StatsHandler) dashboardRead(ctx context.Context, read, requestID string) (result dashboardResult) {
	defer func() {
		// A panic here would otherwise take down the server: this goroutine is
		// not one the HTTP server recovers.
		if p := recover(); p != nil {
			rec := newBufferedResponse(requestID)
			writeServerError(rec, "failed to load stats", errors.New("stats read panicked"))
			result = rec.result()
		}
	}()
	target, err := url.Parse(read)
	if err != nil || target.Scheme != "" || target.Host != "" || target.User != nil || target.Fragment != "" ||
		strings.HasPrefix(target.Path, "/") {
		return dashboardError(http.StatusBadRequest, "invalid stats read")
	}
	handler, id := h.dashboardHandler(target.Path)
	if handler == nil {
		return dashboardError(http.StatusNotFound, "unknown stats read")
	}
	sub, err := http.NewRequestWithContext(ctx, http.MethodGet,
		(&url.URL{Path: "/api/stats/" + target.Path, RawQuery: target.RawQuery}).String(), nil)
	if err != nil {
		return dashboardError(http.StatusBadRequest, "invalid stats read")
	}
	if id != "" {
		sub.SetPathValue("id", id)
	}
	rec := newBufferedResponse(requestID)
	handler(rec, sub)
	return rec.result()
}

// dashboardHandler maps a read to the handler of the endpoint it names. Only
// Stats reads are reachable; the dashboard itself is not.
func (h *StatsHandler) dashboardHandler(path string) (http.HandlerFunc, string) {
	switch path {
	case "overview":
		return h.Overview, ""
	case "heatmap":
		return h.Heatmap, ""
	case "busy-hours":
		return h.BusyHours, ""
	case "chores":
		return h.ChoreStats, ""
	case "breakdown":
		return h.Breakdown, ""
	case "leaderboard":
		return h.Leaderboard, ""
	case "top-chores":
		return h.TopChores, ""
	case "feeding-gaps":
		return h.FeedingGaps, ""
	case "streaks":
		return h.Streaks, ""
	case "recap":
		return h.Recap, ""
	}
	if m := dashboardChoreRead.FindStringSubmatch(path); m != nil {
		switch m[2] {
		case "/time-series":
			return h.ChoreTimeSeries, m[1]
		case "/summary":
			return h.ChoreSummary, m[1]
		default:
			return h.ChoreStatsByID, m[1]
		}
	}
	return nil, ""
}

func dashboardError(status int, message string) dashboardResult {
	data, _ := json.Marshal(map[string]string{"error": message})
	return dashboardResult{Status: status, Data: data}
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		if c := strings.TrimSpace(candidate); c == etag || c == "W/"+etag || c == "*" {
			return true
		}
	}
	return false
}

// bufferedResponse captures one dashboard read's response in memory.
type bufferedResponse struct {
	header http.Header
	status int
	wrote  bool
	body   bytes.Buffer
}

func newBufferedResponse(requestID string) *bufferedResponse {
	rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	if requestID != "" {
		// Errors logged by a read carry the dashboard request's ID.
		rec.header.Set("X-Request-ID", requestID)
	}
	return rec
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if !b.wrote {
		b.status, b.wrote = status, true
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.wrote = true
	return b.body.Write(p)
}

func (b *bufferedResponse) result() dashboardResult {
	data := bytes.TrimSpace(b.body.Bytes())
	if !json.Valid(data) {
		return dashboardError(http.StatusInternalServerError, "failed to load stats")
	}
	return dashboardResult{Status: b.status, Data: data}
}
