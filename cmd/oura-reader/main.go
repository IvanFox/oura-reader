package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ivan-lissitsnoi/oura-reader/internal/config"
	"github.com/ivan-lissitsnoi/oura-reader/internal/crypto"
	"github.com/ivan-lissitsnoi/oura-reader/internal/oauth"
	"github.com/ivan-lissitsnoi/oura-reader/internal/oura"
	"github.com/ivan-lissitsnoi/oura-reader/internal/scheduler"
	"github.com/ivan-lissitsnoi/oura-reader/internal/server"
	"github.com/ivan-lissitsnoi/oura-reader/internal/store"
	"github.com/ivan-lissitsnoi/oura-reader/internal/user"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(); err != nil {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	case "user":
		if err := runUser(); err != nil {
			slog.Error("user command error", "err", err)
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: oura-reader <command>

Commands:
  serve                      Start the HTTP server and scheduler
  user add --name <name>     Create a new user (prints API key)
  user list                  List all users
  user rotate --name <name>  Rotate a user's API key
  user remove --name <name>  Delete a user and their data
`)
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	st, err := store.New(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	cipher, err := crypto.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	userMgr := user.NewManager(st.DB())
	oauthStore := oauth.NewStore(st.DB(), cipher)
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://" + cfg.ListenAddr
	}
	baseURL = strings.TrimRight(baseURL, "/")
	oauthMgr := oauth.NewManager(cfg.ClientID, cfg.ClientSecret, baseURL, oauthStore)
	ouraClient := oura.NewClient(oauthMgr)
	sched := scheduler.New(cfg.FetchInterval, ouraClient, st, userMgr, oauthMgr)

	srv := server.New(cfg.ListenAddr, st, userMgr, oauthMgr, ouraClient, sched)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sched.Start()
	defer sched.Stop()

	slog.Info("starting server", "addr", cfg.ListenAddr)
	return srv.Run(ctx)
}

func runUser() error {
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := store.New(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	mgr := user.NewManager(st.DB())
	ctx := context.Background()

	switch os.Args[2] {
	case "add":
		name := flagValue("--name")
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		key, err := mgr.Add(ctx, name)
		if err != nil {
			return err
		}
		fmt.Printf("User %q created.\nAPI Key: %s\n\nSave this key — it cannot be retrieved later.\n", name, key)

	case "list":
		users, err := mgr.List(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			fmt.Println("No users.")
			return nil
		}
		fmt.Printf("%-4s %-20s %-20s %s\n", "ID", "Name", "Key Prefix", "Created")
		for _, u := range users {
			fmt.Printf("%-4d %-20s %-20s %s\n", u.ID, u.Name, u.APIKeyPrefix, u.CreatedAt)
		}

	case "rotate":
		name := flagValue("--name")
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		key, err := mgr.Rotate(ctx, name)
		if err != nil {
			return err
		}
		fmt.Printf("API key rotated for %q.\nNew API Key: %s\n\nSave this key — it cannot be retrieved later.\n", name, key)

	case "remove":
		name := flagValue("--name")
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if err := mgr.Remove(ctx, name); err != nil {
			return err
		}
		fmt.Printf("User %q removed.\n", name)

	default:
		printUsage()
		os.Exit(1)
	}

	return nil
}

func flagValue(name string) string {
	for i, arg := range os.Args {
		if arg == name && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}
