package scheduler

import (
	"testing"
	"time"
)

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
