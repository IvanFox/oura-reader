package oura

import (
	"net/url"
	"testing"
)

func queryOf(t *testing.T, rawURL string) url.Values {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	return u.Query()
}

// Heartrate-style time-series endpoints must filter via start_datetime/end_datetime.
// Oura silently ignores start_date/end_date on these, returning an unfiltered
// window regardless of the range we ask for.
func TestBuildURL_TimeSeriesUsesDatetime(t *testing.T) {
	c := &Client{}
	q := queryOf(t, c.buildURL(RegistryMap["heartrate"], "2026-07-01", "2026-07-15", ""))

	if got := q.Get("start_datetime"); got != "2026-07-01T00:00:00" {
		t.Errorf("start_datetime = %q, want 2026-07-01T00:00:00", got)
	}
	if got := q.Get("end_datetime"); got != "2026-07-15T23:59:59" {
		t.Errorf("end_datetime = %q, want 2026-07-15T23:59:59", got)
	}
	if q.Has("start_date") || q.Has("end_date") {
		t.Errorf("time-series endpoint must not send date params: %v", q)
	}
}

// Daily/date endpoints keep using start_date/end_date.
func TestBuildURL_DateEndpointUsesDate(t *testing.T) {
	c := &Client{}
	q := queryOf(t, c.buildURL(RegistryMap["daily_sleep"], "2026-07-01", "2026-07-15", ""))

	if got := q.Get("start_date"); got != "2026-07-01" {
		t.Errorf("start_date = %q, want 2026-07-01", got)
	}
	if got := q.Get("end_date"); got != "2026-07-15" {
		t.Errorf("end_date = %q, want 2026-07-15", got)
	}
	if q.Has("start_datetime") || q.Has("end_datetime") {
		t.Errorf("date endpoint must not send datetime params: %v", q)
	}
}

// An empty range emits no bounds, and next_token is always preserved.
func TestBuildURL_NextTokenPreservedNoBounds(t *testing.T) {
	c := &Client{}
	q := queryOf(t, c.buildURL(RegistryMap["heartrate"], "", "", "tok123"))

	if got := q.Get("next_token"); got != "tok123" {
		t.Errorf("next_token = %q, want tok123", got)
	}
	if q.Has("start_datetime") || q.Has("start_date") {
		t.Errorf("empty range must not emit start params: %v", q)
	}
}
