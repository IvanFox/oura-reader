package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())

	loginURL, err := s.oauthMgr.LoginURL(u.ID)
	if err != nil {
		http.Error(w, `{"error":"failed to generate login URL"}`, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, loginURL, http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, `{"error":"missing code or state"}`, http.StatusBadRequest)
		return
	}

	userID, err := s.oauthMgr.Exchange(r.Context(), state, code)
	if err != nil {
		http.Error(w, `{"error":"OAuth exchange failed: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "authenticated",
		"user_id": userID,
	})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())

	hasToken, err := s.oauthMgr.HasToken(r.Context(), u.ID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	expiry, _ := s.oauthMgr.TokenExpiry(r.Context(), u.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"authenticated": hasToken,
		"token_expiry":  expiry,
	})
}
