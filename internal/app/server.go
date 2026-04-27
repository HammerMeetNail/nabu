package app

import (
	"context"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dave/choresy/internal/audit"
	"github.com/dave/choresy/internal/config"
	"github.com/dave/choresy/internal/database"
	"github.com/dave/choresy/internal/handlers"
	"github.com/dave/choresy/internal/middleware"
	webassets "github.com/dave/choresy/web"
)

type Server struct {
	handler http.Handler
}

func NewServer(cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	rateLimiter := middleware.NewRateLimiter(20, time.Minute)

	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/ready", handlers.Ready)

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
	handler = middleware.Session(nil, "choresy_session")(handler)
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

	return NewServer(cfg), db, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func renderIndex(w http.ResponseWriter, cfg config.Config) {
	tmpl := template.Must(template.ParseFS(webassets.Assets, "templates/index.html"))
	data := struct {
		AppName string
	}{
		AppName: "Choresy",
	}
	if err := tmpl.Execute(w, data); err != nil {
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
