package oauth

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ivan-lissitsnoi/oura-reader/internal/crypto"
	"github.com/ivan-lissitsnoi/oura-reader/internal/store"
	"golang.org/x/oauth2"
)

func TestOuraScopesContainsAllRequired(t *testing.T) {
	required := []string{
		"email", "personal", "daily", "session",
		"heartrate", "workout", "tag", "spo2",
		"stress", "heart_health", "ring_configuration",
	}
	have := make(map[string]bool)
	for _, s := range ouraScopes {
		have[s] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Errorf("missing required scope: %q", r)
		}
	}
}

func TestHTTPClientForUser_ClientsCreatedBeforeRefreshShareRefreshedToken(t *testing.T) {
	oura := newFakeOura(t)
	api := newEchoAuthServer(t)
	m, ts, userID := newTestManager(t, oura.srv.URL)
	saveToken(t, ts, userID, "access-0", "refresh-0", time.Now().Add(-time.Hour))

	// Both clients exist before either refreshes, as with overlapping scheduled and API-triggered syncs.
	c1, err := m.HTTPClientForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := m.HTTPClientForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := get(c1, api.URL); err != nil {
		t.Fatalf("first client: %v", err)
	}
	auth, err := get(c2, api.URL)
	if err != nil {
		t.Fatalf("second client: %v", err)
	}
	if auth != "Bearer access-1" {
		t.Errorf("second client sent %q, want %q", auth, "Bearer access-1")
	}
	if got := oura.refreshCount(); got != 1 {
		t.Errorf("refresh requests = %d, want 1", got)
	}
}

func TestHTTPClientForUser_RefreshCompletesAfterCallerContextCancelled(t *testing.T) {
	oura := newFakeOura(t)
	api := newEchoAuthServer(t)
	m, ts, userID := newTestManager(t, oura.srv.URL)
	saveToken(t, ts, userID, "access-0", "refresh-0", time.Now().Add(-time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	c, err := m.HTTPClientForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	if _, err := get(c, api.URL); err != nil {
		t.Fatalf("request after caller cancellation: %v", err)
	}
	stored, err := ts.LoadToken(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RefreshToken != "refresh-1" {
		t.Errorf("stored refresh token = %q, want %q", stored.RefreshToken, "refresh-1")
	}
}

func TestHTTPClientForUser_LogsEachRefreshOnce(t *testing.T) {
	logs := captureLogs(t)
	oura := newFakeOura(t)
	api := newEchoAuthServer(t)
	m, ts, userID := newTestManager(t, oura.srv.URL)
	saveToken(t, ts, userID, "access-0", "refresh-0", time.Now().Add(-time.Hour))

	c, err := m.HTTPClientForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := get(c, api.URL); err != nil {
			t.Fatal(err)
		}
	}

	if got := logs.count("oauth token refreshed"); got != 1 {
		t.Errorf("refresh log records = %d, want 1", got)
	}
}

func TestExchange_ReplacesTokenUsedByExistingUser(t *testing.T) {
	oura := newFakeOura(t)
	api := newEchoAuthServer(t)
	m, ts, userID := newTestManager(t, oura.srv.URL)
	saveToken(t, ts, userID, "access-0", "refresh-0", time.Now().Add(time.Hour))

	before, err := m.HTTPClientForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := get(before, api.URL); err != nil {
		t.Fatal(err)
	}

	loginURL, err := m.LoginURL(userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Exchange(context.Background(), stateFromLoginURL(t, loginURL), "auth-code"); err != nil {
		t.Fatal(err)
	}

	after, err := m.HTTPClientForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := get(after, api.URL)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer access-1" {
		t.Errorf("after re-auth sent %q, want %q", auth, "Bearer access-1")
	}
}

// fakeOura is a token endpoint that, like Oura, rotates refresh tokens and rejects reuse.
type fakeOura struct {
	srv *httptest.Server

	mu        sync.Mutex
	valid     string
	issued    int
	refreshes int
}

func newFakeOura(t *testing.T) *fakeOura {
	t.Helper()
	f := &fakeOura{valid: "refresh-0"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serveToken))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOura) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch r.PostForm.Get("grant_type") {
	case "refresh_token":
		if r.PostForm.Get("refresh_token") != f.valid {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		f.refreshes++
	case "authorization_code":
	default:
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"unsupported_grant_type"}`)
		return
	}
	f.issued++
	f.valid = fmt.Sprintf("refresh-%d", f.issued)
	fmt.Fprintf(w, `{"access_token":"access-%d","refresh_token":%q,"token_type":"bearer","expires_in":3600}`, f.issued, f.valid)
}

func (f *fakeOura) refreshCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refreshes
}

// newEchoAuthServer responds with the request's Authorization header.
func newEchoAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Header.Get("Authorization"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestManager(t *testing.T, tokenURL string) (*Manager, *Store, int64) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	res, err := st.DB().Exec(`INSERT INTO users (name, api_key_hash, api_key_prefix) VALUES ('test', 'hash', 'prefix')`)
	if err != nil {
		t.Fatal(err)
	}
	userID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	cipher, err := crypto.NewCipher("test-key")
	if err != nil {
		t.Fatal(err)
	}
	ts := NewStore(st.DB(), cipher)
	m := NewManager("client-id", "client-secret", "http://localhost", ts)
	m.conf.Endpoint.TokenURL = tokenURL
	return m, ts, userID
}

func saveToken(t *testing.T, ts *Store, userID int64, access, refresh string, expiry time.Time) {
	t.Helper()
	tok := &oauth2.Token{AccessToken: access, RefreshToken: refresh, TokenType: "bearer", Expiry: expiry}
	if err := ts.SaveToken(context.Background(), userID, tok); err != nil {
		t.Fatal(err)
	}
}

func get(c *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func stateFromLoginURL(t *testing.T, loginURL string) string {
	t.Helper()
	u, err := url.Parse(loginURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("state")
}

type logRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func captureLogs(t *testing.T) *logRecorder {
	t.Helper()
	rec := &logRecorder{}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return rec
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, rec.Message)
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

func (r *logRecorder) count(msg string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, m := range r.msgs {
		if m == msg {
			n++
		}
	}
	return n
}
