package main

import (
	"testing"
	"time"
)

func TestNextDailyRun(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 31, 23, 58, 0, 0, location)
	next, err := nextDailyRun(now, "23:59", location)
	if err != nil {
		t.Fatalf("next daily run: %v", err)
	}
	want := time.Date(2026, 8, 31, 23, 59, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("next daily run = %s, want %s", next, want)
	}

	next, err = nextDailyRun(want, "23:59", location)
	if err != nil {
		t.Fatalf("next day run: %v", err)
	}
	want = time.Date(2026, 9, 1, 23, 59, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("next day run = %s, want %s", next, want)
	}
}

func TestNextDailyRunRejectsInvalidClock(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	if _, err := nextDailyRun(time.Now().In(location), "midnight", location); err == nil {
		t.Fatal("expected invalid clock error")
	}
}

func TestScheduledRawLogBackupTargetsPreviousDay(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	next := time.Date(2026, 9, 1, 0, 5, 0, 0, location)
	backupDay := next.AddDate(0, 0, -1)
	want := time.Date(2026, 8, 31, 0, 5, 0, 0, location)
	if !backupDay.Equal(want) {
		t.Fatalf("backup day = %s, want %s", backupDay, want)
	}
}
