package scheduler

import (
	"testing"
	"time"
)

func TestIncrementalRange(t *testing.T) {
	cases := []struct {
		name, last, today  string
		wantStart, wantEnd string
		wantErr            bool
	}{
		// The bug: cursor == today used to produce start == end == today,
		// a window in which Oura returns no sleep documents.
		{name: "cursor is today", last: "2026-09-10", today: "2026-09-10", wantStart: "2026-09-03", wantEnd: "2026-09-11"},
		{name: "cursor behind", last: "2026-09-01", today: "2026-09-10", wantStart: "2026-08-25", wantEnd: "2026-09-11"},
		{name: "first sync", last: "", today: "2026-09-10", wantStart: "2026-08-11", wantEnd: "2026-09-11"},
		{name: "month boundary", last: "2026-03-03", today: "2026-03-31", wantStart: "2026-02-24", wantEnd: "2026-04-01"},
		{name: "bad cursor", last: "garbage", today: "2026-09-10", wantErr: true},
		{name: "bad today", last: "2026-09-10", today: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, err := IncrementalRange(tc.last, tc.today)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got (%q, %q)", start, end)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("got (%q, %q), want (%q, %q)", start, end, tc.wantStart, tc.wantEnd)
			}
			if start >= end {
				t.Errorf("window must span more than one day: (%q, %q)", start, end)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)

	cases := []struct {
		name, start, end   string
		wantStart, wantEnd string
		wantErr            bool
	}{
		{name: "explicit range", start: "2026-04-01", end: "2026-09-10", wantStart: "2026-04-01", wantEnd: "2026-09-10"},
		{name: "end defaults to today", start: "2026-08-20", wantStart: "2026-08-20", wantEnd: "2026-09-10"},
		{name: "single day", start: "2026-08-20", end: "2026-08-20", wantStart: "2026-08-20", wantEnd: "2026-08-20"},
		{name: "missing start", end: "2026-08-20", wantErr: true},
		{name: "bad start format", start: "20.08.2026", wantErr: true},
		{name: "bad end format", start: "2026-08-20", end: "2026-8-1", wantErr: true},
		{name: "start after end", start: "2026-08-21", end: "2026-08-20", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStart, gotEnd, err := ParseRange(tc.start, tc.end, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got (%q, %q)", gotStart, gotEnd)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotStart != tc.wantStart || gotEnd != tc.wantEnd {
				t.Errorf("got (%q, %q), want (%q, %q)", gotStart, gotEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}
