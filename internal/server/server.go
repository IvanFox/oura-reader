package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/ivan-lissitsnoi/oura-reader/internal/oauth"
	"github.com/ivan-lissitsnoi/oura-reader/internal/oura"
	"github.com/ivan-lissitsnoi/oura-reader/internal/scheduler"
	"github.com/ivan-lissitsnoi/oura-reader/internal/store"
	"github.com/ivan-lissitsnoi/oura-reader/internal/user"
)

type Server struct {
	addr       string
	store      *store.Store
	userMgr    *user.Manager
	oauthMgr   *oauth.Manager
	ouraClient *oura.Client
	scheduler  *scheduler.Scheduler
	httpServer *http.Server
}

func New(addr string, st *store.Store, userMgr *user.Manager, oauthMgr *oauth.Manager, ouraClient *oura.Client, sched *scheduler.Scheduler) *Server {
	s := &Server{
		addr:       addr,
		store:      st,
		userMgr:    userMgr,
		oauthMgr:   oauthMgr,
		ouraClient: ouraClient,
		scheduler:  sched,
	}
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)

	// Public routes
	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// OAuth callback is public (browser redirect, no API key in flow).
	r.Get("/api/v1/auth/callback", s.handleAuthCallback)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth(s.userMgr))

		r.Get("/api/v1/auth/login", s.handleAuthLogin)
		r.Get("/api/v1/auth/status", s.handleAuthStatus)

		r.Post("/api/v1/sync", s.handleSync)
		r.Post("/api/v1/sync/{endpoint}", s.handleSyncEndpoint)
		r.Get("/api/v1/sync/status", s.handleSyncStatus)

		r.Get("/api/v1/data", s.handleGetAllData)
		r.Get("/api/v1/data/{endpoint}", s.handleGetData)
	})

	return r
}
