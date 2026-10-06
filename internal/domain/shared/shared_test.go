package shared_test

import (
	"testing"

	"ascend/internal/domain/shared"
)

func TestLocalDateMath(t *testing.T) {
	d := shared.LocalDate("2026-09-24") // Thursday
	if d.Weekday() != 3 {
		t.Fatalf("weekday = %d", d.Weekday())
	}
	if d.StartOfWeek() != "2026-09-21" || d.StartOfMonth() != "2026-09-01" {
		t.Fatalf("week %s month %s", d.StartOfWeek(), d.StartOfMonth())
	}
	if d.AddDays(8) != "2026-10-02" || d.AddDays(-365) != "2025-09-24" {
		t.Fatal("AddDays across month/year boundaries")
	}
	if shared.DaysBetween("2026-03-01", "2026-03-31") != 30 {
		t.Fatal("DaysBetween across the DST change")
	}
	if d.StartOfMonth().AddMonths(-5) != "2026-04-01" {
		t.Fatal("AddMonths")
	}
	if len(shared.DateRange("2026-09-28", "2026-10-02")) != 5 || len(shared.DateRange("2026-10-02", "2026-09-28")) != 0 {
		t.Fatal("DateRange")
	}
	if shared.LocalDate("2026-02-30").IsValid() || !d.IsValid() || shared.LocalDate("").IsValid() {
		t.Fatal("IsValid")
	}
}

func TestRoundMatchesJavaScript(t *testing.T) {
	if shared.Round(2.5, 0) != 3 || shared.Round(-2.5, 0) != -2 || shared.Round(60.249, 1) != 60.2 {
		t.Fatal("Round")
	}
}

func TestFormatNumber(t *testing.T) {
	cases := map[float64]string{0: "0", 999: "999", 1000: "1,000", 12345.5: "12,345.5", 1234567: "1,234,567", -4200: "-4,200"}
	for in, want := range cases {
		if got := shared.FormatNumber(in); got != want {
			t.Errorf("FormatNumber(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestIsClockTime(t *testing.T) {
	for _, ok := range []string{"00:00", "09:30", "23:59"} {
		if !shared.IsClockTime(ok) {
			t.Errorf("%s should be valid", ok)
		}
	}
	for _, bad := range []string{"24:00", "9:30", "12:60", "noon"} {
		if shared.IsClockTime(bad) {
			t.Errorf("%s should be invalid", bad)
		}
	}
}
