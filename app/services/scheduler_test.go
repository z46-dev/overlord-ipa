package services

import (
	"testing"
	"time"
)

func TestCronScheduleSupportsSecondsField(t *testing.T) {
	schedule, err := parseCronSchedule("*/15 * * * * *")
	if err != nil {
		t.Fatalf("parse cron: %v", err)
	}

	start := time.Date(2026, 6, 15, 12, 0, 1, 0, time.UTC)
	next := schedule.Next(start)
	expected := time.Date(2026, 6, 15, 12, 0, 15, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected next run %s, got %s", expected, next)
	}
}

func TestCronScheduleSupportsFiveFieldCron(t *testing.T) {
	schedule, err := parseCronSchedule("*/5 * * * *")
	if err != nil {
		t.Fatalf("parse cron: %v", err)
	}

	start := time.Date(2026, 6, 15, 12, 2, 30, 0, time.UTC)
	next := schedule.Next(start)
	expected := time.Date(2026, 6, 15, 12, 5, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected next run %s, got %s", expected, next)
	}
}

func TestIntervalSecondsToCron(t *testing.T) {
	cases := map[int64]string{
		30:  "*/30 * * * * *",
		60:  "0 */1 * * * *",
		300: "0 */5 * * * *",
	}

	for seconds, expected := range cases {
		if actual := intervalSecondsToCron(seconds); actual != expected {
			t.Fatalf("expected %d seconds to become %q, got %q", seconds, expected, actual)
		}
	}
}
