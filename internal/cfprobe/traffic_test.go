package cfprobe

import (
	"testing"
	"time"
)

func TestPeriodStartTSClampsResetDay(t *testing.T) {
	now := time.Date(2026, time.February, 15, 12, 0, 0, 0, time.UTC)
	got := time.Unix(periodStartTS(now, 31), 0).UTC()
	want := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestPeriodStartTSRollsMissingResetDayToNextMonth(t *testing.T) {
	now := time.Date(2026, time.April, 30, 12, 0, 0, 0, time.UTC)
	got := time.Unix(periodStartTS(now, 31), 0).UTC()
	want := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestPeriodStartTSNoReset(t *testing.T) {
	if got := periodStartTS(time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC), 0); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}
