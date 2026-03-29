package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
)

const (
	authURL  = "https://cloud.ouraring.com/oauth/authorize"
	tokenURL = "https://api.ouraring.com/oauth/token"
)

var ouraScopes = []string{
	"email", "personal", "daily", "session",
	"heartrate", "workout", "tag", "spo2",
}

type Manager struct {
	conf  *oauth2.Config
	store *Store
	mu    sync.Mutex

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

	return m.conf.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
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

	return userID, nil
}

// HTTPClientForUser returns an HTTP client with automatic token refresh for a user.
func (m *Manager) HTTPClientForUser(ctx context.Context, userID int64) (*http.Client, error) {
	token, err := m.store.LoadToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load token for user %d: %w", userID, err)
	}
	if token == nil {
		return nil, fmt.Errorf("no OAuth token for user %d", userID)
	}

	// Create a token source that persists refreshed tokens.
	src := &persistingTokenSource{
		base:   m.conf.TokenSource(ctx, token),
		store:  m.store,
		userID: userID,
	}
	return oauth2.NewClient(ctx, src), nil
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
	mu     sync.Mutex
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}

	// Persist the token in case it was refreshed (new refresh token).
	if err := s.store.SaveToken(context.Background(), s.userID, token); err != nil {
		// Log but don't fail — the token is still usable.
		fmt.Printf("warning: failed to persist refreshed token for user %d: %v\n", s.userID, err)
	}

	return token, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
