package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultSecretsDir = "/run/secrets"

type Config struct {
	ClientID      string
	ClientSecret  string
	EncryptionKey string
	DBPath        string
	ListenAddr    string
	FetchInterval time.Duration
	SecretsDir    string
}

func Load() (*Config, error) {
	secretsDir := envOrDefault("OURA_SECRETS_DIR", defaultSecretsDir)

	cfg := &Config{
		ClientID:      secretOrEnv(secretsDir, "oura_client_id", "OURA_CLIENT_ID"),
		ClientSecret:  secretOrEnv(secretsDir, "oura_client_secret", "OURA_CLIENT_SECRET"),
		EncryptionKey: secretOrEnv(secretsDir, "oura_encryption_key", "OURA_ENCRYPTION_KEY"),
		DBPath:        envOrDefault("OURA_DB_PATH", "data/oura.db"),
		ListenAddr:    envOrDefault("OURA_LISTEN_ADDR", "0.0.0.0:8080"),
		SecretsDir:    secretsDir,
	}

	intervalStr := envOrDefault("OURA_FETCH_INTERVAL", "6h")
	d, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid OURA_FETCH_INTERVAL %q: %w", intervalStr, err)
	}
	cfg.FetchInterval = d

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.ClientID == "" {
		return fmt.Errorf("OURA_CLIENT_ID is required (set env var or create %s/oura_client_id)", c.SecretsDir)
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("OURA_CLIENT_SECRET is required (set env var or create %s/oura_client_secret)", c.SecretsDir)
	}
	if c.EncryptionKey == "" {
		return fmt.Errorf("OURA_ENCRYPTION_KEY is required (set env var or create %s/oura_encryption_key)", c.SecretsDir)
	}
	return nil
}

// secretOrEnv reads a secret from a file first, falling back to an environment variable.
// File path: <secretsDir>/<filename>
func secretOrEnv(secretsDir, filename, envKey string) string {
	path := filepath.Join(secretsDir, filename)
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	return os.Getenv(envKey)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
