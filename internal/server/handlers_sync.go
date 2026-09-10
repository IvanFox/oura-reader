package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ivan-lissitsnoi/oura-reader/internal/oura"
	"github.com/ivan-lissitsnoi/oura-reader/internal/scheduler"
)

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())

	if hasRange(r) {
		s.handleBackfill(w, r, u.ID, "")
		return
	}

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

	if hasRange(r) {
		s.handleBackfill(w, r, u.ID, endpoint)
		return
	}

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

func hasRange(r *http.Request) bool {
	q := r.URL.Query()
	return q.Has("start_date") || q.Has("end_date")
}

// handleBackfill fetches an explicit date range from Oura without moving the
// incremental sync cursor. endpoint == "" means every date-ranged endpoint.
func (s *Server) handleBackfill(w http.ResponseWriter, r *http.Request, userID int64, endpoint string) {
	w.Header().Set("Content-Type", "application/json")

	q := r.URL.Query()
	start, end, err := scheduler.ParseRange(q.Get("start_date"), q.Get("end_date"), time.Now())
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	var counts map[string]int
	if endpoint == "" {
		counts, err = s.scheduler.BackfillUser(r.Context(), userID, start, end)
	} else {
		spec, ok := oura.RegistryMap[endpoint]
		if !ok || !spec.HasDates {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "unknown or non-dated endpoint"})
			return
		}
		counts, err = s.scheduler.BackfillEndpoints(r.Context(), userID, start, end, endpoint)
	}

	resp := map[string]any{"status": "ok", "start_date": start, "end_date": end, "records": counts}
	if err != nil {
		resp["status"] = "error"
		resp["error"] = "backfill failed: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(resp)
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
