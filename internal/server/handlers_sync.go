package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())

	if err := s.scheduler.SyncUser(r.Context(), u.ID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "sync failed: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (s *Server) handleSyncEndpoint(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	endpoint := chi.URLParam(r, "endpoint")

	if err := s.scheduler.SyncEndpoints(r.Context(), u.ID, endpoint); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "sync failed: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "endpoint": endpoint})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())

	states, err := s.store.GetAllSyncStates(r.Context(), u.ID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(states)
}
