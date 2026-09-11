package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HammerMeetNail/nabu/internal/account"
	"github.com/HammerMeetNail/nabu/internal/apns"
	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/background"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/config"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/daynote"
	"github.com/HammerMeetNail/nabu/internal/handlers"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/stats"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
	"github.com/HammerMeetNail/nabu/internal/version"
	webassets "github.com/HammerMeetNail/nabu/web"
)

type Server struct {
	handler    http.Handler
	cancel     context.CancelFunc
	background *sync.WaitGroup
	limiters   []*middleware.RateLimiter
	deliveryDB *sql.DB
}

// Close releases background resources started by the server: it cancels the
// reminder scheduler goroutine and stops each rate-limiter's cleanup goroutine.
// It is safe to call once, after the HTTP server has stopped accepting requests.
func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.background != nil {
		s.background.Wait()
	}
	for _, l := range s.limiters {
		l.Stop()
	}
	if s.deliveryDB != nil {
		return s.deliveryDB.Close()
	}
	return nil
}

func NewServer(cfg config.Config) http.Handler {
	return NewServerWithDB(cfg, nil)
}

func NewServerWithDB(cfg config.Config, db *sql.DB) http.Handler {
	srv, err := newServerWithDB(cfg, db, nil)
	if err != nil {
		panic("could not initialize database delivery pool")
	}
	return srv
}

func newServerWithDB(cfg config.Config, db *sql.DB, queryMetrics *database.QueryMetrics) (http.Handler, error) {
	if err := cfg.Validate(); err != nil {
		panic(err) // construction error, before any workers are started
	}
	if cfg.IsProduction() && db == nil {
		panic("production requires an open database")
	}
	var deliveryDB *sql.DB
	var deliveryMetrics *database.QueryMetrics
	if db != nil {
		initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		deliveryDB, deliveryMetrics, err = database.ForkDeliveryPool(initCtx, db)
		initCancel()
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(cfg.DBMaxOpenConns - database.DeliveryConnections)
		db.SetMaxIdleConns(min(cfg.DBMaxIdleConns, cfg.DBMaxOpenConns-database.DeliveryConnections))
	}
	// Cancelable context so the reminder scheduler goroutine stops on shutdown
	// (Server.Close calls cancel).
	ctx, cancel := context.WithCancel(context.Background())
	mux := http.NewServeMux()

	var authStore auth.Store
	var householdStore household.Store
	var choreStore chore.Store
	var logStore logsvc.Store
	var userPrefsStore userprefs.Store
	var notifStore notification.Store
	var pushStore push.Store
	var dayNoteStore daynote.Store
	var apnsStore apns.Store

	if db != nil {
		authStore = auth.NewPostgresStore(db)
		householdStore = household.NewPostgresStore(db)
		choreStore = chore.NewPostgresStore(db)
		logStore = logsvc.NewPostgresStore(db)
		userPrefsStore = userprefs.NewPostgresStore(db)
		notifStore = notification.NewPostgresStore(db)
		pushStore = push.NewPostgresStore(db)
		dayNoteStore = daynote.NewPostgresStore(db)
		apnsStore = apns.NewPostgresStore(db)
	} else {
		authStore = auth.NewMemoryStore()
		householdStore = household.NewMemoryStore()
		choreStore = chore.NewMemoryStore()
		logStore = logsvc.NewMemoryStore()
		userPrefsStore = userprefs.NewMemoryStore()
		notifStore = notification.NewMemoryStore()
		pushStore = push.NewMemoryStore()
		dayNoteStore = daynote.NewMemoryStore()
		apnsStore = apns.NewMemoryStore()
		pushStore.(*push.MemoryStore).BindSessions(authStore.(*auth.MemoryStore))
		apnsStore.(*apns.MemoryStore).BindSessions(authStore.(*auth.MemoryStore))
	}

	authService := auth.NewService(authStore)
	auditLog := audit.NewStdLogger(log.Default())
	authService.SetAuditLogger(auditLog)
	authService.SetMailer(newMailer(cfg), cfg.AppBaseURL)
	authService.SetOIDCProvider(newOIDCProvider(cfg))
	appleClientIDs := splitCommaList(cfg.AppleClientIDs)
	if cfg.AppleWebClientID != "" && !slices.Contains(appleClientIDs, cfg.AppleWebClientID) {
		appleClientIDs = append(appleClientIDs, cfg.AppleWebClientID)
	}
	if len(appleClientIDs) > 0 {
		authService.SetAppleVerifier(auth.NewAppleVerifier(appleClientIDs))
	}
	if cfg.AppleWebClientID != "" {
		authService.SetAppleWebAuth(&auth.AppleWebAuth{
			ClientID:    cfg.AppleWebClientID,
			RedirectURL: strings.TrimRight(cfg.AppBaseURL, "/") + "/api/auth/apple/web/callback",
		})
	}
	authHandler := handlers.NewAuthHandler(authService, "nabu_session", cfg.ServerSecure, cfg.AppBaseURL)
	accountService := account.NewService(authStore, householdStore)
	accountHandler := handlers.NewAccountHandler(accountService, authHandler)
	householdService := household.NewService(householdStore, authService)
	householdService.WithHistoricalMembers(logStore, authService)
	householdService.SetAuditLogger(auditLog)
	householdHandler := handlers.NewHouseholdHandler(householdService)
	choreService := chore.NewService(choreStore)
	choreService.SetAuditLogger(auditLog)
	choreService.WithMemberships(householdStore)
	choreHandler := handlers.NewChoreHandler(choreService).WithHouseholdStore(householdStore)
	logService := logsvc.NewService(logStore)
	logService.SetAuditLogger(auditLog)
	logHandler := handlers.NewLogHandler(logService).WithChoreStore(choreStore, householdStore)
	notifService := notification.NewService(notifStore).WithAuthorization(householdStore, choreStore)
	notifHandler := handlers.NewNotificationHandler(notifService)
	logHandler.WithNotification(notifService, choreStore, householdStore)
	householdHandler.WithNotification(notifService, householdStore)

	var scheduleStore schedule.Store
	if db != nil {
		scheduleStore = schedule.NewPostgresStore(db)
	} else {
		scheduleStore = schedule.NewMemoryStore()
	}
	// Need to re-wire choreHandler after scheduleStore is created (earlier ref was nil)
	choreHandler.WithScheduleStore(scheduleStore).WithHouseholdStore(householdStore)
	scheduleService := schedule.NewService()
	scheduleHandler := handlers.NewScheduleHandler(scheduleStore, scheduleService)
	scheduleHandler.WithChoreStore(choreStore).WithHouseholdStore(householdStore)
	scheduleHandler.SetAuditLogger(auditLog)
	logHandler.WithScheduleStore(scheduleStore)

	var reminderStore reminder.Store
	if db != nil {
		reminderStore = reminder.NewPostgresStore(db)
	} else {
		reminderStore = reminder.NewMemoryStore()
	}

	if db == nil {
		accountService.SetMemoryCleanup(func(userID int64, householdIDs []int64) {
			d := lifecycle.NewDeletion(userID, householdIDs)
			// Collect dependent IDs under each domain mutex, then clean dependents.
			for _, store := range []any{choreStore, logStore, scheduleStore, reminderStore, notifStore, pushStore, apnsStore, userPrefsStore, dayNoteStore} {
				store.(interface{ CleanupAccount(*lifecycle.Deletion) }).CleanupAccount(d)
			}
		})
		householdStore.(*household.MemoryStore).SetMembershipCleanup(func(userID, householdID int64, member bool) {
			chores, _ := choreStore.ListChores(context.Background(), householdID)
			ids := make([]int64, 0, len(chores))
			for _, ch := range chores {
				ids = append(ids, ch.ID)
			}
			scheduleStore.(*schedule.MemoryStore).MembershipChanged(userID, householdID, member)
			reminderStore.(*reminder.MemoryStore).MembershipChanged(userID, ids, member)
		})
	}

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
	pushHandler := handlers.NewPushHandler(pushStore).WithChores(choreStore)
	pushHandler.SetAuditLogger(auditLog)

	// APNs sender for the native iOS app; a graceful no-op unless all four
	// APNS_* settings are configured (mirrors the VAPID signer's behavior).
	apnsClient := newAPNsClient(cfg, apnsStore)
	apnsHandler := handlers.NewAPNsHandler(apnsStore)
	apnsHandler.SetAuditLogger(auditLog)

	// Every push fans out to all configured channels: Web Push for the PWA,
	// APNs for the native app.
	var pushChannels []push.DataSender
	if vapidSigner != nil {
		pushChannels = append(pushChannels, pushService)
	}
	if apnsClient != nil {
		pushChannels = append(pushChannels, apnsClient)
	}
	pushFanout := push.NewFanoutSender(pushChannels...)
	if len(pushChannels) > 0 {
		notifService.WithPushSender(pushFanout)
	}

	// Background tasks use their reserved pool for every nested store and
	// provider lookup. HTTP push-test/registration requests retain the HTTP pool.
	deliveryHouseholds, deliveryChores, deliveryNotifs := householdStore, choreStore, notifStore
	deliveryReminders, deliverySchedules, deliveryPrefs := reminderStore, scheduleStore, userPrefsStore
	deliveryPush := pushFanout
	if deliveryDB != nil {
		deliveryHouseholds = household.NewPostgresStore(deliveryDB)
		deliveryChores = chore.NewPostgresStore(deliveryDB)
		deliveryNotifs = notification.NewPostgresStore(deliveryDB)
		deliveryReminders = reminder.NewPostgresStore(deliveryDB)
		deliverySchedules = schedule.NewPostgresStore(deliveryDB)
		deliveryPrefs = userprefs.NewPostgresStore(deliveryDB)
		var channels []push.DataSender
		if vapidSigner != nil {
			channels = append(channels, push.NewService(push.NewPostgresStore(deliveryDB), vapidSigner))
		}
		if client := newAPNsClient(cfg, apns.NewPostgresStore(deliveryDB)); client != nil {
			channels = append(channels, client)
		}
		deliveryPush = push.NewFanoutSender(channels...)
	}
	deliveryNotifications := notification.NewService(deliveryNotifs).WithAuthorization(deliveryHouseholds, deliveryChores)
	if len(pushChannels) > 0 {
		deliveryNotifications.WithPushSender(deliveryPush)
	}
	logHandler.WithNotification(deliveryNotifications, deliveryChores, deliveryHouseholds)
	householdHandler.WithNotification(deliveryNotifications, deliveryHouseholds)
	reminderSched := reminder.NewScheduler(deliveryReminders, deliverySchedules, scheduleService,
		deliveryNotifs, deliveryChores, deliveryHouseholds, deliveryPrefs, deliveryPush)
	// Guard the scheduler with a Postgres advisory lock so that running multiple
	// app instances does not emit duplicate reminders (only the leader ticks).
	// In-memory mode (db == nil) is single-instance, so no lock is needed.
	if db != nil {
		reminderSched.SetLeaderLock(reminder.NewPostgresAdvisoryLock(deliveryDB, reminder.LeaderLockKey))
		reminderSched.SetQueryCounter(func() uint64 { return deliveryMetrics.Snapshot().Count })
	}
	var backgroundWG sync.WaitGroup
	if db != nil {
		backgroundWG.Add(1)
		go func() { defer backgroundWG.Done(); queryMetrics.MonitorPool(ctx, db, nil) }()
		backgroundWG.Add(1)
		go func() { defer backgroundWG.Done(); deliveryMetrics.MonitorPoolNamed(ctx, deliveryDB, nil, "delivery") }()
	}
	notificationWork := background.New(ctx, &backgroundWG, 2, 64)
	logHandler.SetBackground(ctx, notificationWork.Submit)
	householdHandler.SetBackground(ctx, notificationWork.Submit)
	backgroundWG.Add(2)
	go func() { defer backgroundWG.Done(); reminderSched.Start(ctx) }()
	go func() { defer backgroundWG.Done(); authService.RunMailOutbox(ctx) }()

	reminderHandler := handlers.NewChoreReminderPrefsHandler(reminderStore).WithChoreStore(choreStore).WithHouseholdStore(householdStore)
	userPrefsService := userprefs.NewService(userPrefsStore)
	preferencesHandler := handlers.NewPreferencesHandler(userPrefsService).WithChoreStore(choreStore).WithHouseholdStore(householdStore)
	dayNoteService := daynote.NewService(dayNoteStore)
	dayNoteHandler := handlers.NewDayNoteHandler(dayNoteService)
	reminderSnoozeHandler := handlers.NewReminderSnoozeHandler(scheduleStore, choreStore, userPrefsStore).WithHouseholdStore(householdStore)
	statsService := stats.NewService(logStore, &choreStatsAdapter{choreStore}).WithMemberships(householdStore)
	statsHandler := handlers.NewStatsHandler(statsService, userPrefsStore)
	exportHandler := handlers.NewExportHandler(householdService, householdStore, choreStore, logService, scheduleStore, dayNoteService)

	hasTrustedProxy := strings.TrimSpace(cfg.TrustedProxyCIDRs) != ""

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitAuthMax, time.Minute)
	rateLimiter.SetMaxClients(cfg.RateLimitMaxClients)
	if err := rateLimiter.SetTrustedProxies(cfg.TrustedProxyCIDRs); err != nil {
		panic("invalid trusted proxy configuration")
	}

	// Aggregate per-IP backstop covering all /api/ routes.
	// It is only constructed/wired when TRUSTED_PROXY_CIDRS is configured: a
	// per-IP global limiter is only safe when the deployment can attribute a
	// real client IP, which behind a reverse proxy/tunnel requires trusting it.
	// The strict auth limiter still applies to /api/auth regardless.
	var globalRateLimiter *middleware.RateLimiter
	if hasTrustedProxy {
		globalRateLimiter = middleware.NewRateLimiter(cfg.RateLimitGlobalMax, time.Minute)
		globalRateLimiter.SetMaxClients(cfg.RateLimitMaxClients)
		if err := globalRateLimiter.SetTrustedProxies(cfg.TrustedProxyCIDRs); err != nil {
			panic("invalid trusted proxy configuration")
		}
	}

	// Tighter per-IP limiter on household joins: invite codes are self-serve
	// (free account + CSRF pair), so without a dedicated cap an attacker can
	// hammer /api/household/join against many codes. 10/min per IP keeps
	// legitimate multi-code joins (several family members, one network)
	// working while stopping brute-force sweeps.
	joinLimiter := middleware.NewRateLimiter(cfg.RateLimitJoinMax, time.Minute)
	joinLimiter.SetMaxClients(cfg.RateLimitMaxClients)
	if err := joinLimiter.SetTrustedProxies(cfg.TrustedProxyCIDRs); err != nil {
		panic("invalid trusted proxy configuration")
	}

	mux.HandleFunc("/health", handlers.Health)
	var probe func(context.Context) error
	if db != nil {
		probe = db.PingContext
	}
	mux.HandleFunc("/ready", handlers.Readiness(probe))

	mux.HandleFunc("/api/auth/register", method(http.MethodPost, authHandler.Register))
	mux.HandleFunc("/api/auth/login", method(http.MethodPost, authHandler.Login))
	mux.HandleFunc("/api/auth/logout", method(http.MethodPost, authHandler.Logout))
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.Me(w, r)
		case http.MethodDelete:
			middleware.RequireAuth(accountHandler.DeleteMe)(w, r)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/auth/email/verification/resend", method(http.MethodPost, authHandler.ResendVerification))
	mux.HandleFunc("/api/auth/email/verify", method(http.MethodGet, authHandler.VerifyEmail))
	mux.HandleFunc("/api/auth/magic-link/request", method(http.MethodPost, authHandler.RequestMagicLink))
	mux.HandleFunc("/api/auth/magic-link/consume", method(http.MethodGet, authHandler.ConsumeMagicLink))
	mux.HandleFunc("/api/auth/password/forgot", method(http.MethodPost, authHandler.ForgotPassword))
	mux.HandleFunc("/api/auth/password/reset", method(http.MethodPost, authHandler.ResetPassword))
	mux.HandleFunc("/api/auth/password", method(http.MethodPost, middleware.RequireAuth(authHandler.ChangePassword)))
	mux.HandleFunc("/api/auth/google/login", method(http.MethodGet, authHandler.GoogleLogin))
	mux.HandleFunc("/api/auth/google/callback", method(http.MethodGet, authHandler.GoogleCallback))
	mux.HandleFunc("/api/auth/apple/native", method(http.MethodPost, authHandler.AppleNative))
	mux.HandleFunc("/api/auth/apple/web/login", method(http.MethodGet, authHandler.AppleWebLogin))
	mux.HandleFunc("/api/auth/apple/web/callback", method(http.MethodPost, authHandler.AppleWebCallback))

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
	exportGate := handlers.NewExportGate()
	mux.HandleFunc("/api/household/data", method(http.MethodGet, middleware.RequireAuth(exportGate.Wrap(exportHandler.Data))))
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

	// Multi-household endpoints
	mux.HandleFunc("/api/households", method(http.MethodGet, middleware.RequireAuth(householdHandler.ListAll)))
	mux.HandleFunc("/api/households/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/activate") && r.Method == http.MethodPost {
			middleware.RequireAuth(householdHandler.Activate)(w, r)
			return
		}
		http.NotFound(w, r)
	})

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
	mux.HandleFunc("/api/logs/export", method(http.MethodGet, middleware.RequireAuth(exportGate.Wrap(logHandler.Export))))
	mux.HandleFunc("/api/logs/latest-per-chore", method(http.MethodGet, middleware.RequireAuth(logHandler.LatestPerChore)))
	mux.HandleFunc("/api/logs/recent-amounts", method(http.MethodGet, middleware.RequireAuth(logHandler.RecentAmounts)))

	mux.HandleFunc("/api/notifications", method(http.MethodGet, middleware.RequireAuth(notifHandler.List)))
	mux.HandleFunc("/api/notifications/read-all", method(http.MethodPost, middleware.RequireAuth(notifHandler.MarkAllRead)))
	mux.HandleFunc("/api/notifications/{id}/read", method(http.MethodPost, middleware.RequireAuth(notifHandler.MarkRead)))
	mux.HandleFunc("/api/notifications/{id}", method(http.MethodDelete, middleware.RequireAuth(notifHandler.Delete)))

	notifPrefsHandler := handlers.NewNotificationPreferencesHandler(notifService)
	mux.HandleFunc("/api/notification-preferences", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			notifPrefsHandler.Get(w, r)
		case http.MethodPatch:
			notifPrefsHandler.Update(w, r)
		default:
			w.Header().Set("Allow", "GET, PATCH")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/chore-reminder-prefs", method(http.MethodGet, middleware.RequireAuth(reminderHandler.List)))
	mux.HandleFunc("/api/chore-reminder-prefs/{choreId}", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			reminderHandler.Update(w, r)
		default:
			w.Header().Set("Allow", "PATCH")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/mobile/apns/register", method(http.MethodPost, middleware.RequireAuth(apnsHandler.Register)))
	mux.HandleFunc("/api/mobile/apns/unregister", method(http.MethodPost, middleware.RequireAuth(apnsHandler.Unregister)))

	mux.HandleFunc("/api/push/subscribe", method(http.MethodPost, middleware.RequireAuth(pushHandler.Subscribe)))
	mux.HandleFunc("/api/push/identity", method(http.MethodGet, middleware.RequireAuth(pushHandler.Identity)))
	mux.HandleFunc("/api/push/unsubscribe", method(http.MethodPost, middleware.RequireAuth(pushHandler.Unsubscribe)))

	mux.HandleFunc("/api/stats/leaderboard", method(http.MethodGet, middleware.RequireAuth(statsHandler.Leaderboard)))
	mux.HandleFunc("/api/stats/streaks", method(http.MethodGet, middleware.RequireAuth(statsHandler.Streaks)))
	mux.HandleFunc("/api/stats/heatmap", method(http.MethodGet, middleware.RequireAuth(statsHandler.Heatmap)))
	mux.HandleFunc("/api/stats/breakdown", method(http.MethodGet, middleware.RequireAuth(statsHandler.Breakdown)))
	mux.HandleFunc("/api/stats/recap", method(http.MethodGet, middleware.RequireAuth(statsHandler.Recap)))
	mux.HandleFunc("/api/stats/overview", method(http.MethodGet, middleware.RequireAuth(statsHandler.Overview)))
	mux.HandleFunc("/api/stats/busy-hours", method(http.MethodGet, middleware.RequireAuth(statsHandler.BusyHours)))
	mux.HandleFunc("/api/stats/top-chores", method(http.MethodGet, middleware.RequireAuth(statsHandler.TopChores)))
	mux.HandleFunc("/api/stats/chores", method(http.MethodGet, middleware.RequireAuth(statsHandler.ChoreStats)))
	mux.HandleFunc("/api/stats/chores/{id}", method(http.MethodGet, middleware.RequireAuth(statsHandler.ChoreStatsByID)))
	mux.HandleFunc("/api/stats/chores/{id}/time-series", method(http.MethodGet, middleware.RequireAuth(statsHandler.ChoreTimeSeries)))
	mux.HandleFunc("/api/stats/chores/{id}/summary", method(http.MethodGet, middleware.RequireAuth(statsHandler.ChoreSummary)))
	mux.HandleFunc("/api/stats/feeding-gaps", method(http.MethodGet, middleware.RequireAuth(statsHandler.FeedingGaps)))

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

	mux.HandleFunc("/api/day-notes", method(http.MethodGet, middleware.RequireAuth(dayNoteHandler.List)))
	mux.HandleFunc("/api/day-notes/{date}", method(http.MethodPut, middleware.RequireAuth(dayNoteHandler.Set)))

	mux.HandleFunc("/api/reminders/snooze", method(http.MethodPost, middleware.RequireAuth(reminderSnoozeHandler.Snooze)))

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

	// Universal links (iOS): Apple's CDN fetches this file to associate
	// https links on this host with the app, letting verification, magic-link,
	// and invite emails open natively. Served only when the Apple team/bundle
	// IDs are configured (the same env vars the APNs sender uses).
	if aasa := appleAppSiteAssociation(cfg); aasa != nil {
		mux.HandleFunc("/.well-known/apple-app-site-association", method(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(aasa) //nolint:errcheck
		}))
	}

	// Privacy policy + support pages (App Store metadata requirement).
	registerStaticPages(mux)

	// Public marketing homepage — no login required. Served ahead of the
	// SPA catch-all so it renders standalone HTML instead of the app shell.
	mux.HandleFunc("/home", method(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		renderHome(w, cfg)
	}))

	// Root route + SPA catch-all. Authenticated users always get the app
	// shell so in-app navigations and deeplinks (including "/") keep working.
	// Anonymous visitors get the marketing homepage at exactly "/" (served
	// from home.html: SEO-visible, server-rendered, no JS required) so
	// crawlers and share links land on real HTML instead of an empty shell.
	// Unknown app routes still render the SPA for anonymous users — its
	// client-side router shows the login view there.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, ok := middleware.CurrentUser(r.Context()); ok {
			renderIndex(w, cfg)
			return
		}
		if r.URL.Path == "/" {
			renderHome(w, cfg)
			return
		}
		renderIndex(w, cfg)
	})

	var handler http.Handler = mux
	handler = middleware.RequestLogger(nil)(handler)
	handler = middleware.SecurityHeaders(cfg.ServerSecure)(handler)
	handler = middleware.Session(authService, "nabu_session")(handler)
	handler = middleware.CSRF("nabu_csrf", cfg.ServerSecure)(handler)
	handler = rateLimiter.Middleware("/api/auth")(handler)
	handler = joinLimiter.Middleware("/api/household/join")(handler)
	// Engage the global per-IP backstop only when client IPs are reliably
	// attributable (trusted proxy configured); otherwise it would key every
	// request to the proxy's single IP and could 429 the whole user base.
	if globalRateLimiter != nil {
		handler = globalRateLimiter.Middleware("/api/")(handler)
	}

	limiters := []*middleware.RateLimiter{rateLimiter, joinLimiter}
	if globalRateLimiter != nil {
		limiters = append(limiters, globalRateLimiter)
	}

	return &Server{handler: handler, cancel: cancel, limiters: limiters, background: &backgroundWG, deliveryDB: deliveryDB}, nil
}

func BuildServer(ctx context.Context, cfg config.Config) (http.Handler, io.Closer, error) {
	return buildServer(ctx, cfg, database.OpenMeasured)
}

type databaseOpener func(string, database.PoolOptions) (*sql.DB, *database.QueryMetrics, error)

func buildServer(ctx context.Context, cfg config.Config, open databaseOpener) (http.Handler, io.Closer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if cfg.DatabaseURL == "" {
		srv := NewServer(cfg)
		// The *Server implements io.Closer (stops the scheduler + rate-limiter
		// goroutines); return it so callers tear those down on shutdown.
		return srv, srv.(io.Closer), nil
	}

	httpConnections := cfg.DBMaxOpenConns - database.DeliveryConnections
	db, queryMetrics, err := open(cfg.DatabaseURL, database.PoolOptions{MaxOpen: httpConnections, MaxIdle: min(cfg.DBMaxIdleConns, httpConnections)})
	if err != nil {
		return nil, nil, err
	}
	if err := database.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	srv, err := newServerWithDB(cfg, db, queryMetrics)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return srv, multiCloser{srv.(io.Closer), db}, nil
}

// multiCloser closes several io.Closers in order, returning the first error.
type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var firstErr error
	for _, c := range m {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

var indexTmpl = template.Must(template.ParseFS(webassets.Assets, "templates/index.html"))

func renderIndex(w http.ResponseWriter, cfg config.Config) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	googleOAuthEnabled := cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""
	data := struct {
		AppName            string
		Version            string
		VAPIDPublicKey     string
		GoogleOAuthEnabled bool
		AppleSignInEnabled bool
	}{
		AppName:            "Nabu",
		Version:            version.Version,
		VAPIDPublicKey:     cfg.VAPIDPublicKey,
		GoogleOAuthEnabled: googleOAuthEnabled,
		AppleSignInEnabled: cfg.AppleWebClientID != "",
	}
	if err := indexTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var homeTmpl = template.Must(template.ParseFS(webassets.Assets, "templates/home.html"))

func renderHome(w http.ResponseWriter, cfg config.Config) {
	data := struct {
		BaseURL string
		Year    int
	}{
		BaseURL: strings.TrimRight(cfg.AppBaseURL, "/"),
		Year:    time.Now().Year(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := homeTmpl.Execute(w, data); err != nil {
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

// newAPNsClient builds the APNs sender when all APNS_* settings are present,
// or returns nil (a disabled channel) when they are not. The .p8 key may be
// provided as literal PEM, PEM with escaped newlines (env-var style), or
// base64 of the PEM.
func newAPNsClient(cfg config.Config, store apns.Store) *apns.Client {
	if cfg.APNSAuthKeyP8 == "" || cfg.APNSKeyID == "" || cfg.APNSTeamID == "" || cfg.APNSBundleID == "" {
		return nil
	}
	keyPEM := strings.ReplaceAll(cfg.APNSAuthKeyP8, `\n`, "\n")
	if !strings.Contains(keyPEM, "-----BEGIN") {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyPEM)); err == nil {
			keyPEM = string(decoded)
		}
	}
	signer, err := apns.NewProviderTokenSigner(keyPEM, cfg.APNSKeyID, cfg.APNSTeamID)
	if err != nil {
		log.Printf("warning: APNs disabled: %v", err)
		return nil
	}
	return apns.NewClient(store, signer, cfg.APNSBundleID)
}

// appleAppSiteAssociation renders the AASA JSON for universal links, or nil
// when the Apple team/bundle IDs are unset. The paths listed here must stay in
// sync with the deep links the iOS app handles (DeepLink.swift): email
// verification, magic-link sign-in, and household invites.
func appleAppSiteAssociation(cfg config.Config) []byte {
	if cfg.APNSTeamID == "" || cfg.APNSBundleID == "" {
		return nil
	}
	appID := cfg.APNSTeamID + "." + cfg.APNSBundleID
	aasa, err := json.Marshal(map[string]any{
		"applinks": map[string]any{
			"details": []map[string]any{{
				"appIDs": []string{appID},
				"components": []map[string]string{
					{"/": "/verify-email"},
					{"/": "/magic-login"},
					{"/": "/join"},
				},
			}},
		},
	})
	if err != nil {
		return nil
	}
	return aasa
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
	return stats.ChoreInfo{ID: c.ID, HouseholdID: c.HouseholdID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category, HasVolumeML: c.HasVolumeML, HasRating: c.HasRating, MetricType: c.MetricType, MetricUnit: c.MetricUnit, IndicatorLabels: c.IndicatorLabels, Visibility: c.Visibility}, nil
}

func (a *choreStatsAdapter) ListChores(ctx context.Context, householdID int64) ([]stats.ChoreInfo, error) {
	chores, err := a.store.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	result := make([]stats.ChoreInfo, len(chores))
	for i, c := range chores {
		result[i] = stats.ChoreInfo{ID: c.ID, HouseholdID: c.HouseholdID, Name: c.Name, Icon: c.Icon, Color: c.Color, Category: c.Category, HasVolumeML: c.HasVolumeML, HasRating: c.HasRating, MetricType: c.MetricType, MetricUnit: c.MetricUnit, IndicatorLabels: c.IndicatorLabels, Visibility: c.Visibility}
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

// splitCommaList splits a comma-separated config value into trimmed,
// non-empty entries.
func splitCommaList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

var swCacheNameRE = regexp.MustCompile(`("nabu-static-)v\d+(")`)

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
