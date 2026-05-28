package app

import (
	"bytes"
	"context"
	"database/sql"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dave/choresy/internal/audit"
	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/chore"
	"github.com/dave/choresy/internal/config"
	"github.com/dave/choresy/internal/database"
	"github.com/dave/choresy/internal/handlers"
	"github.com/dave/choresy/internal/household"
	logsvc "github.com/dave/choresy/internal/log"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/middleware"
	"github.com/dave/choresy/internal/notification"
	"github.com/dave/choresy/internal/push"
	"github.com/dave/choresy/internal/schedule"
	"github.com/dave/choresy/internal/stats"
	"github.com/dave/choresy/internal/userprefs"
	"github.com/dave/choresy/internal/version"
	webassets "github.com/dave/choresy/web"
)

type Server struct {
	handler http.Handler
}

func NewServer(cfg config.Config) http.Handler {
	return NewServerWithDB(cfg, nil)
}

func NewServerWithDB(cfg config.Config, db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	var authStore auth.Store
	var householdStore household.Store
	var choreStore chore.Store
	var logStore logsvc.Store
	var userPrefsStore userprefs.Store
	var notifStore notification.Store
	var pushStore push.Store

	if db != nil {
		authStore = auth.NewPostgresStore(db)
		householdStore = household.NewPostgresStore(db)
		choreStore = chore.NewPostgresStore(db)
		logStore = logsvc.NewPostgresStore(db)
		userPrefsStore = userprefs.NewPostgresStore(db)
		notifStore = notification.NewPostgresStore(db)
		pushStore = push.NewPostgresStore(db)
	} else {
		authStore = auth.NewMemoryStore()
		householdStore = household.NewMemoryStore()
		choreStore = chore.NewMemoryStore()
		logStore = logsvc.NewMemoryStore()
		userPrefsStore = userprefs.NewMemoryStore()
		notifStore = notification.NewMemoryStore()
		pushStore = push.NewMemoryStore()
	}

	authService := auth.NewService(authStore)
	authService.SetAuditLogger(audit.NewStdLogger(log.Default()))
	authService.SetMailer(newMailer(cfg), cfg.AppBaseURL)
	authService.SetOIDCProvider(newOIDCProvider(cfg))
	authHandler := handlers.NewAuthHandler(authService, "choresy_session", cfg.ServerSecure)
	householdService := household.NewService(householdStore, authService)
	householdHandler := handlers.NewHouseholdHandler(householdService)
	choreService := chore.NewService(choreStore)
	choreHandler := handlers.NewChoreHandler(choreService)
	logService := logsvc.NewService(logStore)
	logHandler := handlers.NewLogHandler(logService)
	notifService := notification.NewService(notifStore)
	notifHandler := handlers.NewNotificationHandler(notifService)
	logHandler.WithNotification(notifService, choreStore, householdStore)

	var vapidSigner *push.VAPIDSigner
	if cfg.VAPIDPublicKey != "" && cfg.VAPIDPrivateKey != "" {
		var err error
		vapidSigner, err = push.NewVAPIDSigner(cfg.VAPIDPrivateKey, cfg.VAPIDPublicKey, cfg.VAPIDSubject)
		if err != nil {
			log.Printf("warning: failed to create VAPID signer: %v", err)
			vapidSigner = nil
		}
	}
	pushService := push.NewService(pushStore, vapidSigner)
	if vapidSigner != nil {
		notifService.WithPushSender(pushService)
	}
	pushHandler := handlers.NewPushHandler(pushStore)
	userPrefsService := userprefs.NewService(userPrefsStore)
	preferencesHandler := handlers.NewPreferencesHandler(userPrefsService)
	statsService := stats.NewService(logStore, &choreStatsAdapter{choreStore})
	statsHandler := handlers.NewStatsHandler(statsService)

	var scheduleStore schedule.Store
	if db != nil {
		scheduleStore = schedule.NewPostgresStore(db)
	} else {
		scheduleStore = schedule.NewMemoryStore()
	}
	scheduleService := schedule.NewService()
	scheduleHandler := handlers.NewScheduleHandler(scheduleStore, scheduleService)

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitAuthMax, time.Minute)
	rateLimiter.SetTrustedProxies(cfg.TrustedProxyCIDRs)

	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/ready", handlers.Ready)

	mux.HandleFunc("/api/auth/register", method(http.MethodPost, authHandler.Register))
	mux.HandleFunc("/api/auth/login", method(http.MethodPost, authHandler.Login))
	mux.HandleFunc("/api/auth/logout", method(http.MethodPost, authHandler.Logout))
	mux.HandleFunc("/api/me", method(http.MethodGet, authHandler.Me))
	mux.HandleFunc("/api/auth/email/verification/resend", method(http.MethodPost, authHandler.ResendVerification))
	mux.HandleFunc("/api/auth/email/verify", method(http.MethodGet, authHandler.VerifyEmail))
	mux.HandleFunc("/api/auth/magic-link/request", method(http.MethodPost, authHandler.RequestMagicLink))
	mux.HandleFunc("/api/auth/magic-link/consume", method(http.MethodGet, authHandler.ConsumeMagicLink))
	mux.HandleFunc("/api/auth/password/forgot", method(http.MethodPost, authHandler.ForgotPassword))
	mux.HandleFunc("/api/auth/password/reset", method(http.MethodPost, authHandler.ResetPassword))
	mux.HandleFunc("/api/auth/password", method(http.MethodPost, middleware.RequireAuth(authHandler.ChangePassword)))
	mux.HandleFunc("/api/auth/google/login", method(http.MethodGet, authHandler.GoogleLogin))
	mux.HandleFunc("/api/auth/google/callback", method(http.MethodGet, authHandler.GoogleCallback))

	mux.HandleFunc("/api/household", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			householdHandler.Get(w, r)
		case http.MethodPost:
			householdHandler.Create(w, r)
		case http.MethodPatch:
			householdHandler.Update(w, r)
		default:
			w.Header().Set("Allow", "GET, POST, PATCH")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/household/invites", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			householdHandler.ListInvites(w, r)
		case http.MethodPost:
			householdHandler.CreateInvite(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/household/invites/", method(http.MethodDelete, householdHandler.DeleteInvite))
	mux.HandleFunc("/api/household/join", method(http.MethodPost, householdHandler.Join))
	mux.HandleFunc("/api/household/members/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			householdHandler.UpdateMemberRole(w, r)
		case http.MethodDelete:
			householdHandler.RemoveMember(w, r)
		default:
			w.Header().Set("Allow", "PATCH, DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/household/leave", method(http.MethodPost, householdHandler.Leave))
	mux.HandleFunc("/api/household/transfer", method(http.MethodPost, householdHandler.Transfer))

	mux.HandleFunc("/api/chores", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			choreHandler.List(w, r)
		case http.MethodPost:
			choreHandler.Create(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/chores/defaults", method(http.MethodGet, choreHandler.GetDefaults))
	mux.HandleFunc("/api/chores/seed-defaults", method(http.MethodPost, middleware.RequireAuth(choreHandler.SeedDefaults)))
	mux.HandleFunc("/api/chores/reorder", method(http.MethodPost, middleware.RequireAuth(choreHandler.Reorder)))
	mux.HandleFunc("/api/chores/{id}/restore-default", method(http.MethodPost, middleware.RequireAuth(choreHandler.RestoreDefault)))
	mux.HandleFunc("/api/chores/{id}", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			choreHandler.Get(w, r)
		case http.MethodPatch:
			choreHandler.Update(w, r)
		case http.MethodDelete:
			choreHandler.Delete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/logs", method(http.MethodPost, middleware.RequireAuth(logHandler.Create)))
	mux.HandleFunc("/api/logs/{id}", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			logHandler.Delete(w, r)
		case http.MethodPatch:
			logHandler.Update(w, r)
		default:
			w.Header().Set("Allow", "DELETE, PATCH")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/logs/today", method(http.MethodGet, middleware.RequireAuth(logHandler.Today)))
	mux.HandleFunc("/api/logs/week", method(http.MethodGet, middleware.RequireAuth(logHandler.Week)))
	mux.HandleFunc("/api/logs/month", method(http.MethodGet, middleware.RequireAuth(logHandler.Month)))
	mux.HandleFunc("/api/logs/history", method(http.MethodGet, middleware.RequireAuth(logHandler.History)))
	mux.HandleFunc("/api/logs/latest-per-chore", method(http.MethodGet, middleware.RequireAuth(logHandler.LatestPerChore)))

	mux.HandleFunc("/api/notifications", method(http.MethodGet, middleware.RequireAuth(notifHandler.List)))
	mux.HandleFunc("/api/notifications/read-all", method(http.MethodPost, middleware.RequireAuth(notifHandler.MarkAllRead)))
	mux.HandleFunc("/api/notifications/{id}/read", method(http.MethodPost, middleware.RequireAuth(notifHandler.MarkRead)))
	mux.HandleFunc("/api/notifications/{id}", method(http.MethodDelete, middleware.RequireAuth(notifHandler.Delete)))

	mux.HandleFunc("/api/push/subscribe", method(http.MethodPost, middleware.RequireAuth(pushHandler.Subscribe)))
	mux.HandleFunc("/api/push/unsubscribe", method(http.MethodPost, middleware.RequireAuth(pushHandler.Unsubscribe)))

	mux.HandleFunc("/api/stats/leaderboard", method(http.MethodGet, middleware.RequireAuth(statsHandler.Leaderboard)))
	mux.HandleFunc("/api/stats/streaks", method(http.MethodGet, middleware.RequireAuth(statsHandler.Streaks)))
	mux.HandleFunc("/api/stats/heatmap", method(http.MethodGet, middleware.RequireAuth(statsHandler.Heatmap)))
	mux.HandleFunc("/api/stats/breakdown", method(http.MethodGet, middleware.RequireAuth(statsHandler.Breakdown)))
	mux.HandleFunc("/api/stats/recap", method(http.MethodGet, middleware.RequireAuth(statsHandler.Recap)))
	mux.HandleFunc("/api/stats/overview", method(http.MethodGet, middleware.RequireAuth(statsHandler.Overview)))
	mux.HandleFunc("/api/stats/busy-hours", method(http.MethodGet, middleware.RequireAuth(statsHandler.BusyHours)))
	mux.HandleFunc("/api/stats/chores", method(http.MethodGet, middleware.RequireAuth(statsHandler.ChoreStats)))
	mux.HandleFunc("/api/stats/chores/{id}", method(http.MethodGet, middleware.RequireAuth(statsHandler.ChoreStatsByID)))

	mux.HandleFunc("/api/preferences", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			preferencesHandler.Get(w, r)
		case http.MethodPatch:
			preferencesHandler.Update(w, r)
		default:
			w.Header().Set("Allow", "GET, PATCH")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/schedules", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			scheduleHandler.List(w, r)
		case http.MethodPost:
			scheduleHandler.Create(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/schedules/for-date", method(http.MethodGet, middleware.RequireAuth(scheduleHandler.ForDate)))
	mux.HandleFunc("/api/schedules/{id}", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			scheduleHandler.Update(w, r)
		case http.MethodDelete:
			scheduleHandler.Delete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(err)
	}

	// Pre-process every JS file: inject ?v=VERSION into all relative .js import
	// paths so that a new deploy busts both the Cloudflare CDN cache and browser
	// caches for every module, not just app.js itself.
	versionedJS := buildVersionedJSCache(staticFS, version.Version)

	// Pre-process the service worker: inject the version into CACHE_NAME so
	// the browser detects a new service worker file on every deploy and shows
	// the "App updated" toast without the user needing to close/reopen the PWA.
	versionedSW := buildVersionedSW(staticFS, version.Version)

	staticFileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip query string to get the bare file path.
		p := strings.SplitN(r.URL.Path, "?", 2)[0]
		if strings.HasSuffix(p, ".js") {
			if content, ok := versionedJS[p]; ok {
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				// no-store prevents Cloudflare and browsers from caching;
				// versioned import paths mean each deploy gets fresh URLs anyway.
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusOK)
				w.Write(content) //nolint:errcheck
				return
			}
		}
		if strings.HasSuffix(p, ".css") {
			w.Header().Set("Cache-Control", "no-store")
		}
		staticFileServer.ServeHTTP(w, r)
	})))
	mux.HandleFunc("/service-worker.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		w.Write(versionedSW) //nolint:errcheck
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		renderIndex(w, cfg)
	})

	var handler http.Handler = mux
	handler = middleware.RequestLogger(nil)(handler)
	handler = middleware.SecurityHeaders()(handler)
	handler = middleware.Session(authService, "choresy_session")(handler)
	handler = middleware.CSRF("choresy_csrf")(handler)
	handler = rateLimiter.Middleware("/api/auth")(handler)

	return &Server{handler: handler}
}

func BuildServer(ctx context.Context, cfg config.Config) (http.Handler, io.Closer, error) {
	if cfg.DatabaseURL == "" {
		_ = audit.NewStdLogger(log.Default())
		return NewServer(cfg), io.NopCloser(strings.NewReader("")), nil
	}

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	if err := database.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	return NewServerWithDB(cfg, db), db, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

var indexTmpl = template.Must(template.ParseFS(webassets.Assets, "templates/index.html"))

func renderIndex(w http.ResponseWriter, cfg config.Config) {
	googleOAuthEnabled := cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""
	data := struct {
		AppName            string
		Version            string
		VAPIDPublicKey     string
		GoogleOAuthEnabled bool
	}{
		AppName:            "Choresy",
		Version:            version.Version,
		VAPIDPublicKey:     cfg.VAPIDPublicKey,
		GoogleOAuthEnabled: googleOAuthEnabled,
	}
	if err := indexTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func method(want string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != want {
			w.Header().Set("Allow", want)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func newMailer(cfg config.Config) mail.Sender {
	if cfg.SMTPHost != "" {
		return mail.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
	}
	return mail.UnavailableSender{}
}

func newOIDCProvider(cfg config.Config) auth.OIDCProvider {
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return nil
	}
	return &auth.GoogleOIDCProvider{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  strings.TrimRight(cfg.AppBaseURL, "/") + "/api/auth/google/callback",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Issuer:       "https://accounts.google.com",
	}
}

type choreStatsAdapter struct {
	store chore.Store
}

func (a *choreStatsAdapter) GetChore(ctx context.Context, id int64) (stats.ChoreInfo, error) {
	c, err := a.store.GetChore(ctx, id)
	if err != nil {
		return stats.ChoreInfo{}, err
	}
	return stats.ChoreInfo{ID: c.ID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category, HasVolumeML: c.HasVolumeML, IndicatorLabels: c.IndicatorLabels}, nil
}

func (a *choreStatsAdapter) ListChores(ctx context.Context, householdID int64) ([]stats.ChoreInfo, error) {
	chores, err := a.store.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	result := make([]stats.ChoreInfo, len(chores))
	for i, c := range chores {
		result[i] = stats.ChoreInfo{ID: c.ID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category, HasVolumeML: c.HasVolumeML, IndicatorLabels: c.IndicatorLabels}
	}
	return result, nil
}

// buildVersionedJSCache walks all .js files under the given FS, rewrites every
// relative ES-module import path by appending ?v=<ver>, and returns a map of
// bare file path (e.g. "js/app.js") → modified content.  This ensures that
// every deploy produces new import URLs for all modules, busting both the
// Cloudflare CDN cache and browser caches without requiring any manual
// cache-purge or per-file query-string management.
var relativeJSImport = regexp.MustCompile(`(from\s+["'])(\.\/[^"'?#\s]+\.js)(["'])`)

func buildVersionedJSCache(fsys fs.FS, ver string) map[string][]byte {
	cache := make(map[string][]byte)
	replacement := []byte("${1}${2}?v=" + ver + "${3}")
	_ = fs.WalkDir(fsys, "js", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil
		}
		modified := relativeJSImport.ReplaceAll(raw, replacement)
		// Map key is the URL path segment after /static/ — strip the "js/" prefix
		// so "js/app.js" → "js/app.js" (matches r.URL.Path after StripPrefix).
		cache[path] = modified
		if !bytes.Equal(raw, modified) {
			log.Printf("versioned JS imports in %s (%d replacements)", path, bytes.Count(modified, []byte("?v="+ver)))
		}
		return nil
	})
	return cache
}

var swCacheNameRE = regexp.MustCompile(`("choresy-static-)v\d+(")`)

func buildVersionedSW(fsys fs.FS, ver string) []byte {
	raw, err := fs.ReadFile(fsys, "service-worker.js")
	if err != nil {
		panic("service-worker.js not found in embedded FS: " + err.Error())
	}
	replacement := []byte("${1}" + ver + "${2}")
	modified := swCacheNameRE.ReplaceAll(raw, replacement)
	log.Printf("versioned service worker cache name to %q", ver)
	return modified
}
