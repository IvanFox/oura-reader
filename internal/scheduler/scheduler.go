package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ivan-lissitsnoi/oura-reader/internal/oura"
	"github.com/ivan-lissitsnoi/oura-reader/internal/store"
)

// UserLister returns user IDs that have OAuth tokens.
type UserLister interface {
	GetAllWithTokens(ctx context.Context) ([]int64, error)
}

// TokenChecker checks if a user has a valid OAuth token.
type TokenChecker interface {
	HasToken(ctx context.Context, userID int64) (bool, error)
}

type Scheduler struct {
	interval     time.Duration
	client       *oura.Client
	store        *store.Store
	userLister   UserLister
	tokenChecker TokenChecker
	stopCh       chan struct{}
	doneCh       chan struct{}
}

func New(interval time.Duration, client *oura.Client, st *store.Store, userLister UserLister, tokenChecker TokenChecker) *Scheduler {
	return &Scheduler{
		interval:     interval,
		client:       client,
		store:        st,
		userLister:   userLister,
		tokenChecker: tokenChecker,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	go s.run()
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

func (s *Scheduler) run() {
	defer close(s.doneCh)

	// Run once at startup.
	s.syncAll()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.syncAll()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) syncAll() {
	ctx := context.Background()
	userIDs, err := s.userLister.GetAllWithTokens(ctx)
	if err != nil {
		slog.Error("failed to list users for sync", "err", err)
		return
	}

	for _, userID := range userIDs {
		slog.Info("syncing user", "user_id", userID)
		if err := s.SyncUser(ctx, userID); err != nil {
			slog.Error("sync failed for user", "user_id", userID, "err", err)
		}
	}
}

// SyncUser syncs all endpoints for a single user.
func (s *Scheduler) SyncUser(ctx context.Context, userID int64) error {
	return s.SyncEndpoints(ctx, userID, oura.EndpointNames()...)
}

// SyncEndpoints syncs specific endpoints for a user.
func (s *Scheduler) SyncEndpoints(ctx context.Context, userID int64, endpoints ...string) error {
	today := time.Now().Format("2006-01-02")
	var errs []error

	for _, name := range endpoints {
		spec, ok := oura.RegistryMap[name]
		if !ok {
			errs = append(errs, fmt.Errorf("unknown endpoint: %s", name))
			continue
		}

		if err := s.syncEndpoint(ctx, userID, spec, today); err != nil {
			slog.Error("endpoint sync failed", "endpoint", name, "user_id", userID, "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d endpoint(s) failed", len(errs))
	}
	return nil
}

func (s *Scheduler) syncEndpoint(ctx context.Context, userID int64, spec oura.EndpointSpec, today string) error {
	startDate := ""
	if spec.HasDates {
		lastDate, _, err := s.store.GetSyncState(ctx, userID, spec.Name)
		if err != nil {
			return fmt.Errorf("get sync state: %w", err)
		}
		if lastDate == "" {
			// Default: 30 days ago for first sync.
			startDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		} else {
			startDate = lastDate
		}
	}

	records, err := s.client.Fetch(ctx, userID, spec, startDate, today)
	if err != nil {
		return err
	}

	for _, raw := range records {
		ouraID := oura.ExtractField(raw, spec.IDField)
		day := oura.ExtractDay(raw, spec)

		// For heartrate with no ID field, use the timestamp as ouraID.
		if spec.IDField == "" && spec.DayField == "timestamp" {
			ouraID = oura.ExtractField(raw, "timestamp")
		}

		if err := s.store.UpsertOuraData(ctx, userID, spec.Name, day, ouraID, json.RawMessage(raw)); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
	}

	if spec.HasDates {
		if err := s.store.SetSyncState(ctx, userID, spec.Name, today); err != nil {
			return fmt.Errorf("set sync state: %w", err)
		}
	}

	slog.Info("synced endpoint", "endpoint", spec.Name, "user_id", userID, "records", len(records))
	return nil
}
