package store

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMessagePartitionYearBoundsUsesUTC(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	value := time.Date(2027, time.January, 1, 7, 0, 0, 0, location)
	if year := MessagePartitionYear(value); year != 2026 {
		t.Fatalf("partition year = %d, want 2026", year)
	}
	start, end, err := MessagePartitionYearBounds(2026)
	if err != nil {
		t.Fatalf("year bounds: %v", err)
	}
	if !start.Equal(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)) ||
		!end.Equal(time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("bounds = %s .. %s", start, end)
	}
}

func TestEnsureMessagePartitionWindowIsNoopOutsidePostgres(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := EnsureMessagePartitionWindow(context.Background(), db, time.Now()); err != nil {
		t.Fatalf("ensure sqlite partition window: %v", err)
	}
}

func TestMessageOnlineWindowKeepsCurrentAndPreviousUTCYears(t *testing.T) {
	now := time.Date(2026, time.July, 16, 8, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	if year := MessageMinimumOnlineYear(now); year != 2025 {
		t.Fatalf("minimum online year = %d, want 2025", year)
	}
	want := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	if cutoff := MessageOnlineCutoff(now); !cutoff.Equal(want) {
		t.Fatalf("online cutoff = %s, want %s", cutoff, want)
	}
	wantEnd := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if end := MessageOnlineEnd(now); !end.Equal(wantEnd) {
		t.Fatalf("online end = %s, want %s", end, wantEnd)
	}
}
