package domain

import (
	"testing"
	"time"
)

func TestParseTimeOfDay(t *testing.T) {
	t.Run("valid HH:MM", func(t *testing.T) {
		tod, err := ParseTimeOfDay("09:05")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tod.String() != "09:05" {
			t.Errorf("String() = %q, want 09:05", tod.String())
		}
		if tod.MinutesSinceStart() != 9*60+5 {
			t.Errorf("MinutesSinceStart() = %d, want %d", tod.MinutesSinceStart(), 9*60+5)
		}
	})

	for _, bad := range []string{"", "24:00", "12:60", "noon", "12:00:00", "10-00", "1234"} {
		t.Run("invalid "+bad, func(t *testing.T) {
			if _, err := ParseTimeOfDay(bad); err == nil {
				t.Errorf("ParseTimeOfDay(%q) expected error, got nil", bad)
			}
		})
	}

	// time.Parse("15:04") is lenient about a single-digit hour: "1:05" parses as
	// 01:05. (The doc comment claims it is rejected; Go accepts it.)
	if tod, err := ParseTimeOfDay("1:05"); err != nil || tod.String() != "01:05" {
		t.Errorf("ParseTimeOfDay(\"1:05\") = (%v, %v), want (01:05, nil)", tod, err)
	}
}

func TestTimeOfDay_AfterAndMinutesUntil(t *testing.T) {
	from := mustTOD(t, "10:00")
	to := mustTOD(t, "11:30")

	if !to.After(from) {
		t.Error("11:30 should be After 10:00")
	}
	if from.After(to) {
		t.Error("10:00 should not be After 11:30")
	}

	mins, ok := from.MinutesUntil(to)
	if !ok || mins != 90 {
		t.Errorf("MinutesUntil = (%d,%v), want (90,true)", mins, ok)
	}

	// Reversed / equal order → (0, false).
	if mins, ok := to.MinutesUntil(from); ok || mins != 0 {
		t.Errorf("reversed MinutesUntil = (%d,%v), want (0,false)", mins, ok)
	}
	if mins, ok := from.MinutesUntil(from); ok || mins != 0 {
		t.Errorf("equal MinutesUntil = (%d,%v), want (0,false)", mins, ok)
	}
}

func TestTimeOfDay_ToTimeIn(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	tod := mustTOD(t, "14:30")
	date := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	got := tod.ToTimeIn(date, loc)

	if got.Hour() != 14 || got.Minute() != 30 {
		t.Errorf("ToTimeIn hour/min = %d:%d, want 14:30", got.Hour(), got.Minute())
	}
	if got.Location() != loc {
		t.Errorf("ToTimeIn location = %v, want %v", got.Location(), loc)
	}
	if y, m, d := got.Date(); y != 2026 || m != time.July || d != 1 {
		t.Errorf("ToTimeIn date = %d-%02d-%02d, want 2026-07-01", y, m, d)
	}
}

func TestTimeOfDay_ValueAndScan(t *testing.T) {
	tod := mustTOD(t, "08:15")

	v, err := tod.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	if v.(string) != "08:15" {
		t.Errorf("Value = %v, want 08:15", v)
	}

	// Scan accepts both "HH:MM" and "HH:MM:SS" (Postgres TIME text form).
	var scanned TimeOfDay
	for _, src := range []string{"08:15", "08:15:00"} {
		if err := scanned.Scan(src); err != nil {
			t.Fatalf("Scan(%q) error: %v", src, err)
		}
		if scanned.String() != "08:15" {
			t.Errorf("Scan(%q) → %q, want 08:15", src, scanned.String())
		}
	}

	// Non-string source is rejected.
	if err := scanned.Scan(123); err == nil {
		t.Error("Scan(int) expected error")
	}
	// Malformed string is rejected.
	if err := scanned.Scan("bad"); err == nil {
		t.Error("Scan(\"bad\") expected error")
	}
}

func TestBookingStatus_String(t *testing.T) {
	if StatusConfirmed.String() != "confirmed" {
		t.Errorf("String() = %q, want confirmed", StatusConfirmed.String())
	}
}

func mustTOD(t *testing.T, s string) TimeOfDay {
	t.Helper()
	tod, err := ParseTimeOfDay(s)
	if err != nil {
		t.Fatalf("ParseTimeOfDay(%q): %v", s, err)
	}
	return tod
}
