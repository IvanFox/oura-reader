package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ivan-lissitsnoi/oura-reader/internal/oura"
)

func (s *Server) handleGetData(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	endpoint := chi.URLParam(r, "endpoint")

	if _, ok := oura.RegistryMap[endpoint]; !ok {
		http.Error(w, `{"error":"unknown endpoint"}`, http.StatusBadRequest)
		return
	}

	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)

	data, total, err := s.store.QueryOuraData(r.Context(), u.ID, endpoint, startDate, endDate, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if data == nil {
		data = []json.RawMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data":   data,
		"count":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleGetAllData(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")
	limit := queryInt(r, "limit", 100)

	result := make(map[string]any)
	for _, spec := range oura.Registry {
		data, _, err := s.store.QueryOuraData(r.Context(), u.ID, spec.Name, startDate, endDate, limit, 0)
		if err != nil {
			continue
		}
		if data == nil {
			data = []json.RawMessage{}
		}
		result[spec.Name] = data
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}
