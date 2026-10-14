package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	authURL  = "https://cloud.ouraring.com/oauth/authorize"
	// Apps registered on developer.ouraring.com (new portal) are rejected by the
	// legacy https://api.ouraring.com/oauth/token with 400 invalid_request.
	tokenURL = "https://moi.ouraring.com/oauth/v2/ext/oauth-token"
)

var ouraScopes = []string{
	"email", "personal", "daily", "session",
	"heartrate", "workout", "tag", "spo2",
	"stress", "heart_health", "ring_configuration",
}

const refreshTimeout = 30 * time.Second

type Manager struct {
	conf  *oauth2.Config
	store *Store

	// sources holds one token source per user so concurrent callers never
	// refresh with the same single-use refresh token.
	sources map[int64]*persistingTokenSource
	mu      sync.Mutex

	// pendingStates maps state → userID for in-progress OAuth flows.
	pendingStates map[string]int64
	stateMu       sync.Mutex
}

func NewManager(clientID, clientSecret, baseURL string, store *Store) *Manager {
	redirectURL := baseURL + "/api/v1/auth/callback"
	return &Manager{
		conf: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  authURL,
				TokenURL: tokenURL,
			},
			RedirectURL: redirectURL,
			Scopes:      ouraScopes,
		},
		store:         store,
		sources:       make(map[int64]*persistingTokenSource),
		pendingStates: make(map[string]int64),
	}
}

// LoginURL generates the authorization URL for a user.
func (m *Manager) LoginURL(userID int64) (string, error) {
	state, err := randomState()
	if err != nil {
		return "", err
	}

	m.stateMu.Lock()
	m.pendingStates[state] = userID
	m.stateMu.Unlock()

	v := url.Values{
		"client_id":     {m.conf.ClientID},
		"redirect_uri":  {m.conf.RedirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(m.conf.Scopes, " ")},
		"state":         {state},
	}
	return authURL + "?" + v.Encode(), nil
}

// Exchange handles the OAuth callback: validates state, exchanges code for tokens, persists.
func (m *Manager) Exchange(ctx context.Context, state, code string) (int64, error) {
	m.stateMu.Lock()
	userID, ok := m.pendingStates[state]
	if ok {
		delete(m.pendingStates, state)
	}
	m.stateMu.Unlock()

	if !ok {
		return 0, fmt.Errorf("invalid or expired OAuth state")
	}

	token, err := m.conf.Exchange(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("exchange code: %w", err)
	}

	if err := m.store.SaveToken(ctx, userID, token); err != nil {
		return 0, fmt.Errorf("save token: %w", err)
	}

	m.mu.Lock()
	delete(m.sources, userID)
	m.mu.Unlock()

	return userID, nil
}

// HTTPClientForUser returns an HTTP client with automatic token refresh for a user.
func (m *Manager) HTTPClientForUser(ctx context.Context, userID int64) (*http.Client, error) {
	src, err := m.tokenSource(ctx, userID)
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(ctx, src), nil
}

func (m *Manager) tokenSource(ctx context.Context, userID int64) (*persistingTokenSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if src, ok := m.sources[userID]; ok {
		return src, nil
	}

	token, err := m.store.LoadToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load token for user %d: %w", userID, err)
	}
	if token == nil {
		return nil, fmt.Errorf("no OAuth token for user %d", userID)
	}

	// Detached from ctx: Oura rotates the refresh token on receipt, so a refresh
	// aborted by a disconnecting caller would lose the new pair.
	refreshCtx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: refreshTimeout})
	src := &persistingTokenSource{
		base:           m.conf.TokenSource(refreshCtx, token),
		store:          m.store,
		userID:         userID,
		persistedToken: token.AccessToken,
	}
	m.sources[userID] = src
	return src, nil
}

// HasToken checks if a user has an OAuth token stored.
func (m *Manager) HasToken(ctx context.Context, userID int64) (bool, error) {
	return m.store.HasToken(ctx, userID)
}

// TokenExpiry returns the token expiry time for a user.
func (m *Manager) TokenExpiry(ctx context.Context, userID int64) (string, error) {
	token, err := m.store.LoadToken(ctx, userID)
	if err != nil {
		return "", err
	}
	if token == nil {
		return "", nil
	}
	return token.Expiry.Format("2006-01-02T15:04:05Z"), nil
}

// persistingTokenSource wraps an oauth2.TokenSource and saves refreshed tokens.
type persistingTokenSource struct {
	base   oauth2.TokenSource
	store  *Store
	userID int64

	mu             sync.Mutex
	persistedToken string // access token last written to the store
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	if token.AccessToken == s.persistedToken {
		return token, nil
	}

	if err := s.store.SaveToken(context.Background(), s.userID, token); err != nil {
		// The in-memory token is still usable; saving is retried on the next call.
		slog.Error("failed to persist refreshed oauth token", "user_id", s.userID, "err", err)
		return token, nil
	}
	s.persistedToken = token.AccessToken
	slog.Info("oauth token refreshed", "user_id", s.userID, "expiry", token.Expiry)
	return token, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
