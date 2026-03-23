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
