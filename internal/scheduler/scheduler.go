package scheduler

import (
	"context"
	"encoding/json"
	"errors"
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

	n, err := s.fetchAndStore(ctx, userID, spec, startDate, today)
	if err != nil {
		return err
	}

	if spec.HasDates {
		if err := s.store.SetSyncState(ctx, userID, spec.Name, today); err != nil {
			return fmt.Errorf("set sync state: %w", err)
		}
	}

	slog.Info("synced endpoint", "endpoint", spec.Name, "user_id", userID, "records", n)
	return nil
}

// fetchAndStore fetches [startDate, endDate] from Oura and upserts every record.
// Returns the number of records received.
func (s *Scheduler) fetchAndStore(ctx context.Context, userID int64, spec oura.EndpointSpec, startDate, endDate string) (int, error) {
	records, err := s.client.Fetch(ctx, userID, spec, startDate, endDate)
	if err != nil {
		return 0, err
	}

	for _, raw := range records {
		ouraID := oura.ExtractField(raw, spec.IDField)
		day := oura.ExtractDay(raw, spec)

		// For heartrate with no ID field, use the timestamp as ouraID.
		if spec.IDField == "" && spec.DayField == "timestamp" {
			ouraID = oura.ExtractField(raw, "timestamp")
		}

		if err := s.store.UpsertOuraData(ctx, userID, spec.Name, day, ouraID, json.RawMessage(raw)); err != nil {
			return 0, fmt.Errorf("upsert: %w", err)
		}
	}
	return len(records), nil
}

const dateLayout = "2006-01-02"

// ParseRange validates an explicit backfill range. startDate is required,
// endDate defaults to today. Both must be YYYY-MM-DD and start <= end.
func ParseRange(startDate, endDate string, now time.Time) (string, string, error) {
	if startDate == "" {
		return "", "", errors.New("start_date is required")
	}
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return "", "", errors.New("start_date must be YYYY-MM-DD")
	}
	if endDate == "" {
		endDate = now.Format(dateLayout)
	}
	end, err := time.Parse(dateLayout, endDate)
	if err != nil {
		return "", "", errors.New("end_date must be YYYY-MM-DD")
	}
	if end.Before(start) {
		return "", "", errors.New("start_date must not be after end_date")
	}
	return startDate, endDate, nil
}

// BackfillEndpoints fetches an explicit date range for the given endpoints and
// upserts it. It never touches sync_state, so the incremental cursor is not moved.
//
// The upstream end_date is widened by one day: Oura's sleep/sleep_time endpoints
// do not return documents for the last day of a range (a start_date == end_date
// window comes back empty). Upserts are idempotent, so the extra day is harmless
// for the other endpoints. Returned counts include that extra day.
func (s *Scheduler) BackfillEndpoints(ctx context.Context, userID int64, startDate, endDate string, endpoints ...string) (map[string]int, error) {
	end, err := time.Parse(dateLayout, endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end_date: %w", err)
	}
	fetchEnd := end.AddDate(0, 0, 1).Format(dateLayout)

	counts := make(map[string]int, len(endpoints))
	var errs []error
	for _, name := range endpoints {
		spec, ok := oura.RegistryMap[name]
		if !ok {
			errs = append(errs, fmt.Errorf("unknown endpoint: %s", name))
			continue
		}
		if !spec.HasDates {
			errs = append(errs, fmt.Errorf("%s: endpoint has no date range", name))
			continue
		}
		n, err := s.fetchAndStore(ctx, userID, spec, startDate, fetchEnd)
		if err != nil {
			slog.Error("backfill failed", "endpoint", name, "user_id", userID, "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		counts[name] = n
		slog.Info("backfilled endpoint", "endpoint", name, "user_id", userID,
			"start_date", startDate, "end_date", endDate, "records", n)
	}
	return counts, errors.Join(errs...)
}

// BackfillUser backfills every date-ranged endpoint for a user.
func (s *Scheduler) BackfillUser(ctx context.Context, userID int64, startDate, endDate string) (map[string]int, error) {
	var names []string
	for _, spec := range oura.Registry {
		if spec.HasDates {
			names = append(names, spec.Name)
		}
	}
	return s.BackfillEndpoints(ctx, userID, startDate, endDate, names...)
}
