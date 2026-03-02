package timeprovider

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")

	tp := New(Config{TimeLocation: loc})
	if tp == nil {
		t.Fatal("New() returned nil")
	}
	if tp.GetLocation() != loc {
		t.Errorf("New() location = %v, want %v", tp.GetLocation(), loc)
	}
}

func TestTimeProvider_GetLocation(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	tp := &timeProvider{timeLocation: loc}

	if got := tp.GetLocation(); got != loc {
		t.Errorf("GetLocation() = %v, want %v", got, loc)
	}
}

func TestTimeProvider_ParseTime(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	tp := &timeProvider{timeLocation: loc}

	t.Run("valid RFC3339", func(t *testing.T) {
		got, err := tp.ParseTime(time.RFC3339, "2024-06-15T10:30:00+07:00")
		if err != nil {
			t.Fatalf("ParseTime() error = %v", err)
		}
		// Verify the parsed time has correct values
		if got.Year() != 2024 || got.Month() != 6 || got.Day() != 15 {
			t.Errorf("ParseTime() date = %v, want 2024-06-15", got)
		}
	})

	t.Run("valid DateOnly layout", func(t *testing.T) {
		got, err := tp.ParseTime(time.DateOnly, "2024-06-15")
		if err != nil {
			t.Fatalf("ParseTime() error = %v", err)
		}
		want := time.Date(2024, 6, 15, 0, 0, 0, 0, loc)
		if !got.Equal(want) {
			t.Errorf("ParseTime() = %v, want %v", got, want)
		}
	})

	t.Run("valid custom layout", func(t *testing.T) {
		got, err := tp.ParseTime("2006/01/02", "2024/06/15")
		if err != nil {
			t.Fatalf("ParseTime() error = %v", err)
		}
		if got.Year() != 2024 || got.Month() != 6 || got.Day() != 15 {
			t.Errorf("ParseTime() date = %v, want 2024-06-15", got)
		}
	})

	t.Run("invalid value for layout", func(t *testing.T) {
		_, err := tp.ParseTime(time.DateOnly, "not-a-date")
		if err == nil {
			t.Error("Expected error for invalid value")
		}
	})

	t.Run("uses provider location", func(t *testing.T) {
		got, err := tp.ParseTime(time.DateOnly, "2024-06-15")
		if err != nil {
			t.Fatalf("ParseTime() error = %v", err)
		}
		if got.Location().String() != loc.String() {
			t.Errorf("ParseTime() location = %v, want %v", got.Location(), loc)
		}
	})
}

func TestTimeProvider_GetNow(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	tp := &timeProvider{timeLocation: loc}

	before := time.Now().In(loc)
	got := tp.GetNow()
	after := time.Now().In(loc)

	if got.Before(before) || got.After(after) {
		t.Errorf("GetNow() = %v, expected between %v and %v", got, before, after)
	}
	if got.Location().String() != loc.String() {
		t.Errorf("GetNow() location = %v, want %v", got.Location(), loc)
	}
}

func TestTimeProvider_IsBetween(t *testing.T) {
	loc := time.UTC
	tp := &timeProvider{timeLocation: loc}

	base := time.Date(2024, 6, 15, 12, 0, 0, 0, loc)
	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)

	tests := []struct {
		name   string
		target time.Time
		from   time.Time
		to     time.Time
		want   bool
	}{
		{"target within range", base, from, to, true},
		{"target equals from (inclusive)", from, from, to, true},
		{"target equals to (inclusive)", to, from, to, true},
		{"target before range", base.Add(-2 * time.Hour), from, to, false},
		{"target after range", base.Add(2 * time.Hour), from, to, false},
		{"from equals to, target matches", from, from, from, true},
		{"from equals to, target differs", base, from, from, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tp.IsBetween(tt.target, tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsBetween() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeProvider_IsExpired(t *testing.T) {
	loc := time.UTC
	tp := &timeProvider{timeLocation: loc}

	t.Run("already expired (past time, no buffer)", func(t *testing.T) {
		pastTime := time.Now().Add(-2 * time.Hour)
		if !tp.IsExpired(pastTime, 0) {
			t.Error("IsExpired() = false, want true for past time")
		}
	})

	t.Run("not expired (future time, no buffer)", func(t *testing.T) {
		futureTime := time.Now().Add(2 * time.Hour)
		if tp.IsExpired(futureTime, 0) {
			t.Error("IsExpired() = true, want false for future time")
		}
	})

	t.Run("expired with buffer (time + buffer < now)", func(t *testing.T) {
		// time is 2 hours ago, buffer is 1 hour → expired at 1h ago
		pastTime := time.Now().Add(-2 * time.Hour)
		if !tp.IsExpired(pastTime, time.Hour) {
			t.Error("IsExpired() = false, want true (expired 1h ago)")
		}
	})

	t.Run("not expired with buffer (time + buffer > now)", func(t *testing.T) {
		// time is 30 minutes ago, buffer is 1 hour → expires in 30m
		recentTime := time.Now().Add(-30 * time.Minute)
		if tp.IsExpired(recentTime, time.Hour) {
			t.Error("IsExpired() = true, want false (not expired yet)")
		}
	})

	t.Run("future time with buffer extends expiry further", func(t *testing.T) {
		futureTime := time.Now().Add(time.Hour)
		if tp.IsExpired(futureTime, time.Hour) {
			t.Error("IsExpired() = true, want false for future time + positive buffer")
		}
	})
	t.Run("future time with buffer extends expiry further", func(t *testing.T) {
		futureTime := time.Now().Add(time.Hour)
		if !tp.IsExpired(futureTime, 2*time.Hour) {
			t.Error("IsExpired() = false, want true for future time + positive buffer")
		}
	})
}

func TestTimeProvider_GetToday(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	tp := &timeProvider{timeLocation: loc}

	today := tp.GetToday()
	if !today.IsValid() {
		t.Errorf("GetToday() = %q is not a valid date", today)
	}

	// GetToday should equal GetDateOnly(now)
	expected := tp.GetDateOnly(time.Now())
	if today != expected {
		t.Errorf("GetToday() = %v, want %v", today, expected)
	}
}

func TestTimeProvider_IsToday(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	tp := &timeProvider{timeLocation: loc}

	t.Run("today returns true", func(t *testing.T) {
		today := tp.GetToday()
		if !tp.IsToday(today) {
			t.Errorf("IsToday(%v) = false, want true", today)
		}
	})

	t.Run("yesterday returns false", func(t *testing.T) {
		yesterday, err := tp.SubDays(tp.GetToday(), 1)
		if err != nil {
			t.Fatalf("SubDays() error = %v", err)
		}
		if tp.IsToday(yesterday) {
			t.Errorf("IsToday(%v) = true, want false", yesterday)
		}
	})

	t.Run("tomorrow returns false", func(t *testing.T) {
		tomorrow, err := tp.AddDays(tp.GetToday(), 1)
		if err != nil {
			t.Fatalf("AddDays() error = %v", err)
		}
		if tp.IsToday(tomorrow) {
			t.Errorf("IsToday(%v) = true, want false", tomorrow)
		}
	})

	t.Run("fixed past date returns false", func(t *testing.T) {
		if tp.IsToday(DateOnlyT("2020-01-01")) {
			t.Error("IsToday(2020-01-01) = true, want false")
		}
	})
}
