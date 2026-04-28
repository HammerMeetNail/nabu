package app

import (
	"context"
	"database/sql"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
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
	"github.com/dave/choresy/internal/stats"
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

	if db != nil {
		authStore = auth.NewPostgresStore(db)
		householdStore = household.NewPostgresStore(db)
		choreStore = chore.NewPostgresStore(db)
		logStore = logsvc.NewPostgresStore(db)
	} else {
		authStore = auth.NewMemoryStore()
		householdStore = household.NewMemoryStore()
		choreStore = chore.NewMemoryStore()
		logStore = logsvc.NewMemoryStore()
	}

	authService := auth.NewService(authStore)
	authService.SetAuditLogger(audit.NewStdLogger(log.Default()))
	authService.SetMailer(newMailer(cfg), cfg.AppBaseURL)
	authHandler := handlers.NewAuthHandler(authService, "choresy_session", cfg.ServerSecure)
	householdService := household.NewService(householdStore)
	householdHandler := handlers.NewHouseholdHandler(householdService, authService)
	choreService := chore.NewService(choreStore)
	choreHandler := handlers.NewChoreHandler(choreService)
	logService := logsvc.NewService(logStore)
	logHandler := handlers.NewLogHandler(logService)
	statsService := stats.NewService(logStore, &choreStatsAdapter{choreStore})
	statsHandler := handlers.NewStatsHandler(statsService)
	rateLimiter := middleware.NewRateLimiter(20, time.Minute)
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
	mux.HandleFunc("/api/logs/{id}", method(http.MethodDelete, middleware.RequireAuth(logHandler.Delete)))
	mux.HandleFunc("/api/logs/today", method(http.MethodGet, middleware.RequireAuth(logHandler.Today)))
	mux.HandleFunc("/api/logs/week", method(http.MethodGet, middleware.RequireAuth(logHandler.Week)))
	mux.HandleFunc("/api/logs/month", method(http.MethodGet, middleware.RequireAuth(logHandler.Month)))

	mux.HandleFunc("/api/stats/leaderboard", method(http.MethodGet, middleware.RequireAuth(statsHandler.Leaderboard)))
	mux.HandleFunc("/api/stats/streaks", method(http.MethodGet, middleware.RequireAuth(statsHandler.Streaks)))
	mux.HandleFunc("/api/stats/heatmap", method(http.MethodGet, middleware.RequireAuth(statsHandler.Heatmap)))
	mux.HandleFunc("/api/stats/breakdown", method(http.MethodGet, middleware.RequireAuth(statsHandler.Breakdown)))
	mux.HandleFunc("/api/stats/recap", method(http.MethodGet, middleware.RequireAuth(statsHandler.Recap)))

	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/service-worker.js", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/service-worker.js"
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
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
	data := struct {
		AppName string
	}{
		AppName: "Choresy",
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
	return stats.ChoreInfo{ID: c.ID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category}, nil
}

func (a *choreStatsAdapter) ListChores(ctx context.Context, householdID int64) ([]stats.ChoreInfo, error) {
	chores, err := a.store.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	result := make([]stats.ChoreInfo, len(chores))
	for i, c := range chores {
		result[i] = stats.ChoreInfo{ID: c.ID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category}
	}
	return result, nil
}
