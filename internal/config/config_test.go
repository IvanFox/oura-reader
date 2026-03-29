package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretOrEnv_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "oura_client_id")
	if err := os.WriteFile(secretFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := secretOrEnv(dir, "oura_client_id", "OURA_CLIENT_ID")
	if got != "from-file" {
		t.Fatalf("got %q, want %q", got, "from-file")
	}
}

func TestSecretOrEnv_FallsBackToEnv(t *testing.T) {
	t.Setenv("OURA_CLIENT_ID", "from-env")

	got := secretOrEnv("/nonexistent", "oura_client_id", "OURA_CLIENT_ID")
	if got != "from-env" {
		t.Fatalf("got %q, want %q", got, "from-env")
	}
}

func TestSecretOrEnv_FileOverridesEnv(t *testing.T) {
	t.Setenv("OURA_CLIENT_ID", "from-env")

	dir := t.TempDir()
	secretFile := filepath.Join(dir, "oura_client_id")
	if err := os.WriteFile(secretFile, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := secretOrEnv(dir, "oura_client_id", "OURA_CLIENT_ID")
	if got != "from-file" {
		t.Fatalf("got %q, want %q (file should override env)", got, "from-file")
	}
}

func TestValidate_BaseURL_Valid(t *testing.T) {
	cfg := &Config{
		ClientID:      "id",
		ClientSecret:  "secret",
		EncryptionKey: "key",
		BaseURL:       "https://server.tail1234.ts.net",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_BaseURL_Empty_IsValid(t *testing.T) {
	cfg := &Config{
		ClientID:      "id",
		ClientSecret:  "secret",
		EncryptionKey: "key",
		BaseURL:       "",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_BaseURL_BadScheme(t *testing.T) {
	cfg := &Config{
		ClientID:      "id",
		ClientSecret:  "secret",
		EncryptionKey: "key",
		BaseURL:       "ftp://server.example.com",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for non-http(s) scheme")
	}
}

func TestValidate_BaseURL_HasPath(t *testing.T) {
	cfg := &Config{
		ClientID:      "id",
		ClientSecret:  "secret",
		EncryptionKey: "key",
		BaseURL:       "https://server.example.com/some/path",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for URL with path")
	}
}

func TestLoad_BaseURL_FromEnv(t *testing.T) {
	t.Setenv("OURA_BASE_URL", "https://myserver.ts.net")
	t.Setenv("OURA_CLIENT_ID", "id")
	t.Setenv("OURA_CLIENT_SECRET", "secret")
	t.Setenv("OURA_ENCRYPTION_KEY", "key")
	t.Setenv("OURA_SECRETS_DIR", "/nonexistent")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://myserver.ts.net" {
		t.Fatalf("got BaseURL %q, want %q", cfg.BaseURL, "https://myserver.ts.net")
	}
}

func TestSecretOrEnv_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "oura_encryption_key")
	if err := os.WriteFile(secretFile, []byte("  secret-key  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := secretOrEnv(dir, "oura_encryption_key", "OURA_ENCRYPTION_KEY")
	if got != "secret-key" {
		t.Fatalf("got %q, want %q", got, "secret-key")
	}
}
